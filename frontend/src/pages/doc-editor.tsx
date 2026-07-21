import { useCallback, useEffect, useRef, useState } from "react";
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { toast } from "sonner";
import { AlertTriangle, Save, X } from "lucide-react";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { Center, Skeleton } from "@/components/ui/misc";
import { api, type Version } from "@/api";
import { htmlToFile } from "@/lib/html-file";
import { errorMessage, isVersionConflict } from "@/lib/errors";
import { queryKeys } from "@/lib/queryKeys";

// DocEditor edits a document's raw single-file HTML and saves the result as a
// new immutable version. It is deliberately a plain textarea: aidocs documents
// carry inline <script>, inline CSS, and document-KV-bound controls, so the
// HTML must round-trip as an opaque string. Nothing here parses or reserialises
// it.
//
// onClose is called with the saved version on a successful save and with
// nothing when the session is abandoned, so the page can land the user on
// whatever was just written.
export function DocEditor({
  docId,
  baseVersion,
  onClose,
}: {
  docId: string;
  baseVersion: string;
  onClose: (saved?: Version) => void;
}) {
  const q = useQueryClient();
  // Snapshot the base for the lifetime of the session. `baseVersion` is the
  // page's `latest`, which moves whenever the document query refetches (a
  // reconnect, an uploaded version). Following it would re-key the fetch out
  // from under the draft and then save the old-based edits against the new
  // base -- a silent overwrite of whatever the other person just saved,
  // which is precisely the lost update the 409 handling exists to catch.
  const [base] = useState(baseVersion);
  const [draft, setDraft] = useState<string | null>(null);
  const [summary, setSummary] = useState("");
  const [conflict, setConflict] = useState(false);

  const source = useQuery({
    queryKey: queryKeys.versionHTML(docId, base),
    queryFn: () => api.getVersionHTML(docId, base),
    enabled: !!docId && !!base,
    // Versions are immutable, so the fetched HTML never goes stale.
    staleTime: Infinity,
    meta: { suppressGlobalError: true },
  });

  // Seed the draft once. Re-seeding on every source change would clobber the
  // user's edits if the query ever refetched.
  useEffect(() => {
    setDraft((current) =>
      current === null && source.data !== undefined ? source.data : current,
    );
  }, [source.data]);

  const original = source.data ?? "";
  const dirty = draft !== null && source.data !== undefined && draft !== original;

  const save = useMutation({
    mutationFn: () =>
      api.createVersion(docId, base, summary.trim(), htmlToFile(draft ?? "")),
    onSuccess: (saved) => {
      toast.success("New version saved.");
      q.invalidateQueries({ queryKey: queryKeys.versions(docId) });
      q.invalidateQueries({ queryKey: queryKeys.document(docId) });
      q.invalidateQueries({ queryKey: queryKeys.comments(docId) });
      onClose(saved);
    },
    onError: (e) => {
      // A stale base gets a persistent banner rather than a toast: the user's
      // unsaved edits are still in the box and they need a visible decision
      // point, not something that fades.
      if (isVersionConflict(e)) {
        setConflict(true);
        return;
      }
      toast.error(errorMessage(e, "Save failed"));
    },
  });

  // Returns whether the editor actually closed, so the Back guard below can
  // tell "user discarded" from "user stayed".
  const requestClose = useCallback(() => {
    if (dirty && !window.confirm("Discard your unsaved changes?")) return false;
    onClose();
    return true;
  }, [dirty, onClose]);

  // Warn on tab close / reload while edits are unsaved. preventDefault() on
  // its own only cancels in Chrome 119+ and Firefox; Safari and older Chrome
  // still key off returnValue, so set both.
  useEffect(() => {
    if (!dirty) return;
    const onBeforeUnload = (e: BeforeUnloadEvent) => {
      e.preventDefault();
      e.returnValue = "";
    };
    window.addEventListener("beforeunload", onBeforeUnload);
    return () => window.removeEventListener("beforeunload", onBeforeUnload);
  }, [dirty]);

  // beforeunload does not fire on SPA navigation, and the app mounts a
  // BrowserRouter (not a data router), so react-router's useBlocker is
  // unavailable. Guard browser Back the way the mobile comments sheet does in
  // doc-page: push a throwaway history entry once there are unsaved edits, so
  // Back pops *that* instead of dropping ?edit=1 and unmounting this editor
  // with the draft still in it.
  const guarded = useRef(false);
  useEffect(() => {
    if (!dirty || guarded.current) return;
    window.history.pushState({ aidocsEditorGuard: true }, "");
    guarded.current = true;
  }, [dirty]);

  // Registered for the whole session and reading requestClose through a ref,
  // so the entry above is pushed exactly once no matter how often the draft
  // flips between dirty and clean. The entry is deliberately not popped on
  // close: onClose replaces the URL it sits on, so backing over it here would
  // race that replacement straight back into ?edit=1.
  const requestCloseRef = useRef(requestClose);
  requestCloseRef.current = requestClose;
  useEffect(() => {
    const onPop = (e: PopStateEvent) => {
      if (!guarded.current) return;
      // Landing *on* our own entry means some other guarded surface (the
      // mobile comments sheet) popped its entry, not that ours went away.
      if ((e.state as { aidocsEditorGuard?: boolean } | null)?.aidocsEditorGuard)
        return;
      guarded.current = false;
      if (requestCloseRef.current()) return;
      // The user chose to stay: re-arm the guard so the next Back asks again.
      window.history.pushState({ aidocsEditorGuard: true }, "");
      guarded.current = true;
    };
    window.addEventListener("popstate", onPop);
    return () => window.removeEventListener("popstate", onPop);
  }, []);

  const onTextareaKeyDown = (e: React.KeyboardEvent<HTMLTextAreaElement>) => {
    if ((e.metaKey || e.ctrlKey) && e.key.toLowerCase() === "s") {
      e.preventDefault();
      if (dirty && !save.isPending) save.mutate();
      return;
    }
    // Tab indents instead of leaving the editor. Shift+Tab is left alone so
    // keyboard users can still tab out.
    if (e.key !== "Tab" || e.shiftKey) return;
    const el = e.currentTarget;
    const start = el.selectionStart;
    const end = el.selectionEnd;
    // A non-collapsed selection means the insert below would silently
    // replace whatever was selected — and since this textarea is
    // React-controlled, the programmatic rewrite clears the browser's
    // native undo stack, so there'd be no way to get it back (no drafts or
    // autosave either, per spec). Leave Tab's native behavior (move focus)
    // alone rather than risk destroying a selected block of HTML; only a
    // bare caret gets the two-space insert.
    if (start !== end) return;
    e.preventDefault();
    setDraft(`${el.value.slice(0, start)}  ${el.value.slice(end)}`);
    requestAnimationFrame(() => {
      el.selectionStart = el.selectionEnd = start + 2;
    });
  };

  // Show the skeleton both while the fetch is still in flight and for the
  // one render where data has arrived but the seeding effect above hasn't
  // run yet — `draft === null` alone conflates "still loading" with "load
  // failed," which would flash the error state on every successful load
  // (passive effects run in a later scheduler tick, after the browser can
  // paint the pre-effect render).
  if (source.isLoading || (draft === null && !source.error)) {
    return (
      <div className="h-full p-4">
        <Skeleton className="h-full w-full" />
      </div>
    );
  }
  if (source.error) {
    return (
      <Center>
        <div className="max-w-md text-center">
          <p className="text-sm text-[var(--color-fg-muted)]">
            Could not load the document HTML.
          </p>
          <Button
            variant="outline"
            size="sm"
            className="mt-3"
            onClick={() => onClose()}
          >
            Back to document
          </Button>
        </div>
      </Center>
    );
  }
  // Unreachable in practice — the skeleton branch above already covers
  // `draft === null && !source.error`, and this branch covers
  // `source.error` — but TypeScript can't derive `draft !== null` from
  // those two independent conditions, so this keeps the textarea's value
  // genuinely typed as `string` below instead of silently falling back.
  if (draft === null) return null;

  return (
    <div className="flex h-full flex-col bg-[var(--color-surface)]">
      {conflict ? (
        <div
          role="alert"
          className="flex items-start gap-2 border-b border-[var(--color-warning)]/40 bg-[var(--color-warning)]/10 px-4 py-3 text-sm text-[var(--color-fg)]"
        >
          <AlertTriangle className="mt-0.5 h-4 w-4 shrink-0 text-[var(--color-warning)]" />
          <div className="flex-1">
            Someone else saved a newer version while you were editing. Your changes are
            still here — copy them somewhere safe, then reload to start from the latest
            version.
          </div>
          <Button variant="outline" size="sm" onClick={() => window.location.reload()}>
            Reload
          </Button>
        </div>
      ) : null}
      <div className="flex flex-wrap items-center gap-2 border-b border-[var(--color-border)] px-4 py-2">
        <Input
          aria-label="Change summary"
          placeholder="Change summary (optional)"
          className="max-w-xs flex-1"
          value={summary}
          onChange={(e) => setSummary(e.target.value)}
        />
        <div className="flex-1" />
        <span className="text-xs text-[var(--color-fg-muted)]">
          {dirty ? "Unsaved changes" : "No changes"}
        </span>
        <Button variant="ghost" size="sm" onClick={() => requestClose()}>
          <X className="h-4 w-4" /> Cancel
        </Button>
        <Button
          size="sm"
          disabled={!dirty || save.isPending}
          onClick={() => save.mutate()}
        >
          <Save className="h-4 w-4" />
          {save.isPending ? "Saving…" : "Save new version"}
        </Button>
      </div>
      <textarea
        aria-label="Document HTML"
        spellCheck={false}
        autoCapitalize="off"
        autoCorrect="off"
        value={draft}
        onChange={(e) => setDraft(e.target.value)}
        onKeyDown={onTextareaKeyDown}
        className="flex-1 resize-none bg-[var(--color-surface)] p-4 font-mono text-xs leading-relaxed text-[var(--color-fg)] outline-none"
      />
    </div>
  );
}
