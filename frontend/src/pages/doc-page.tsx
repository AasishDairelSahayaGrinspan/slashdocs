import { useTitle } from "@/lib/use-title";
import { useCallback, useEffect, useMemo, useRef, useState } from "react";
import { useNavigate, useSearchParams, useParams } from "react-router-dom";
import { AnimatePresence, motion } from "framer-motion";
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { toast } from "sonner";
import { api, APIError, currentVersionID, docTitle, versionID } from "@/api";
import { queryKeys } from "@/lib/queryKeys";
import { Center, Skeleton } from "@/components/ui/misc";
import { Sheet, SheetContent, SheetHeader, SheetTitle } from "@/components/ui/sheet";
import { Button } from "@/components/ui/button";
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from "@/components/ui/dialog";
import { Input, Label, Textarea } from "@/components/ui/input";
import { useIsMobile } from "@/lib/use-media-query";
import { errorMessage } from "@/lib/errors";
import { DocProvider, type PendingSelection } from "./doc-context";
import { DocBar } from "./doc-bar";
import { RenderFrame } from "./doc-render-frame";
import { Comments } from "./doc-comments";

const MIN_PANEL_WIDTH = 240;
const MAX_PANEL_WIDTH = 640;
const DEFAULT_PANEL_WIDTH = 380;

const pageStagger = {
  hidden: {},
  visible: { transition: { staggerChildren: 0.1, delayChildren: 0.05 } },
};
const barSlide = {
  hidden: { opacity: 0, y: -8 },
  visible: {
    opacity: 1,
    y: 0,
    transition: { duration: 0.35, ease: [0.25, 0.4, 0.25, 1] as const },
  },
};
const contentFade = {
  hidden: { opacity: 0 },
  visible: {
    opacity: 1,
    transition: { duration: 0.4, ease: [0.25, 0.4, 0.25, 1] as const },
  },
};

function DocumentErrorState({
  title,
  description,
}: {
  title: string;
  description: string;
}) {
  return (
    <Center>
      <div className="max-w-md text-center">
        <h1 className="text-lg font-semibold text-[var(--color-fg)]">{title}</h1>
        <p className="mt-2 text-sm text-[var(--color-fg-muted)]">{description}</p>
      </div>
    </Center>
  );
}

