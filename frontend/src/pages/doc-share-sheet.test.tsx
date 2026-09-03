import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import {
  cleanup,
  fireEvent,
  render,
  screen,
  waitFor,
  within,
} from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { afterEach, describe, expect, it, vi } from "vitest";
import { Sheet } from "@/components/ui/sheet";
import { ShareSheet } from "./doc-share-sheet";

type FetchOptions = {
  groupsEnabled?: boolean;
  groupSearchFails?: boolean;
  groupSearchDelayMs?: number;
  groupItems?: Array<{ id: string; email: string; name: string }>;
  accessFails?: boolean;
  canonicalOwner?: boolean;
  owner?: { id: string; email?: string; name?: string };
};

function installFetch(options: FetchOptions = {}) {
  const calls: Array<{ url: string; init?: RequestInit }> = [];
  vi.stubGlobal(
    "fetch",
    vi.fn(async (input: RequestInfo | URL, init?: RequestInit) => {
      const url = String(input);
      calls.push({ url, init });
      if (url === "/v1/config") {
        return jsonResponse({
          deployment: "org",
          org_name: "Acme",
          everyone_label: "Anyone in Acme",
          google_groups_enabled: options.groupsEnabled ?? true,
        });
      }
      if (url.startsWith("/v1/google-groups")) {
        if (options.groupSearchDelayMs) {
          await new Promise((resolve) =>
            setTimeout(resolve, options.groupSearchDelayMs),
          );
        }
        if (options.groupSearchFails) {
          return jsonResponse(
            { error: { code: "dependency_unavailable", message: "temporary" } },
            503,
          );
        }
        return jsonResponse({
          items: options.groupItems ?? [
            {
              id: "groups/platform",
              email: "platform@example.com",
              name: "Platform",
            },
          ],
        });
      }
      if (url.startsWith("/v1/users")) return jsonResponse({ items: [] });
      if (url === "/v1/documents/doc_1/access") {
        if (options.accessFails) {
          return jsonResponse(
            { error: { code: "forbidden", message: "not allowed" } },
            403,
          );
        }
        return jsonResponse({
          current_role: "owner",
          canonical_owner: options.canonicalOwner ?? true,
          items: [
            {
              principal: {
                type: "user",
                ...(options.owner ?? {
                  id: "owner",
                  email: "owner@example.com",
                  name: "Owner",
                }),
              },
              effective_role: "owner",
              sources: [
                {
                  resource_type: "document",
                  resource_id: "doc_1",
                  resource_title: "Doc",
                  role: "owner",
                },
              ],
            },
            {
              principal: {
                type: "google_group",
                id: "groups/owners",
                email: "owners@example.com",
                name: "Owners",
              },
              effective_role: "owner",
              sources: [
                {
                  resource_type: "document",
                  resource_id: "doc_1",
                  resource_title: "Doc",
                  role: "owner",
                  grant_id: "gr_owner",
                },
              ],
            },
            {
              principal: {
                type: "google_group",
                id: "groups/inherited",
                email: "inherited@example.com",
                name: "Inherited group",
              },
              effective_role: "viewer",
              sources: [
                {
                  resource_type: "folder",
                  resource_id: "fld_parent",
                  resource_title: "Parent folder",
                  role: "viewer",
                  grant_id: "gr_inherited",
                },
              ],
            },
          ],
        });
      }
      if (url === "/v1/documents/doc_1/grants") {
        return jsonResponse({ id: "gr_new", role: "commenter" }, 201);
      }
      if (url.includes("/grants/")) return jsonResponse({ id: "gr_owner" });
      throw new Error(`unexpected fetch: ${url}`);
    }),
  );
  return calls;
}

function jsonResponse(payload: unknown, status = 200) {
  return new Response(JSON.stringify(payload), {
    status,
    headers: { "Content-Type": "application/json" },
  });
}

function renderShare(props: { isCanonicalOwner?: boolean } = {}) {
  const client = new QueryClient({
    defaultOptions: { queries: { retry: false }, mutations: { retry: false } },
  });
  return render(
    <QueryClientProvider client={client}>
      <Sheet open>
        <ShareSheet
          docId="doc_1"
          canManageAccess
          isCanonicalOwner={props.isCanonicalOwner ?? true}
        />
      </Sheet>
    </QueryClientProvider>,
  );
}

afterEach(() => {
  cleanup();
  vi.unstubAllGlobals();
  vi.useRealTimers();
});

