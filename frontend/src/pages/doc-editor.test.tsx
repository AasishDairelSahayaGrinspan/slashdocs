import { beforeEach, describe, expect, it, vi } from "vitest";
import { fireEvent, screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { api, APIError, type Version } from "@/api";
import { renderWithProviders } from "@/test/render";
import { DocEditor } from "@/pages/doc-editor";

const ORIGINAL = "<!doctype html><html><body><p>Helo world</p></body></html>";
const FIXED = "<!doctype html><html><body><p>Hello world</p></body></html>";

const savedVersion: Version = { id: "ver_2", number: 2 };

// sonner renders toasts into a portal that is not mounted in these tests;
// stubbing keeps the component under test from touching real DOM globals.
vi.mock("sonner", () => ({
  toast: { success: vi.fn(), error: vi.fn() },
}));

beforeEach(() => {
  vi.spyOn(api, "getVersionHTML").mockResolvedValue(ORIGINAL);
});

describe("DocEditor", () => {
  it("loads the base version HTML into the textarea", async () => {
    renderWithProviders(
      <DocEditor docId="doc_1" baseVersion="ver_1" onClose={vi.fn()} />,
    );

    const box = await screen.findByRole("textbox", { name: /document html/i });
    expect(box).toHaveValue(ORIGINAL);
    expect(api.getVersionHTML).toHaveBeenCalledWith("doc_1", "ver_1");
  });

  it("disables Save until the HTML actually changes", async () => {
    const user = userEvent.setup();
    renderWithProviders(
      <DocEditor docId="doc_1" baseVersion="ver_1" onClose={vi.fn()} />,
    );

    const box = await screen.findByRole("textbox", { name: /document html/i });
    const save = screen.getByRole("button", { name: /save new version/i });
    expect(save).toBeDisabled();

    await user.clear(box);
    await user.type(box, "x");
    expect(save).toBeEnabled();
  });

  it("posts the edited HTML against the base version and closes", async () => {
    const user = userEvent.setup();
    const create = vi.spyOn(api, "createVersion").mockResolvedValue(savedVersion);
    const onClose = vi.fn();
    renderWithProviders(
      <DocEditor docId="doc_1" baseVersion="ver_1" onClose={onClose} />,
    );

    const box = await screen.findByRole("textbox", { name: /document html/i });
    await user.clear(box);
    await user.type(box, FIXED);
    await user.type(
      screen.getByRole("textbox", { name: /change summary/i }),
      "fix typo",
    );
    await user.click(screen.getByRole("button", { name: /save new version/i }));

    await waitFor(() => expect(create).toHaveBeenCalledTimes(1));
    const [doc, base, summary, file] = create.mock.calls[0];
    expect(doc).toBe("doc_1");
    expect(base).toBe("ver_1");
    expect(summary).toBe("fix typo");
    await expect(file.text()).resolves.toBe(FIXED);
    await waitFor(() => expect(onClose).toHaveBeenCalledTimes(1));
  });

  it("keeps the edits and shows a conflict banner on a stale base version", async () => {
    const user = userEvent.setup();
    vi.spyOn(api, "createVersion").mockRejectedValue(
      new APIError(409, "version_conflict", "base_version_id is stale", {
        current_version_id: "ver_9",
      }),
    );
    const onClose = vi.fn();
    renderWithProviders(
      <DocEditor docId="doc_1" baseVersion="ver_1" onClose={onClose} />,
    );

    const box = await screen.findByRole("textbox", { name: /document html/i });
    await user.clear(box);
    await user.type(box, FIXED);
    await user.click(screen.getByRole("button", { name: /save new version/i }));

    expect(await screen.findByRole("alert")).toHaveTextContent(
      /someone else saved a newer version/i,
    );
    // Re-query rather than reuse `box`: if a regression made the component
    // setDraft(null) on conflict, the textarea would unmount but a stale
    // node reference would still report the old value and this assertion
    // would pass anyway.
    expect(screen.getByRole("textbox", { name: /document html/i })).toHaveValue(FIXED);
    expect(onClose).not.toHaveBeenCalled();
  });

  it("surfaces a non-conflict failure without a banner", async () => {
    const user = userEvent.setup();
    const { toast } = await import("sonner");
    vi.spyOn(api, "createVersion").mockRejectedValue(
      new APIError(413, "payload_too_large", "HTML file exceeds 10 MiB"),
    );
    renderWithProviders(
      <DocEditor docId="doc_1" baseVersion="ver_1" onClose={vi.fn()} />,
    );

    const box = await screen.findByRole("textbox", { name: /document html/i });
    await user.clear(box);
    await user.type(box, "x");
    await user.click(screen.getByRole("button", { name: /save new version/i }));

    await waitFor(() =>
      expect(toast.error).toHaveBeenCalledWith("HTML file exceeds 10 MiB"),
    );
    expect(screen.queryByRole("alert")).not.toBeInTheDocument();
  });

  it("closes immediately when nothing was edited", async () => {
    const user = userEvent.setup();
    const onClose = vi.fn();
    renderWithProviders(
      <DocEditor docId="doc_1" baseVersion="ver_1" onClose={onClose} />,
    );

    await screen.findByRole("textbox", { name: /document html/i });
    await user.click(screen.getByRole("button", { name: /cancel/i }));
    expect(onClose).toHaveBeenCalledTimes(1);
  });

  it("asks before discarding unsaved edits", async () => {
    const user = userEvent.setup();
    const confirmSpy = vi.spyOn(window, "confirm").mockReturnValue(false);
    const onClose = vi.fn();
    renderWithProviders(
      <DocEditor docId="doc_1" baseVersion="ver_1" onClose={onClose} />,
    );

    const box = await screen.findByRole("textbox", { name: /document html/i });
    await user.clear(box);
    await user.type(box, "x");
    await user.click(screen.getByRole("button", { name: /cancel/i }));

    expect(confirmSpy).toHaveBeenCalledTimes(1);
    expect(onClose).not.toHaveBeenCalled();
  });

  it("reports a failed load instead of showing an empty editor", async () => {
    vi.spyOn(api, "getVersionHTML").mockRejectedValue(
      new APIError(403, "forbidden", "viewer role required"),
    );
    renderWithProviders(
      <DocEditor docId="doc_1" baseVersion="ver_1" onClose={vi.fn()} />,
    );

    expect(
      await screen.findByText(/could not load the document html/i),
    ).toBeInTheDocument();
    expect(
      screen.queryByRole("textbox", { name: /document html/i }),
    ).not.toBeInTheDocument();
  });

  it("inserts two spaces at the caret when Tab is pressed without a selection", async () => {
    renderWithProviders(
      <DocEditor docId="doc_1" baseVersion="ver_1" onClose={vi.fn()} />,
    );

    const box = (await screen.findByRole("textbox", {
      name: /document html/i,
    })) as HTMLTextAreaElement;

    // Collapse the caret to a specific mid-string position (not 0, not the
    // end) so the assertion proves the insert happens *at the caret*, not
    // just somewhere.
    box.setSelectionRange(3, 3);
    fireEvent.keyDown(box, { key: "Tab" });

    expect(box).toHaveValue(`${ORIGINAL.slice(0, 3)}  ${ORIGINAL.slice(3)}`);
  });

  it("does not destroy a selected range when Tab is pressed", async () => {
    renderWithProviders(
      <DocEditor docId="doc_1" baseVersion="ver_1" onClose={vi.fn()} />,
    );

    const box = (await screen.findByRole("textbox", {
      name: /document html/i,
    })) as HTMLTextAreaElement;

    // Select the entire document and press Tab, the way anyone reflexively
    // indenting a selected block would. This must not silently replace the
    // selection with two spaces.
    box.setSelectionRange(0, ORIGINAL.length);
    fireEvent.keyDown(box, { key: "Tab" });

    expect(box).toHaveValue(ORIGINAL);
  });

  it("saves on Cmd/Ctrl+S when dirty, and does nothing when clean", async () => {
    const user = userEvent.setup();
    const create = vi.spyOn(api, "createVersion").mockResolvedValue(savedVersion);
    renderWithProviders(
      <DocEditor docId="doc_1" baseVersion="ver_1" onClose={vi.fn()} />,
    );

    const box = await screen.findByRole("textbox", { name: /document html/i });

    fireEvent.keyDown(box, { key: "s", metaKey: true });
    expect(create).not.toHaveBeenCalled();

    await user.clear(box);
    await user.type(box, "x");
    fireEvent.keyDown(box, { key: "s", metaKey: true });

    await waitFor(() => expect(create).toHaveBeenCalledTimes(1));
  });

  it("registers a beforeunload guard while dirty and removes it once clean again", async () => {
    const user = userEvent.setup();
    const addSpy = vi.spyOn(window, "addEventListener");
    const removeSpy = vi.spyOn(window, "removeEventListener");
    renderWithProviders(
      <DocEditor docId="doc_1" baseVersion="ver_1" onClose={vi.fn()} />,
    );

    const box = await screen.findByRole("textbox", { name: /document html/i });
    expect(addSpy).not.toHaveBeenCalledWith("beforeunload", expect.any(Function));

    await user.type(box, "x");
    expect(addSpy).toHaveBeenCalledWith("beforeunload", expect.any(Function));

    // Restore the exact original text: back to not-dirty should tear the
    // guard back down.
    fireEvent.change(box, { target: { value: ORIGINAL } });
    expect(removeSpy).toHaveBeenCalledWith("beforeunload", expect.any(Function));
  });
});