export function DocumentPage() {
  const { id = "" } = useParams();
  const navigate = useNavigate();
  const queryClient = useQueryClient();
  const [searchParams, setSearchParams] = useSearchParams();
  const fullscreen = searchParams.get("fullscreen") === "1";
  const queryVersion = searchParams.get("version") || undefined;
  const repositoryId = searchParams.get("repo") || undefined;
  const doc = useQuery({
    queryKey: queryKeys.document(id),
    queryFn: () => api.getDocument(id),
    meta: { suppressGlobalError: true },
  });
  const me = useQuery({ queryKey: queryKeys.me(), queryFn: api.me });
  const effectiveRepositoryId = repositoryId || doc.data?.repository_id || "";
  const repository = useQuery({
    queryKey: queryKeys.repository(effectiveRepositoryId),
    queryFn: () => api.getRepository(effectiveRepositoryId),
    enabled: !!effectiveRepositoryId,
    meta: { suppressGlobalError: true },
  });
  const versions = useQuery({
    queryKey: queryKeys.versions(id),
    queryFn: () => api.listVersions(id),
    enabled: !!id && !doc.error,
    meta: { suppressGlobalError: true },
  });
  // One comments query feeds both the paint overlay and the side panel so they
  // stay in sync.
  const comments = useQuery({
    queryKey: queryKeys.comments(id),
    queryFn: () => api.listComments(id),
    enabled: !!id && !doc.error,
    meta: { suppressGlobalError: true },
  });
  const [selection, setSelection] = useState<PendingSelection | null>(null);
  const [activeComment, setActiveComment] = useState<string>();
  const [selectedVersion, setSelectedVersion] = useState<string>();
  const isMobile = useIsMobile();
  const [panelOpen, setPanelOpen] = useState(true);
  const [mobilePanelOpen, setMobilePanelOpen] = useState(false);
  const [panelWidth, setPanelWidth] = useState(DEFAULT_PANEL_WIDTH);
  const [showFullscreenHint, setShowFullscreenHint] = useState(false);
  const [editing, setEditing] = useState(false);
  const [editReady, setEditReady] = useState(false);
  const [editDirty, setEditDirty] = useState(false);
  const [editRequest, setEditRequest] = useState(0);
  const [awaitingSerialization, setAwaitingSerialization] = useState(false);
  const [saveDialogOpen, setSaveDialogOpen] = useState(false);
  const [changeTitle, setChangeTitle] = useState("");
  const [changeSummary, setChangeSummary] = useState("");
  const [saveError, setSaveError] = useState<string>();
  useTitle(doc.data ? docTitle(doc.data) : undefined);

  const handleDocCommentClick = useCallback(
    (commentId: string) => {
      setActiveComment(commentId);
      if (fullscreen) return;
      if (isMobile) setMobilePanelOpen(true);
      else setPanelOpen(true);
    },
    [fullscreen, isMobile],
  );

  // On mobile, a text selection from the rendered doc should pop the
  // comments sheet open so the compose flow is reachable.
  useEffect(() => {
    if (isMobile && !fullscreen && selection?.quote) setMobilePanelOpen(true);
  }, [isMobile, fullscreen, selection]);

  const toggleComments = useCallback(() => {
    if (isMobile) setMobilePanelOpen((o) => !o);
    else setPanelOpen((o) => !o);
  }, [isMobile]);

  // On mobile the comments sheet should be dismissable with the browser/OS
  // Back gesture, without leaving the document. We push a dedicated history
  // entry when it opens; Back pops that entry (closing the sheet) instead of
  // navigating away. Closing via the UI removes the entry we added.
  const sheetHistoryRef = useRef(false);
  useEffect(() => {
    if (!mobilePanelOpen) return;
    window.history.pushState({ aidocsCommentsSheet: true }, "");
    sheetHistoryRef.current = true;
    const onPop = () => {
      sheetHistoryRef.current = false;
      setMobilePanelOpen(false);
    };
    window.addEventListener("popstate", onPop);
    return () => {
      window.removeEventListener("popstate", onPop);
      // If the sheet closed via the UI (not via Back), remove the entry we
      // pushed so the next Back returns to the documents list as expected.
      if (sheetHistoryRef.current) {
        sheetHistoryRef.current = false;
        window.history.back();
      }
    };
  }, [mobilePanelOpen]);

  // Radix Dialog's scroll-lock (react-remove-scroll) can leave the body
  // with `overflow: hidden` / `pointer-events: none` after the mobile
  // comments sheet closes, which freezes document scrolling. Restore the
  // body once the sheet is fully closed. (`modal={false}` on the Sheet
  // would avoid the lock, but Radix then skips the overlay and outside
  // dismissal entirely, so we keep modal behavior and clean up after it.)
  useEffect(() => {
    if (mobilePanelOpen) return;
    const restore = () => {
      for (const el of [document.body, document.documentElement]) {
        el.style.removeProperty("overflow");
        el.style.removeProperty("padding-right");
        el.style.removeProperty("pointer-events");
        el.removeAttribute("data-scroll-locked");
      }
    };
    // Run after Radix's own close cleanup on the next frame.
    const id = window.requestAnimationFrame(restore);
    return () => window.cancelAnimationFrame(id);
  }, [mobilePanelOpen]);

  const versionItems = versions.data?.items || [];
  const latest =
    currentVersionID(doc.data || {}) ||
    versionID(versionItems[versionItems.length - 1] || {});
  const current = selectedVersion || queryVersion || latest;
  const requiresReview =
    !!effectiveRepositoryId && repository.data?.protected !== false;

  const saveTextEdit = useMutation({
    mutationFn: async ({
      html,
      title,
      summary,
    }: {
      html: string;
      title: string;
      summary: string;
    }) => {
      const file = new File([html], `${id || "document"}.html`, {
        type: "text/html;charset=utf-8",
      });
      const normalizedTitle = title.trim() || `Update ${docTitle(doc.data || {})}`;
      const normalizedSummary = summary.trim() || "Edited document text";
      const openPullRequest = async (targetRepositoryId: string) => ({
        kind: "pull_request" as const,
        result: await api.createPullRequest(targetRepositoryId, {
          document_id: id,
          base_version_id: latest || current,
          title: normalizedTitle,
          body: summary.trim(),
          change_summary: normalizedSummary,
          file,
        }),
      });

      if (requiresReview && effectiveRepositoryId) {
        return openPullRequest(effectiveRepositoryId);
      }
      try {
        return {
          kind: "version" as const,
          result: await api.createVersion(
            id,
            latest || current,
            normalizedSummary,
            file,
          ),
        };
      } catch (error) {
        // The server remains authoritative. If protection changed while the
        // editor was open, transparently turn the same text edit into a PR.
        if (error instanceof APIError && error.code === "pull_request_required") {
          const details =
            error.details && typeof error.details === "object"
              ? (error.details as Record<string, unknown>)
              : undefined;
          const protectedRepositoryId =
            typeof details?.repository_id === "string"
              ? details.repository_id
              : effectiveRepositoryId;
          if (protectedRepositoryId) return openPullRequest(protectedRepositoryId);
        }
        throw error;
      }
    },
    onSuccess: async (saved) => {
      setSaveDialogOpen(false);
      setAwaitingSerialization(false);
      setEditing(false);
      setEditDirty(false);
      setEditReady(false);
      setSaveError(undefined);
      setChangeSummary("");
      if (saved.kind === "pull_request") {
        toast.success(`Pull request #${saved.result.number} opened.`);
        navigate(
          `/app/repos/${saved.result.repository_id}/pulls/${saved.result.number}`,
        );
        return;
      }
      await Promise.all([
        queryClient.invalidateQueries({ queryKey: queryKeys.document(id) }),
        queryClient.invalidateQueries({ queryKey: queryKeys.versions(id) }),
        queryClient.invalidateQueries({ queryKey: queryKeys.comments(id) }),
      ]);
      setSelectedVersion(undefined);
      toast.success("Document text updated.");
    },
    onError: (error) => {
      setAwaitingSerialization(false);
      setSaveError(errorMessage(error, "Could not save document changes"));
    },
  });

  const beginEditing = useCallback(() => {
    setSelectedVersion(undefined);
    setSelection(null);
    setActiveComment(undefined);
    setPanelOpen(false);
    setMobilePanelOpen(false);
    setEditDirty(false);
    setEditReady(false);
    setSaveError(undefined);
    setChangeTitle(`Update ${docTitle(doc.data || {})}`);
    setChangeSummary("");
    setEditing(true);
  }, [doc.data]);

  const cancelEditing = useCallback(() => {
    if (
      editDirty &&
      !window.confirm("Discard the text changes you have made to this document?")
    ) {
      return;
    }
    setSaveDialogOpen(false);
    setAwaitingSerialization(false);
    setEditing(false);
    setEditDirty(false);
    setEditReady(false);
    setSaveError(undefined);
  }, [editDirty]);

  const openSaveDialog = useCallback(() => {
    if (!editDirty || saveTextEdit.isPending || awaitingSerialization) return;
    setSaveError(undefined);
    setSaveDialogOpen(true);
  }, [editDirty, saveTextEdit.isPending, awaitingSerialization]);

  const requestEditedHTML = useCallback(() => {
    setSaveError(undefined);
    setAwaitingSerialization(true);
    setEditRequest((request) => request + 1);
  }, []);

  const handleEditedHTML = useCallback(
    (html: string) => {
      if (!awaitingSerialization || saveTextEdit.isPending) return;
      setAwaitingSerialization(false);
      saveTextEdit.mutate({
        html,
        title: changeTitle,
        summary: changeSummary,
      });
    },
    [awaitingSerialization, saveTextEdit, changeTitle, changeSummary],
  );

  const handleEditError = useCallback((message: string) => {
    setAwaitingSerialization(false);
    setSaveError(message);
  }, []);

  useEffect(() => {
    if (!editing || !editDirty) return;
    const warn = (event: BeforeUnloadEvent) => {
      event.preventDefault();
    };
    window.addEventListener("beforeunload", warn);
    return () => window.removeEventListener("beforeunload", warn);
  }, [editing, editDirty]);

  useEffect(() => {
    if (!editing) return;
    const onKeyDown = (event: KeyboardEvent) => {
      if (event.key === "Escape" && !saveDialogOpen) cancelEditing();
      if (
        (event.ctrlKey || event.metaKey) &&
        !event.altKey &&
        event.key.toLowerCase() === "s"
      ) {
        event.preventDefault();
        openSaveDialog();
      }
    };
    window.addEventListener("keydown", onKeyDown);
    return () => window.removeEventListener("keydown", onKeyDown);
  }, [editing, saveDialogOpen, cancelEditing, openSaveDialog]);

  const exitFullscreen = useCallback(() => {
    setSearchParams(
      (params) => {
        const next = new URLSearchParams(params);
        next.delete("fullscreen");
        return next;
      },
      { replace: true },
    );
  }, [setSearchParams]);

  useEffect(() => {
    if (!fullscreen) return;
    setShowFullscreenHint(true);
    const timeout = window.setTimeout(() => setShowFullscreenHint(false), 4000);
    const onKeyDown = (event: KeyboardEvent) => {
      if (event.key === "Escape") exitFullscreen();
    };
    window.addEventListener("keydown", onKeyDown);
    return () => {
      window.clearTimeout(timeout);
      window.removeEventListener("keydown", onKeyDown);
    };
  }, [fullscreen, exitFullscreen]);

  const ctx = useMemo(
    () => ({
      docId: id,
      version: current,
      comments: comments.data?.items || [],
      commentsLoading: comments.isLoading,
      commentsError: !!comments.error,
      principal: me.data?.principal,
      selection,
      setSelection,
      activeComment,
      setActiveComment,
      handleDocCommentClick,
    }),
    [
      id,
      current,
      comments.data,
      comments.isLoading,
      comments.error,
      me.data,
      selection,
      activeComment,
      setActiveComment,
      handleDocCommentClick,
    ],
  );

  const onResizePointerDown = (e: React.PointerEvent<HTMLDivElement>) => {
    e.preventDefault();
    const el = e.currentTarget;
    el.setPointerCapture(e.pointerId);
    const startX = e.clientX;
    const startWidth = panelWidth;

    const onMove = (ev: PointerEvent) => {
      const delta = startX - ev.clientX;
      setPanelWidth(
        Math.min(MAX_PANEL_WIDTH, Math.max(MIN_PANEL_WIDTH, startWidth + delta)),
      );
    };

    const onUp = (ev: PointerEvent) => {
      el.releasePointerCapture(ev.pointerId);
      el.removeEventListener("pointermove", onMove);
      el.removeEventListener("pointerup", onUp);
    };

    el.addEventListener("pointermove", onMove);
    el.addEventListener("pointerup", onUp);
  };

  if (doc.isLoading) {
    return (
      <div className="p-6">
        <Skeleton className="mb-3 h-7 w-72" />
        <Skeleton className="h-[60vh] w-full" />
      </div>
    );
  }
  if (doc.error) {
    if (doc.error instanceof APIError) {
      if (doc.error.status === 403) {
        return (
          <DocumentErrorState
            title="You don't have access to this document"
            description="Please ask the document owner to grant you access."
          />
        );
      }
      if (doc.error.status === 404) {
        return (
          <DocumentErrorState
            title="Document not found"
            description="This document may have been deleted, or the link may be incorrect."
          />
        );
      }
    }
    return (
      <DocumentErrorState
        title="Could not load document"
        description="Something went wrong while loading this document. Please try again."
      />
    );
  }
  if (!doc.data) return null;

  return (
    <DocProvider value={ctx}>
      {fullscreen ? (
        <div className="relative h-full bg-[var(--color-surface-muted)]">
          {showFullscreenHint ? (
            <div className="pointer-events-none absolute left-1/2 top-6 z-20 -translate-x-1/2 rounded-full bg-[var(--color-surface)] px-6 py-3 text-base font-semibold text-[var(--color-fg)] shadow-xl ring-1 ring-[var(--color-border)]">
              <span className="hidden sm:inline">Press Esc to exit full screen</span>
              <span className="sm:hidden">Go back to exit full screen</span>
            </div>
          ) : null}
          {current ? <RenderFrame /> : <Center>No version available.</Center>}
        </div>
      ) : (
        <motion.div
          className="flex h-full flex-col"
          initial="hidden"
          animate="visible"
          variants={pageStagger}
        >
          <motion.div variants={barSlide}>
            <DocBar
              title={docTitle(doc.data)}
              docId={id}
              owner={doc.data.owner}
              principal={me.data?.principal}
              parentFolder={doc.data.parent_folder}
              repositoryId={effectiveRepositoryId || undefined}
              current={current}
              latest={latest}
              versions={versions.data?.items || []}
              onSelectVersion={setSelectedVersion}
              commentsOpen={isMobile ? mobilePanelOpen : panelOpen}
              onToggleComments={toggleComments}
              editing={editing}
              editDirty={editDirty}
              editReady={editReady}
              savingEdit={awaitingSerialization || saveTextEdit.isPending}
              onStartEditing={beginEditing}
              onSaveEditing={openSaveDialog}
              onCancelEditing={cancelEditing}
            />
          </motion.div>
          <motion.div variants={contentFade} className="flex flex-1 overflow-hidden">
            <section className="relative flex-1 bg-[var(--color-surface-muted)]">
              {current ? (
                <RenderFrame
                  editing={editing}
                  editRequest={editRequest}
                  onEditReadyChange={setEditReady}
                  onEditDirtyChange={setEditDirty}
                  onEditedHTML={handleEditedHTML}
                  onEditError={handleEditError}
                  onEditSaveShortcut={openSaveDialog}
                />
              ) : (
                <Center>No version available.</Center>
              )}
              {editing ? (
                <div className="pointer-events-none absolute bottom-4 left-1/2 z-10 -translate-x-1/2 rounded-full border border-[var(--color-accent)]/40 bg-[var(--color-surface)]/95 px-4 py-2 text-xs font-medium text-[var(--color-fg-muted)] shadow-lg backdrop-blur">
                  Click any text to edit it · ⌘S to save
                </div>
              ) : null}
            </section>
            <AnimatePresence initial={false}>
              {panelOpen && !editing && (
                <motion.aside
                  key="comments-panel"
                  initial={{ width: 0 }}
                  animate={{ width: panelWidth }}
                  exit={{ width: 0 }}
                  transition={{ type: "spring", duration: 0.35, bounce: 0 }}
                  className="relative hidden shrink-0 overflow-hidden border-l border-[var(--color-border)] bg-[var(--color-surface)] md:block"
                >
                  <div className="flex h-full flex-col" style={{ width: panelWidth }}>
                    <div
                      className="absolute inset-y-0 left-0 z-10 w-1 cursor-col-resize transition-colors hover:bg-[var(--color-border-strong)]/50"
                      onPointerDown={onResizePointerDown}
                    />
                    <Comments />
                  </div>
                </motion.aside>
              )}
            </AnimatePresence>
          </motion.div>

          {/* Mobile: comments live in a slide-over sheet */}
          <Sheet open={mobilePanelOpen} onOpenChange={setMobilePanelOpen}>
            <SheetContent
              side="right"
              className="w-full max-w-md p-0 md:hidden"
              aria-describedby={undefined}
            >
              <SheetHeader className="sr-only">
                <SheetTitle>Comments</SheetTitle>
              </SheetHeader>
              <Comments />
            </SheetContent>
          </Sheet>
          <SaveTextChangesDialog
            open={saveDialogOpen}
            onOpenChange={(open) => {
              if (saveTextEdit.isPending || awaitingSerialization) return;
              setSaveDialogOpen(open);
            }}
            requiresReview={requiresReview}
            title={changeTitle}
            onTitleChange={setChangeTitle}
            summary={changeSummary}
            onSummaryChange={setChangeSummary}
            pending={awaitingSerialization || saveTextEdit.isPending}
            error={saveError}
            onSubmit={requestEditedHTML}
          />
        </motion.div>
      )}
    </DocProvider>
  );
}

