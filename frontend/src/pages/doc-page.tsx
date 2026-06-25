import { useTitle } from "@/lib/use-title";
import { useCallback, useMemo, useState } from "react";
import { useParams } from "react-router-dom";
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
      <div style={{ maxWidth: "448px", textAlign: "center" }}>
        <h1 style={{ fontSize: "18px", fontWeight: 600, color: "#18181b" }}>{title}</h1>
        <p style={{ marginTop: "8px", fontSize: "14px", color: "#71717a" }}>{description}</p>
      </div>
    </Center>
  );
}

export function DocumentPage() {
  const { id = "" } = useParams();
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
  useTitle(doc.data ? docTitle(doc.data) : undefined);

  const handleDocCommentClick = useCallback((commentId: string) => {
    setActiveComment(commentId);
    setPanelOpen(true);
  }, []);

  const versionItems = versions.data?.items || [];
  const latest =
    currentVersionID(doc.data || {}) ||
    versionID(versionItems[versionItems.length - 1] || {});
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
      <div style={{ padding: "24px" }}>
        <Skeleton style={{ marginBottom: "12px", height: "28px", width: "288px" }} />
        <Skeleton style={{ height: "60vh", width: "100%" }} />
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
      <div style={{ display: "flex", height: "100%", flexDirection: "column" }}>
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
        <div style={{ display: "flex", flex: 1, overflow: "hidden" }}>
          <section style={{ position: "relative", flex: 1, background: "#f4f4f5" }}>
            {current ? <RenderFrame /> : <Center>No version available.</Center>}
          </section>
          {panelOpen && (
            <aside
              style={{
                position: "relative",
                display: "flex",
                flexDirection: "column",
                flexShrink: 0,
                borderLeft: "1px solid #e4e4e7",
                background: "#fff",
                width: panelWidth,
              }}
            >
              <div
                style={{
                  position: "absolute",
                  top: 0,
                  bottom: 0,
                  left: 0,
                  zIndex: 10,
                  width: "4px",
                  cursor: "col-resize",
                  transition: "background-color 0.15s",
                }}
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
