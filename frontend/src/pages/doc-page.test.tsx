import { beforeEach, describe, expect, it, vi } from "vitest";
import { screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { MemoryRouter, Route, Routes, useLocation } from "react-router-dom";
import type { QueryClient } from "@tanstack/react-query";
import {
  api,
  type Document,
  type EffectiveAccessItem,
  type Principal,
  type Version,
} from "@/api";
import { queryKeys } from "@/lib/queryKeys";
import { renderWithProviders } from "@/test/render";
import { DocumentPage } from "@/pages/doc-page";

// These tests cover the doc-page <-> DocEditor seam: who is allowed into
// ?edit=1, and where the user lands after a save. The editor's own behaviour
// lives in doc-editor.test.tsx.

vi.mock("sonner", () => ({
  toast: { success: vi.fn(), error: vi.fn() },
}));

const OWNER: Principal = { type: "user", id: "user_owner", email: "owner@example.com" };
const ORIGINAL = "<!doctype html><html><body><p>Hi</p></body></html>";

function principal(id: string): Principal {
  return { type: "user", id, email: `${id}@example.com` };
}

function accessItem(p: Principal, effective_role: string): EffectiveAccessItem {
  return { principal: p, effective_role, sources: [] };
}

// The rendered document lives in a cross-origin iframe that jsdom never
// loads, so its <iframe title> is the only observable "we are showing the
// document, not the editor" signal.
const renderFrame = () => screen.queryByTitle("Rendered document");
const htmlBox = () => screen.queryByRole("textbox", { name: /document html/i });

function LocationProbe() {
  const location = useLocation();
  return <div data-testid="location">{`${location.pathname}${location.search}`}</div>;
}

function renderDocPage(url: string) {
  return renderWithProviders(
    <MemoryRouter initialEntries={[url]}>
      <LocationProbe />
      <Routes>
        <Route path="/app/d/:id" element={<DocumentPage />} />
      </Routes>
    </MemoryRouter>,
  );
}

// A negative assertion about the editor is worthless until the permission
// lookup it depends on has actually resolved -- before that, every caller
// looks like a viewer.
async function waitForAccessSettled(client: QueryClient) {
  await waitFor(() =>
    expect(client.getQueryState(queryKeys.documentAccess("doc_1"))?.status).toBe(
      "success",
    ),
  );
}

let doc: Document;

beforeEach(() => {
  doc = {
    id: "doc_1",
    title: "Spec doc",
    owner: OWNER,
    current_version_id: "ver_2",
  };
  vi.spyOn(api, "getDocument").mockImplementation(async () => doc);
  vi.spyOn(api, "listVersions").mockResolvedValue({
    items: [
      { id: "ver_1", number: 1 },
      { id: "ver_2", number: 2 },
    ],
  });
  vi.spyOn(api, "listComments").mockResolvedValue({ items: [] });
  vi.spyOn(api, "getVersionHTML").mockResolvedValue(ORIGINAL);
  vi.spyOn(api, "renderToken").mockResolvedValue({
    token: "tok",
    url: "https://render.example/doc",
  });
  vi.spyOn(api, "getConfig").mockResolvedValue({
    deployment: "org",
    org_name: "Test Org",
    everyone_label: "Everyone at Test Org",
  });
});

describe("DocumentPage ?edit=1 permission gate", () => {
  it("shows the document, not the editor, to a viewer who opens an edit link", async () => {
    const viewer = principal("user_viewer");
    vi.spyOn(api, "me").mockResolvedValue({ principal: viewer });
    vi.spyOn(api, "documentAccess").mockResolvedValue({
      items: [accessItem(viewer, "viewer")],
    });

    const { client } = renderDocPage("/app/d/doc_1?edit=1");
    await waitForAccessSettled(client);
    await waitFor(() => expect(renderFrame()).toBeInTheDocument());

    // The GET .../html endpoint is viewer-accessible, so nothing downstream
    // stops a viewer from filling an editor they can never save from.
    expect(htmlBox()).not.toBeInTheDocument();
    expect(api.getVersionHTML).not.toHaveBeenCalled();
  });

  it("shows the document, not the editor, to a commenter who opens an edit link", async () => {
    const commenter = principal("user_commenter");
    vi.spyOn(api, "me").mockResolvedValue({ principal: commenter });
    vi.spyOn(api, "documentAccess").mockResolvedValue({
      items: [accessItem(commenter, "commenter")],
    });

    const { client } = renderDocPage("/app/d/doc_1?edit=1");
    await waitForAccessSettled(client);
    await waitFor(() => expect(renderFrame()).toBeInTheDocument());

    expect(htmlBox()).not.toBeInTheDocument();
  });

  it("opens the editor for a non-owner whose effective role is editor", async () => {
    const editor = principal("user_editor");
    vi.spyOn(api, "me").mockResolvedValue({ principal: editor });
    vi.spyOn(api, "documentAccess").mockResolvedValue({
      items: [accessItem(editor, "editor")],
    });

    renderDocPage("/app/d/doc_1?edit=1");

    expect(await screen.findByRole("textbox", { name: /document html/i })).toHaveValue(
      ORIGINAL,
    );
  });

  it("opens the editor for the owner without waiting on the access lookup", async () => {
    vi.spyOn(api, "me").mockResolvedValue({ principal: OWNER });
    // Never resolves: the owner short-circuit must not depend on it.
    vi.spyOn(api, "documentAccess").mockReturnValue(new Promise(() => {}));

    renderDocPage("/app/d/doc_1?edit=1");

    expect(await screen.findByRole("textbox", { name: /document html/i })).toHaveValue(
      ORIGINAL,
    );
  });

  it("refuses to edit a pinned older version, matching the bar's own gate", async () => {
    vi.spyOn(api, "me").mockResolvedValue({ principal: OWNER });
    vi.spyOn(api, "documentAccess").mockResolvedValue({ items: [] });

    // ver_1 is genuinely older than the document's current version. The bar
    // hides Edit HTML here (the base would always 409), so the page must not
    // open an editor from the same URL -- and it would silently be editing
    // `latest`, not the version on screen.
    renderDocPage("/app/d/doc_1?version=ver_1&edit=1");

    await waitFor(() => expect(renderFrame()).toBeInTheDocument());
    expect(htmlBox()).not.toBeInTheDocument();
    expect(api.getVersionHTML).not.toHaveBeenCalled();
    expect(api.renderToken).toHaveBeenCalledWith("doc_1", "ver_1");
  });

  it("waits for the access lookup instead of flashing the document at an editor", async () => {
    const editor = principal("user_editor");
    vi.spyOn(api, "me").mockResolvedValue({ principal: editor });
    let grantAccess = () => {};
    vi.spyOn(api, "documentAccess").mockReturnValue(
      new Promise((resolve) => {
        grantAccess = () => resolve({ items: [accessItem(editor, "editor")] });
      }),
    );

    renderDocPage("/app/d/doc_1?edit=1");

    // The rest of the page has loaded; only the role is outstanding.
    await waitFor(() => expect(api.listVersions).toHaveBeenCalled());
    expect(renderFrame()).not.toBeInTheDocument();
    expect(api.renderToken).not.toHaveBeenCalled();

    grantAccess();
    expect(await screen.findByRole("textbox", { name: /document html/i })).toHaveValue(
      ORIGINAL,
    );
    // No render token was ever minted for a frame the user never wanted.
    expect(api.renderToken).not.toHaveBeenCalled();
  });
});

describe("DocumentPage save landing", () => {
  it("moves the user onto the new version when the URL pinned an older one", async () => {
    const user = userEvent.setup();
    vi.spyOn(api, "me").mockResolvedValue({ principal: OWNER });
    vi.spyOn(api, "documentAccess").mockResolvedValue({ items: [] });
    const saved: Version = { id: "ver_3", number: 3 };
    vi.spyOn(api, "createVersion").mockImplementation(async () => {
      // The server has moved on; so does every query that reads it.
      doc = { ...doc, current_version_id: "ver_3" };
      vi.mocked(api.listVersions).mockResolvedValue({
        items: [{ id: "ver_1", number: 1 }, { id: "ver_2", number: 2 }, saved],
      });
      return saved;
    });

    // ?version=ver_2 is what the Full screen link writes and what the version
    // history dialog leaves behind -- an ordinary way to arrive at the editor.
    renderDocPage("/app/d/doc_1?version=ver_2&edit=1");

    const box = await screen.findByRole("textbox", { name: /document html/i });
    await user.type(box, "x");
    await user.click(screen.getByRole("button", { name: /save new version/i }));

    // The pin must go with the edit flag: left behind, it strands the user on
    // the pre-edit HTML and hides the Edit button, which reads as a silently
    // failed save. Poll on the exact URL: toHaveTextContent matches a
    // substring, so the pre-edit "/app/d/doc_1?version=ver_2&edit=1" already
    // contains "/app/d/doc_1" and would let the assertion pass before the
    // save's navigation settles (a race the slower CI runner loses).
    await waitFor(() =>
      expect(screen.getByTestId("location").textContent).toBe("/app/d/doc_1"),
    );
    expect(
      await screen.findByRole("button", { name: /edit html/i }),
    ).toBeInTheDocument();
    await waitFor(() => expect(api.renderToken).toHaveBeenCalledWith("doc_1", "ver_3"));
  });

  it("keeps the user on the document after cancelling an edit", async () => {
    const user = userEvent.setup();
    vi.spyOn(api, "me").mockResolvedValue({ principal: OWNER });
    vi.spyOn(api, "documentAccess").mockResolvedValue({ items: [] });

    renderDocPage("/app/d/doc_1?edit=1");

    await screen.findByRole("textbox", { name: /document html/i });
    await user.click(screen.getByRole("button", { name: /cancel/i }));

    await waitFor(() => expect(htmlBox()).not.toBeInTheDocument());
    expect(screen.getByTestId("location").textContent).toBe("/app/d/doc_1");
  });
});