function SaveTextChangesDialog({
  open,
  onOpenChange,
  requiresReview,
  title,
  onTitleChange,
  summary,
  onSummaryChange,
  pending,
  error,
  onSubmit,
}: {
  open: boolean;
  onOpenChange: (open: boolean) => void;
  requiresReview: boolean;
  title: string;
  onTitleChange: (title: string) => void;
  summary: string;
  onSummaryChange: (summary: string) => void;
  pending: boolean;
  error?: string;
  onSubmit: () => void;
}) {
  return (
    <Dialog open={open} onOpenChange={onOpenChange}>
      <DialogContent>
        <DialogHeader>
          <DialogTitle>
            {requiresReview ? "Propose text changes" : "Save text changes"}
          </DialogTitle>
          <DialogDescription>
            {requiresReview
              ? "This document is protected. Your edits will open a pull request for review."
              : "Your edits will be saved as a new document version."}
          </DialogDescription>
        </DialogHeader>
        <form
          onSubmit={(event) => {
            event.preventDefault();
            onSubmit();
          }}
        >
          <div className="space-y-4">
            {requiresReview ? (
              <div className="space-y-1.5">
                <Label htmlFor="text-change-title">Change title</Label>
                <Input
                  id="text-change-title"
                  value={title}
                  onChange={(event) => onTitleChange(event.target.value)}
                  placeholder="Summarize the proposed change"
                  autoFocus
                />
              </div>
            ) : null}
            <div className="space-y-1.5">
              <Label htmlFor="text-change-summary">Summary</Label>
              <Textarea
                id="text-change-summary"
                value={summary}
                onChange={(event) => onSummaryChange(event.target.value)}
                placeholder="What did you change, and why?"
                rows={4}
                autoFocus={!requiresReview}
              />
            </div>
            {error ? (
              <p role="alert" className="text-sm text-[var(--color-danger)]">
                {error}
              </p>
            ) : null}
          </div>
          <DialogFooter>
            <Button
              type="button"
              variant="outline"
              disabled={pending}
              onClick={() => onOpenChange(false)}
            >
              Keep editing
            </Button>
            <Button
              type="submit"
              disabled={pending || (requiresReview && !title.trim())}
            >
              {pending
                ? "Preparing changes…"
                : requiresReview
                  ? "Create pull request"
                  : "Save new version"}
            </Button>
          </DialogFooter>
        </form>
      </DialogContent>
    </Dialog>
  );
}
