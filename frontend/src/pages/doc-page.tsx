import { useTitle } from "@/lib/use-title";
import { useCallback, useEffect, useMemo, useState } from "react";
import { useSearchParams, useParams } from "react-router-dom";
import { useQuery } from "@tanstack/react-query";
import { api, APIError, currentVersionID, docTitle, versionID } from "@/api";
import { queryKeys } from "@/lib/queryKeys";
import { Center, Skeleton } from "@/components/ui/misc";
import { DocProvider, type PendingSelection } from "./doc-context";
import { DocBar } from "./doc-bar";
import { RenderFrame } from "./doc-render-frame";
import { Comments } from "./doc-comments";

const MIN_PANEL_WIDTH = 240;
const MAX_PANEL_WIDTH = 640;
const DEFAULT_PANEL_WIDTH = 380;

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
  const [searchParams, setSearchParams] = useSearchParams();
  const fullscreen = searchParams.get("fullscreen") === "1";
  const queryVersion = searchParams.get("version") || undefined;
  const doc = useQuery({
    queryKey: queryKeys.document(id),
    queryFn: () => api.getDocument(id),
    meta: { suppressGlobalError: true },
  });
  const me = useQuery({ queryKey: queryKeys.me(), queryFn: api.me });
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
  const [panelOpen, setPanelOpen] = useState(true);
  const [panelWidth, setPanelWidth] = useState(DEFAULT_PANEL_WIDTH);
  const [showFullscreenHint, setShowFullscreenHint] = useState(false);
  useTitle(doc.data ? docTitle(doc.data) : undefined);

  const handleDocCommentClick = useCallback((commentId: string) => {
    setActiveComment(commentId);
    if (!fullscreen) setPanelOpen(true);
  }, [fullscreen]);

  const versionItems = versions.data?.items || [];
  const latest =
    currentVersionID(doc.data || {}) ||
    versionID(versionItems[versionItems.length - 1] || {});
  const current = selectedVersion || queryVersion || latest;

  const exitFullscreen = useCallback(() => {
    setSearchParams((params) => {
      const next = new URLSearchParams(params);
      next.delete("fullscreen");
      return next;
    }, { replace: true });
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
              Press Esc to exit full screen
            </div>
          ) : null}
          {current ? <RenderFrame /> : <Center>No version available.</Center>}
        </div>
      ) : (
      <div className="flex h-full flex-col">
        <DocBar
          title={docTitle(doc.data)}
          docId={id}
          owner={doc.data.owner}
          principal={me.data?.principal}
          parentFolder={doc.data.parent_folder}
          current={current}
          latest={latest}
          versions={versions.data?.items || []}
          onSelectVersion={setSelectedVersion}
          commentsOpen={panelOpen}
          onToggleComments={() => setPanelOpen((o) => !o)}
        />
        <div className="flex flex-1 overflow-hidden">
          <section className="relative flex-1 bg-[var(--color-surface-muted)]">
            {current ? <RenderFrame /> : <Center>No version available.</Center>}
          </section>
          {panelOpen && (
            <aside
              className="relative hidden shrink-0 flex-col border-l border-[var(--color-border)] bg-[var(--color-surface)] md:flex"
              style={{ width: panelWidth }}
            >
              <div
                className="absolute inset-y-0 left-0 z-10 w-1 cursor-col-resize transition-colors hover:bg-[var(--color-border-strong)]/50"
                onPointerDown={onResizePointerDown}
              />
              <Comments />
            </aside>
          )}
        </div>
      </div>
      )}
    </DocProvider>
  );
}
