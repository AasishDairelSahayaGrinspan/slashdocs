import { useTitle } from "@/lib/use-title";
import { useMemo, useState } from "react";
import { useParams } from "react-router-dom";
import { useQuery } from "@tanstack/react-query";
import { api, currentVersionID, docTitle, versionID } from "@/api";
import { queryKeys } from "@/lib/queryKeys";
import { Center, Skeleton } from "@/components/ui/misc";
import { DocProvider, type PendingSelection } from "./doc-context";
import { DocBar } from "./doc-bar";
import { RenderFrame } from "./doc-render-frame";
import { Comments } from "./doc-comments";

const MIN_PANEL_WIDTH = 240;
const MAX_PANEL_WIDTH = 640;
const DEFAULT_PANEL_WIDTH = 380;

export function DocumentPage() {
  const { id = "" } = useParams();
  const doc = useQuery({
    queryKey: queryKeys.document(id),
    queryFn: () => api.getDocument(id),
  });
  const me = useQuery({ queryKey: queryKeys.me(), queryFn: api.me });
  const versions = useQuery({
    queryKey: queryKeys.versions(id),
    queryFn: () => api.listVersions(id),
  });
  // One comments query feeds both the paint overlay and the side panel so they
  // stay in sync.
  const comments = useQuery({
    queryKey: queryKeys.comments(id),
    queryFn: () => api.listComments(id),
    enabled: !!id,
  });
  const [selection, setSelection] = useState<PendingSelection | null>(null);
  const [activeComment, setActiveComment] = useState<string>();
  const [selectedVersion, setSelectedVersion] = useState<string>();
  const [panelOpen, setPanelOpen] = useState(true);
  const [panelWidth, setPanelWidth] = useState(DEFAULT_PANEL_WIDTH);
  useTitle(doc.data ? docTitle(doc.data) : undefined);

  const versionItems = versions.data?.items || [];
  const latest =
    currentVersionID(doc.data || {}) || versionID(versionItems[versionItems.length - 1] || {});
  const current = selectedVersion || latest;

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
      setPanelWidth(Math.min(MAX_PANEL_WIDTH, Math.max(MIN_PANEL_WIDTH, startWidth + delta)));
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
  if (doc.error) return <Center>Could not load document.</Center>;
  if (!doc.data) return null;

  return (
    <DocProvider value={ctx}>
      <div className="flex h-full flex-col">
        <DocBar
          title={docTitle(doc.data)}
          docId={id}
          owner={doc.data.owner}
          principal={me.data?.principal}
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
    </DocProvider>
  );
}
