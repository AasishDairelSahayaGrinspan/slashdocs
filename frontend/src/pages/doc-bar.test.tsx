import type { ComponentProps } from "react";
import { describe, it, expect, vi, beforeEach } from "vitest";
import { render, screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { MemoryRouter } from "react-router-dom";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { api, type EffectiveAccessItem, type Principal, type Version } from "@/api";
import { queryKeys } from "@/lib/queryKeys";
import { DocBar } from "@/pages/doc-bar";

// These tests cover only the canEdit gate (who sees the "Edit HTML" button).
// Version history, sharing, delete, and full screen are pre-existing and out
// of scope here.

// DocBar mounts ShareSheet unconditionally -- it's a sibling of the Sheet
// trigger, not lazily rendered on open -- so ShareSheet's own useQuery calls
// for documentAccess/config run on every render regardless of which gate case
// is under test. sonner is stubbed the same way doc-editor.test.tsx stubs it,
// since no toast portal is mounted in these tests.
vi.mock("sonner", () => ({
  toast: { success: vi.fn(), error: vi.fn() },
}));

const OWNER: Principal = { type: "user", id: "user_owner", email: "owner@example.com" };
const VERSIONS: Version[] = [
  { id: "ver_1", number: 1 },
  { id: "ver_2", number: 2 },
];

function principal(id: string): Principal {
  return { type: "user", id, email: `${id}@example.com` };
}

function accessItem(p: Principal, effective_role: string): EffectiveAccessItem {
  return { principal: p, effective_role, sources: [] };
}

// A local wrapper rather than @/test/render's renderWithProviders, so a test
// can hold onto the QueryClient and prove the documentAccess query has
// actually settled before asserting a *negative* case ("still hidden after
// the role resolves"). screen.queryByRole would pass trivially on the very
// first render, before any network response, whether or not the gate logic
// is correct -- a bug that only manifests once the role loads would go
// undetected without waiting for settlement first.
function renderDocBar(overrides: Partial<ComponentProps<typeof DocBar>> = {}) {
  const client = new QueryClient({
    defaultOptions: { queries: { retry: false }, mutations: { retry: false } },
  });
  const props: ComponentProps<typeof DocBar> = {
    title: "Spec doc",
    docId: "doc_1",
    owner: OWNER,
    principal: OWNER,
    current: "ver_2",
    latest: "ver_2",
    versions: VERSIONS,
    onSelectVersion: vi.fn(),
    onEdit: vi.fn(),
    commentsOpen: false,
    onToggleComments: vi.fn(),
    ...overrides,
  };
  render(
    <QueryClientProvider client={client}>
      <MemoryRouter>
        <DocBar {...props} />
      </MemoryRouter>
    </QueryClientProvider>,
  );
  return { props, client };
}

async function waitForAccessSettled(client: QueryClient, docId: string) {
  await waitFor(() =>
    expect(client.getQueryState(queryKeys.documentAccess(docId))?.status).toBe(
      "success",
    ),
  );
}

beforeEach(() => {
  // ShareSheet always mounts and always queries getConfig with no `enabled`
  // gate; stub it so it doesn't attempt a real fetch in jsdom.
  vi.spyOn(api, "getConfig").mockResolvedValue({
    deployment: "org",
    org_name: "Test Org",
    everyone_label: "Everyone at Test Org",
  });
});

describe("DocBar edit-permission gate", () => {
  it("shows Edit HTML for the owner, without waiting on the access lookup", () => {
    // Never resolves -- proves the owner short-circuit doesn't depend on
    // documentAccess settling to anything useful (or at all).
    vi.spyOn(api, "documentAccess").mockReturnValue(new Promise(() => {}));

    renderDocBar({ principal: OWNER, owner: OWNER });

    expect(screen.getByRole("button", { name: /edit html/i })).toBeInTheDocument();
  });

  it("shows Edit HTML for a non-owner whose effective role is editor", async () => {
    const editor = principal("user_editor");
    vi.spyOn(api, "documentAccess").mockResolvedValue({
      items: [accessItem(editor, "editor")],
    });

    renderDocBar({ principal: editor });

    expect(
      await screen.findByRole("button", { name: /edit html/i }),
    ).toBeInTheDocument();
  });

  it("hides Edit HTML for a non-owner whose effective role is commenter", async () => {
    const commenter = principal("user_commenter");
    vi.spyOn(api, "documentAccess").mockResolvedValue({
      items: [accessItem(commenter, "commenter")],
    });

    const { client } = renderDocBar({ principal: commenter });
    await waitForAccessSettled(client, "doc_1");

    expect(
      screen.queryByRole("button", { name: /edit html/i }),
    ).not.toBeInTheDocument();
  });

  it("hides Edit HTML for a non-owner whose effective role is viewer", async () => {
    const viewer = principal("user_viewer");
    vi.spyOn(api, "documentAccess").mockResolvedValue({
      items: [accessItem(viewer, "viewer")],
    });

    const { client } = renderDocBar({ principal: viewer });
    await waitForAccessSettled(client, "doc_1");

    expect(
      screen.queryByRole("button", { name: /edit html/i }),
    ).not.toBeInTheDocument();
  });

  it("hides Edit HTML while viewing an older version, even for the owner", () => {
    vi.spyOn(api, "documentAccess").mockResolvedValue({ items: [] });

    renderDocBar({ current: "ver_1", latest: "ver_2" });

    expect(
      screen.queryByRole("button", { name: /edit html/i }),
    ).not.toBeInTheDocument();
  });

  it("hides Edit HTML when there is no latest version yet", () => {
    vi.spyOn(api, "documentAccess").mockResolvedValue({ items: [] });

    renderDocBar({ current: "", latest: "" });

    expect(
      screen.queryByRole("button", { name: /edit html/i }),
    ).not.toBeInTheDocument();
  });

  it("calls onEdit exactly once when clicked", async () => {
    const user = userEvent.setup();
    vi.spyOn(api, "documentAccess").mockReturnValue(new Promise(() => {}));
    const { props } = renderDocBar({ principal: OWNER, owner: OWNER });

    await user.click(screen.getByRole("button", { name: /edit html/i }));

    expect(props.onEdit).toHaveBeenCalledTimes(1);
  });
});
