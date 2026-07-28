import { useCallback, useEffect, useMemo, useRef, useState } from "react";
import { useQuery } from "@tanstack/react-query";
import { toast } from "sonner";
import { api, APIError, type Anchor, type DocumentKVEvent } from "@/api";
import { queryKeys } from "@/lib/queryKeys";
import { COMMENT_STATUS } from "@/lib/constants";
import { Center } from "@/components/ui/misc";
import { useResolvedTheme } from "@/lib/theme";
import { errorMessage } from "@/lib/errors";
import { useDoc } from "./doc-context";

// Renders the sandboxed document iframe and exchanges selection and paint
// messages with the in-iframe bridge. Message origins are validated on both
// ends, and painting is driven by the bridge's ready signal.
export function RenderFrame() {
  const {
    docId,
    version,
    comments,
    activeComment,
    setSelection,
    handleDocCommentClick,
  } = useDoc();
  const ref = useRef<HTMLIFrameElement>(null);
  const [ready, setReady] = useState(false);
  const theme = useResolvedTheme();
  // Latest resolved theme, readable from stable event handlers without
  // forcing them to re-register (which caused a race where aidocs:ready
  // could arrive during listener teardown and be missed).
  const themeRef = useRef(theme);
  themeRef.current = theme;
  const kvRevisionRef = useRef(0);

  useEffect(() => {
    kvRevisionRef.current = 0;
  }, [docId]);

  const token = useQuery({
    queryKey: queryKeys.render(version),
    queryFn: () => api.renderToken(docId, version),
    staleTime: 4 * 60_000,
    enabled: !!version,
  });

  const paintPayload = useMemo(
    () =>
      comments
        // Resolved comments keep their anchor server-side (in case a resolve
        // is reversed) but should not be highlighted in the rendered doc.
        .filter((c) => c.status !== COMMENT_STATUS.resolved)
        .map((c) => ({
          id: c.id,
          quote: c.selected_text || c.anchor?.quote,
          anchor: c.anchor,
        })),
    [comments],
  );

  const frameOrigin = useMemo(() => {
    if (!token.data?.url) return null;
    try {
      return new URL(token.data.url, window.location.href).origin;
    } catch {
      return null;
    }
  }, [token.data]);

  const rememberKVRevision = useCallback((revision: unknown) => {
    if (typeof revision !== "number" || !Number.isFinite(revision)) return;
    kvRevisionRef.current = Math.max(kvRevisionRef.current, revision);
  }, []);

  const postToCurrentFrame = useCallback(
    (message: unknown) => {
      if (!frameOrigin) return;
      ref.current?.contentWindow?.postMessage(message, frameOrigin);
    },
    [frameOrigin],
  );

  // Reset readiness whenever we load a different frame.
  useEffect(() => {
    setReady(false);
  }, [frameOrigin, token.data?.url]);

  // The resolved theme is baked into the iframe URL so the bridge can apply
  // it before the document's first paint (no flash); live toggles are pushed
  // over postMessage below.
  const frameSrc = useMemo(() => {
    const url = token.data?.url;
    if (!url) return undefined;
    const sep = url.includes("?") ? "&" : "?";
    return `${url}${sep}aidocs_theme=${theme}`;
    // theme is intentionally excluded: changing it must not reload the frame.
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [token.data?.url]);

  const paint = useCallback(() => {
    if (!ready || !frameOrigin) return;
    ref.current?.contentWindow?.postMessage(
      { type: "aidocs:paint", comments: paintPayload, active: activeComment },
      frameOrigin,
    );
  }, [ready, paintPayload, activeComment, frameOrigin]);

  // Push the theme to the bridge whenever it changes OR once the frame
  // becomes ready. Depending on `ready` (state, not a ref) guarantees this
  // re-runs after the frame signals ready, so a theme set while the frame
  // was still loading is always applied — fixing the intermittent wrong
  // theme and the "switch does nothing" cases.
  useEffect(() => {
    if (!ready || !frameOrigin) return;
    ref.current?.contentWindow?.postMessage(
      { type: "aidocs:theme", theme },
      frameOrigin,
    );
  }, [theme, ready, frameOrigin]);

  // ── Pinch-to-zoom: intercept trackpad pinch (wheel + ctrlKey) and apply
  // a CSS scale to the iframe instead of zooming the entire browser page.
  // Zoom is anchored to the cursor position (magnifying-glass feel).
  const MIN_SCALE = 0.25;
  const MAX_SCALE = 4;
  const wrapperRef = useRef<HTMLDivElement>(null);
  const [scale, setScale] = useState(1);
  const scaleRef = useRef(1);
  const cursorRef = useRef({ x: 0, y: 0 });

  const applyZoom = useCallback((deltaY: number, cursor?: { x: number; y: number }) => {
    const el = wrapperRef.current;
    if (!el) return;
    const oldScale = scaleRef.current;
    const newScale = Math.min(
      MAX_SCALE,
      Math.max(MIN_SCALE, oldScale * (1 - deltaY * 0.005)),
    );
    if (newScale === oldScale) return;

    const anchor = cursor ?? cursorRef.current;
    cursorRef.current = anchor;

    const rect = el.getBoundingClientRect();
    const cx = anchor.x - rect.left;
    const cy = anchor.y - rect.top;

    // Point in unscaled document space under cursor.
    const docX = (el.scrollLeft + cx) / oldScale;
    const docY = (el.scrollTop + cy) / oldScale;

    scaleRef.current = newScale;
    setScale(newScale);

    // After scale change, adjust scroll so the same doc point stays under cursor.
    requestAnimationFrame(() => {
      el.scrollLeft = docX * newScale - cx;
      el.scrollTop = docY * newScale - cy;
    });
  }, []);

  useEffect(() => {
    const el = wrapperRef.current;
    if (!el) return;
    const onWheel = (e: WheelEvent) => {
      if (!e.ctrlKey) return;
      e.preventDefault();
      applyZoom(e.deltaY, { x: e.clientX, y: e.clientY });
    };
    const onMouseMove = (e: MouseEvent) => {
      cursorRef.current = { x: e.clientX, y: e.clientY };
    };
    el.addEventListener("wheel", onWheel, { passive: false });
    el.addEventListener("mousemove", onMouseMove);
    return () => {
      el.removeEventListener("wheel", onWheel);
      el.removeEventListener("mousemove", onMouseMove);
    };
  }, [applyZoom]);

  useEffect(() => {
    const onMsg = (e: MessageEvent) => {
      if (!frameOrigin || e.origin !== frameOrigin) return;
      if (e.source !== ref.current?.contentWindow) return;
      const requestWindow = e.source;
      const postToFrame = (message: unknown) => {
        if (!requestWindow || ref.current?.contentWindow !== requestWindow) return;
        requestWindow.postMessage(message, frameOrigin);
      };
      const handleKVRequest = async (data: Record<string, unknown>) => {
        const requestId = String(data.requestId || "");
        const operation = String(data.operation || "");
        const sendError = (err: unknown) => {
          postToFrame({
            type: "aidocs:kv-response",
            requestId,
            ok: false,
            error:
              err instanceof APIError
                ? { message: err.message, code: err.code, status: err.status }
                : { message: errorMessage(err, "KV request failed") },
          });
        };
        try {
          if (operation === "all" || operation === "hydrate" || operation === "get") {
            const snapshot = await api.listDocumentKV(docId);
            rememberKVRevision(snapshot.revision);
            const result =
              operation === "get"
                ? {
                    revision: snapshot.revision,
                    value:
                      typeof data.key === "string"
                        ? snapshot.items[data.key]
                        : undefined,
                  }
                : snapshot;
            postToFrame({
              type: "aidocs:kv-response",
              requestId,
              ok: true,
              result,
            });
            return;
          }
          if (operation === "patch") {
            const set =
              data.set && typeof data.set === "object" && !Array.isArray(data.set)
                ? (data.set as Record<string, unknown>)
                : {};
            const deleteKeys = Array.isArray(data.delete)
              ? data.delete.filter((key): key is string => typeof key === "string")
              : [];
            const result = await api.patchDocumentKV(docId, {
              set,
              delete: deleteKeys,
            });
            const alreadyDelivered = result.revision <= kvRevisionRef.current;
            rememberKVRevision(result.revision);
            postToFrame({
              type: "aidocs:kv-response",
              requestId,
              ok: true,
              result,
            });
            if (!alreadyDelivered) {
              postToFrame({
                type: "aidocs:kv-event",
                detail: {
                  ...result,
                  source: String(data.source || "local"),
                  operation: String(data.kvOperation || "patch"),
                },
              });
            }
            return;
          }
          postToFrame({
            type: "aidocs:kv-response",
            requestId,
            ok: false,
            error: { message: "unknown KV operation" },
          });
        } catch (err) {
          sendError(err);
        }
      };
      if (e.data?.type === "aidocs:selection") {
        const anchor = e.data.anchor as Partial<Anchor> | undefined;
        const quote = anchor?.quote ?? e.data.quote;
        if (quote) setSelection({ quote, anchor });
      }
      if (e.data?.type === "aidocs:kv-request") {
        void handleKVRequest(e.data as Record<string, unknown>);
      }
      if (e.data?.type === "aidocs:activate" && typeof e.data.id === "string") {
        handleDocCommentClick(e.data.id);
      }
      if (e.data?.type === "aidocs:keydown" && e.data.key === "Escape") {
        window.dispatchEvent(new KeyboardEvent("keydown", { key: "Escape" }));
      }
      if (e.data?.type === "aidocs:zoom" && typeof e.data.deltaY === "number") {
        const frameRect = ref.current?.getBoundingClientRect();
        const cursor =
          frameRect &&
          typeof e.data.clientX === "number" &&
          typeof e.data.clientY === "number"
            ? {
                x: frameRect.left + e.data.clientX * scaleRef.current,
                y: frameRect.top + e.data.clientY * scaleRef.current,
              }
            : undefined;
        applyZoom(e.data.deltaY, cursor);
      }
      if (e.data?.type === "aidocs:ready") {
        setReady(true);
        ref.current?.contentWindow?.postMessage(
          { type: "aidocs:theme", theme: themeRef.current },
          frameOrigin,
        );
      }
    };
    window.addEventListener("message", onMsg);
    return () => window.removeEventListener("message", onMsg);
  }, [
    docId,
    frameOrigin,
    setSelection,
    handleDocCommentClick,
    applyZoom,
    rememberKVRevision,
  ]);

  useEffect(() => {
    if (!ready || !frameOrigin || !docId) return;
    const source = new EventSource(
      `/v1/documents/${encodeURIComponent(docId)}/kv/events?after_revision=${kvRevisionRef.current}`,
      { withCredentials: true },
    );
    source.onmessage = (event) => {
      let detail: DocumentKVEvent;
      try {
        detail = JSON.parse(event.data) as DocumentKVEvent;
      } catch {
        return;
      }
      if (!detail || typeof detail.revision !== "number") return;
      if (detail.revision <= kvRevisionRef.current) return;
      rememberKVRevision(detail.revision);
      postToCurrentFrame({
        type: "aidocs:kv-event",
        detail: {
          ...detail,
          source: "sync",
          operation: "sync",
        },
      });
    };
    return () => source.close();
  }, [docId, frameOrigin, ready, postToCurrentFrame, rememberKVRevision]);

  // Push fresh paint data once the frame is ready and whenever it changes.
  useEffect(() => {
    paint();
  }, [paint]);

  // Forward print requests from the DocBar dropdown to the bridge, which
  // calls print() on the inner document iframe — preserving the bridge
  // script, KV store, and all other integrations in the printed output.
  useEffect(() => {
    const onPrint = () => {
      if (!ready || !frameOrigin) {
        toast.error("Document is still loading. Please try again in a moment.");
        return;
      }
      ref.current?.contentWindow?.postMessage({ type: "aidocs:print" }, frameOrigin);
      toast.success('Print dialog opened — choose "Save as PDF" to export.');
    };
    window.addEventListener("aidocs:print", onPrint);
    return () => window.removeEventListener("aidocs:print", onPrint);
  }, [ready, frameOrigin]);

  // The parent (DocPage) only mounts RenderFrame when a current version
  // exists, so version is always set here.
  if (token.isLoading) return <Center>Preparing secure render…</Center>;
  if (token.error) return <Center>Could not create render token.</Center>;

  const showZoomBadge = Math.round(scale * 100) !== 100;

  return (
    <div ref={wrapperRef} className="relative h-full w-full overflow-auto">
      {/* Spacer creates the scrollable area at zoomed size */}
      <div
        style={{
          position: "relative",
          width: `${Math.max(100, scale * 100)}%`,
          height: `${Math.max(100, scale * 100)}%`,
        }}
      >
        <iframe
          ref={ref}
          title="Rendered document"
          style={{
            position: "absolute",
            top: 0,
            left: 0,
            width: `${100 / scale}%`,
            height: `${100 / scale}%`,
            transform: `scale(${scale})`,
            transformOrigin: "0 0",
            background: theme === "dark" ? "#0b0d12" : "#fff",
          }}
          src={frameSrc}
          sandbox="allow-scripts allow-same-origin allow-popups allow-popups-to-escape-sandbox"
        />
      </div>
      {showZoomBadge && (
        <button
          type="button"
          onClick={() => {
            scaleRef.current = 1;
            setScale(1);
          }}
          className="sticky bottom-3 left-[calc(100%-3.5rem)] z-10 rounded-full bg-[var(--color-surface)] px-2.5 py-1 text-xs font-medium text-[var(--color-fg-muted)] shadow-md ring-1 ring-[var(--color-border)] transition-opacity hover:text-[var(--color-fg)]"
          title="Reset zoom"
        >
          {Math.round(scale * 100)}%
        </button>
      )}
    </div>
  );
}
