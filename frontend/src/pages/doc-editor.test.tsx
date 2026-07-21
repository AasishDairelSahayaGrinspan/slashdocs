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
  // The Back guard pushes a real history entry and deliberately leaves it in
  // place; jsdom keeps one history per test file, so clear the current
  // entry's state to keep tests independent of each other's ordering.
  window.history.replaceState(null, "");
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

  it("keeps saving against the base version the session started on", async () => {
    const user = userEvent.setup();
    const create = vi.spyOn(api, "createVersion").mockResolvedValue(savedVersion);
    const { rerender } = renderWithProviders(
      <DocEditor docId="doc_1" baseVersion="ver_1" onClose={vi.fn()} />,
    );

    const box = await screen.findByRole("textbox", { name: /document html/i });
    await user.clear(box);
    await user.type(box, "MY EDIT");

    // Someone else saved while this session was open, so the parent's
    // `latest` advances and it hands us a newer base. Rebasing the draft
    // onto it would turn the 409 that protects the other person's version
    // into a silent overwrite.
    rerender(<DocEditor docId="doc_1" baseVersion="ver_2" onClose={vi.fn()} />);

    // The editor must not tear the draft down to re-fetch the newer base.
    expect(screen.getByRole("textbox", { name: /document html/i })).toHaveValue(
      "MY EDIT",
    );
    expect(api.getVersionHTML).toHaveBeenCalledTimes(1);

    await user.click(screen.getByRole("button", { name: /save new version/i }));

    await waitFor(() => expect(create).toHaveBeenCalledTimes(1));
    const [, base, , file] = create.mock.calls[0];
    expect(base).toBe("ver_1");
    await expect(file.text()).resolves.toBe("MY EDIT");
  });

  it("cancels a tab close while dirty, in Safari's dialect as well as Chrome's", async () => {
    const user = userEvent.setup();
    renderWithProviders(
      <DocEditor docId="doc_1" baseVersion="ver_1" onClose={vi.fn()} />,
    );

    const box = await screen.findByRole("textbox", { name: /document html/i });
    expect(dispatchBeforeUnload().defaultPrevented).toBe(false);

    await user.type(box, "x");
    const dirtyEvent = dispatchBeforeUnload();
    // preventDefault() alone is only honoured by Chrome 119+ and Firefox;
    // Safari and older Chrome need returnValue set as well.
    expect(dirtyEvent.defaultPrevented).toBe(true);
    expect(dirtyEvent.returnValueSet).toHaveBeenCalledWith("");

    // Restore the exact original text: back to not-dirty should stop
    // blocking the unload.
    fireEvent.change(box, { target: { value: ORIGINAL } });
    expect(dispatchBeforeUnload().defaultPrevented).toBe(false);
  });

  it("turns browser Back into the same discard confirmation as Cancel", async () => {
    const user = userEvent.setup();
    const confirmSpy = vi.spyOn(window, "confirm").mockReturnValue(false);
    // fireEvent.popState dispatches an event without touching jsdom's
    // history, so window.history.state cannot show whether the guard was
    // re-armed -- it still holds whatever was pushed last. Count the pushes
    // instead.
    const pushSpy = vi.spyOn(window.history, "pushState");
    const onClose = vi.fn();
    renderWithProviders(
      <DocEditor docId="doc_1" baseVersion="ver_1" onClose={onClose} />,
    );

    const box = await screen.findByRole("textbox", { name: /document html/i });
    // Clean: Back is an ordinary exit, nothing to guard.
    expect(window.history.state?.aidocsEditorGuard).toBeUndefined();
    expect(pushSpy).not.toHaveBeenCalled();

    await user.type(box, "x");
    // A throwaway entry so Back pops it instead of unmounting the editor
    // (beforeunload does not fire on SPA navigation, and BrowserRouter
    // offers no useBlocker).
    expect(window.history.state?.aidocsEditorGuard).toBe(true);
    expect(pushSpy).toHaveBeenCalledTimes(1);

    fireEvent.popState(window, { state: null });
    expect(confirmSpy).toHaveBeenCalledTimes(1);
    expect(onClose).not.toHaveBeenCalled();
    expect(screen.getByRole("textbox", { name: /document html/i })).toHaveValue(
      `${ORIGINAL}x`,
    );
    // Declining re-arms the guard -- a second entry is pushed -- so a second
    // Back asks again rather than leaving the editor unguarded.
    expect(pushSpy).toHaveBeenCalledTimes(2);
    expect(pushSpy).toHaveBeenLastCalledWith({ aidocsEditorGuard: true }, "");

    confirmSpy.mockReturnValue(true);
    fireEvent.popState(window, { state: null });
    expect(onClose).toHaveBeenCalledTimes(1);
  });

  it("ignores a Back that belongs to another guarded surface", async () => {
    const user = userEvent.setup();
    const confirmSpy = vi.spyOn(window, "confirm").mockReturnValue(true);
    const onClose = vi.fn();
    renderWithProviders(
      <DocEditor docId="doc_1" baseVersion="ver_1" onClose={onClose} />,
    );

    const box = await screen.findByRole("textbox", { name: /document html/i });
    await user.type(box, "x");

    // The mobile comments sheet pushes its own entry on top of ours; Back
    // popping *that* lands on our guard entry and must not close the editor.
    fireEvent.popState(window, { state: { aidocsEditorGuard: true } });

    expect(confirmSpy).not.toHaveBeenCalled();
    expect(onClose).not.toHaveBeenCalled();
  });
});

// jsdom cannot construct a BeforeUnloadEvent, and its plain Event exposes the
// legacy boolean `returnValue` whose setter *is* preventDefault -- which would
// make a returnValue-only handler indistinguishable from a correct one. Shadow
// the property with a spy so the two signals stay independently observable.
function dispatchBeforeUnload() {
  const event = new Event("beforeunload", { cancelable: true });
  const returnValueSet = vi.fn();
  Object.defineProperty(event, "returnValue", {
    configurable: true,
    get: () => "",
    set: returnValueSet,
  });
  window.dispatchEvent(event);
  return { defaultPrevented: event.defaultPrevented, returnValueSet };
}