describe("Google Group sharing", () => {
  it("switches target modes, debounces search, and requires verified selection", async () => {
    const calls = installFetch();
    renderShare();
    expect(await screen.findByRole("tab", { name: "Google Groups" })).toBeVisible();
    fireEvent.click(screen.getByRole("tab", { name: "Google Groups" }));
    const input = screen.getByPlaceholderText("Search group name or email");
    fireEvent.change(input, { target: { value: "plat" } });
    expect(screen.getByRole("button", { name: "Share" })).toBeDisabled();
    await waitFor(() =>
      expect(calls.some((call) => call.url.includes("/v1/google-groups?q=plat"))).toBe(
        true,
      ),
    );
    await waitFor(() => expect(screen.getByText("platform@example.com")).toBeVisible());
    fireEvent.click(screen.getByText("Platform"));
    expect(screen.getByRole("button", { name: "Share" })).toBeEnabled();
    fireEvent.click(screen.getByRole("button", { name: "Share" }));
    await waitFor(() => {
      const request = calls.find(
        (call) =>
          call.url === "/v1/documents/doc_1/grants" && call.init?.method === "POST",
      );
      expect(JSON.parse(String(request?.init?.body))).toEqual({
        principal: { type: "google_group", email: "platform@example.com" },
        role: "commenter",
      });
    });
  });

  it("shows dependency errors and hides group controls when disabled", async () => {
    installFetch({ groupSearchFails: true });
    renderShare();
    await userEvent.click(await screen.findByRole("tab", { name: "Google Groups" }));
    await userEvent.type(
      screen.getByPlaceholderText("Search group name or email"),
      "pl",
    );
    expect(
      await screen.findByText(/Google Group search is temporarily unavailable/i),
    ).toBeVisible();

    cleanup();
    vi.unstubAllGlobals();
    installFetch({ groupsEnabled: false });
    renderShare();
    await waitFor(() =>
      expect(screen.queryByRole("tab", { name: "Google Groups" })).toBeNull(),
    );
  });

  it("shows loading, empty, and access-authorization states", async () => {
    installFetch({ groupSearchDelayMs: 100, groupItems: [], accessFails: true });
    renderShare();
    await userEvent.click(await screen.findByRole("tab", { name: "Google Groups" }));
    await userEvent.type(
      screen.getByPlaceholderText("Search group name or email"),
      "zz",
    );
    expect(await screen.findByText("Searching Google Groups…")).toBeVisible();
    expect(
      await screen.findByText(/No verified organization groups matched/i),
    ).toBeVisible();
    expect(await screen.findByText("Could not load access list.")).toBeVisible();
  });

  it("offers owner only when the current user is the canonical owner", async () => {
    installFetch();
    renderShare({ isCanonicalOwner: false });
    await userEvent.click(await screen.findByRole("tab", { name: "Google Groups" }));
    const form = screen.getByRole("button", { name: "Share" }).closest("form");
    expect(form).not.toBeNull();
    expect(within(form!).queryByRole("option", { name: "Owner" })).toBeNull();

    cleanup();
    renderShare({ isCanonicalOwner: true });
    await userEvent.click(await screen.findByRole("tab", { name: "Google Groups" }));
    const ownerForm = screen.getByRole("button", { name: "Share" }).closest("form");
    expect(within(ownerForm!).getByRole("option", { name: "Owner" })).toBeVisible();
  });

  it("distinguishes canonical, co-owner, direct, and inherited access", async () => {
    installFetch();
    renderShare();
    expect(await screen.findByText("Canonical owner")).toBeVisible();
    expect(screen.getByText("Google Group co-owner · direct access")).toBeVisible();
    expect(screen.getByText("Inherited from Parent folder")).toBeVisible();
  });

  it("does not expose an internal id for an unhydrated canonical owner", async () => {
    installFetch({ owner: { id: "usr_google_123456789" } });
    renderShare();
    expect(await screen.findByText("Canonical owner")).toBeVisible();
    expect(screen.getAllByText("Owner").length).toBeGreaterThan(0);
    expect(screen.queryByText("usr_google_123456789")).toBeNull();
  });

  it("shows owner only to canonical owners and confirms owner-grant changes", async () => {
    installFetch();
    renderShare({ isCanonicalOwner: true });
    await screen.findByText("Google Group co-owner · direct access");
    const ownerSelect = screen
      .getAllByLabelText("Access role")
      .find((select) => (select as HTMLSelectElement).value === "owner")!;
    expect(ownerSelect).toBeEnabled();
    fireEvent.change(ownerSelect, { target: { value: "editor" } });
    expect(await screen.findByText(/Owner-level Google Group access/i)).toBeVisible();
    expect(screen.getByRole("button", { name: "Change owner access" })).toBeVisible();
  });
});
