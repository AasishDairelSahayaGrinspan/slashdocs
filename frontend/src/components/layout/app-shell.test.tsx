import { beforeEach, describe, expect, it, vi } from "vitest";
import { screen, waitFor } from "@testing-library/react";
import { MemoryRouter, Route, Routes } from "react-router-dom";
import { api, type Principal } from "@/api";
import { renderWithProviders } from "@/test/render";
import { AppShell } from "@/components/layout/app-shell";

// These tests cover only which document modes hide the TopBar. Auth
// redirects and the account menu are pre-existing and out of scope here.

const USER: Principal = { type: "user", id: "user_1", email: "user@example.com" };

// The TopBar's logo is a Link straight out of the document -- while the HTML
// editor is open it is the largest unguarded exit on the page, and it sits
// directly above the disabled back button in the doc bar.
const logo = () => screen.queryByRole("link", { name: /aidocs/i });

function renderShell(url: string) {
  return renderWithProviders(
    <MemoryRouter initialEntries={[url]}>
      <Routes>
        <Route path="/app" element={<AppShell />}>
          <Route path="d/:id" element={<div>document</div>} />
          <Route path="documents" element={<div>documents</div>} />
        </Route>
      </Routes>
    </MemoryRouter>,
  );
}

beforeEach(() => {
  vi.spyOn(api, "me").mockResolvedValue({ principal: USER });
});

describe("AppShell chrome", () => {
  it("shows the top bar on an ordinary document page", async () => {
    renderShell("/app/d/doc_1");

    expect(await screen.findByText("document")).toBeInTheDocument();
    expect(logo()).toBeInTheDocument();
  });

  it("hides the top bar while the HTML editor is open", async () => {
    renderShell("/app/d/doc_1?edit=1");

    expect(await screen.findByText("document")).toBeInTheDocument();
    // Nothing in the top bar can ask about unsaved edits: it navigates out of
    // the editor's tree, and beforeunload does not fire on SPA navigation.
    expect(logo()).not.toBeInTheDocument();
  });

  it("hides the top bar in full screen", async () => {
    renderShell("/app/d/doc_1?fullscreen=1");

    expect(await screen.findByText("document")).toBeInTheDocument();
    expect(logo()).not.toBeInTheDocument();
  });

  it("keeps the top bar on non-document pages that carry an edit param", async () => {
    renderShell("/app/documents?edit=1");

    expect(await screen.findByText("documents")).toBeInTheDocument();
    expect(logo()).toBeInTheDocument();
  });

  it("gives the editor the same full-bleed main region as full screen", async () => {
    renderShell("/app/d/doc_1?edit=1");

    await screen.findByText("document");
    await waitFor(() =>
      expect(screen.getByRole("main")).toHaveClass("flex-1", "overflow-hidden"),
    );
  });
});
