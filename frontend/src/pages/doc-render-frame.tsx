import { useCallback, useEffect, useMemo, useRef } from "react";
import { useQuery } from "@tanstack/react-query";
import { api, type Anchor } from "@/api";
import { queryKeys } from "@/lib/queryKeys";
import { COMMENT_STATUS } from "@/lib/constants";
import { Center } from "@/components/ui/misc";
import { useResolvedTheme } from "@/lib/theme";
import { useDoc } from "./doc-context";

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
  const readyRef = useRef(false);
  const theme = useResolvedTheme();

  const token = useQuery({
    queryKey: queryKeys.render(version),
    queryFn: () => api.renderToken(docId, version),
    staleTime: 4 * 60_000,
    enabled: !!version,
  });

  const paintPayload = useMemo(
    () =>
      comments
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

  useEffect(() => {
    readyRef.current = false;
  }, [frameOrigin, token.data?.url]);

  const frameSrc = useMemo(() => {
    const url = token.data?.url;
    if (!url) return undefined;
    const sep = url.includes("?") ? "&" : "?";
    return `${url}${sep}aidocs_theme=${theme}`;
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [token.data?.url]);

  const paint = useCallback(() => {
    if (!readyRef.current || !frameOrigin) return;
    ref.current?.contentWindow?.postMessage(
      { type: "aidocs:paint", comments: paintPayload, active: activeComment },
      frameOrigin,
    );
  }, [paintPayload, activeComment, frameOrigin]);

  useEffect(() => {
    if (!readyRef.current || !frameOrigin) return;
    ref.current?.contentWindow?.postMessage(
      { type: "aidocs:theme", theme },
      frameOrigin,
    );
  }, [theme, frameOrigin]);

  useEffect(() => {
    const onMsg = (e: MessageEvent) => {
      if (!frameOrigin || e.origin !== frameOrigin) return;
      if (e.data?.type === "aidocs:selection") {
        const anchor = e.data.anchor as Partial<Anchor> | undefined;
        const quote = anchor?.quote ?? e.data.quote;
        if (quote) setSelection({ quote, anchor });
      }
      if (e.data?.type === "aidocs:activate" && typeof e.data.id === "string") {
        handleDocCommentClick(e.data.id);
      }
      if (e.data?.type === "aidocs:ready") {
        readyRef.current = true;
        paint();
        ref.current?.contentWindow?.postMessage(
          { type: "aidocs:theme", theme },
          frameOrigin,
        );
      }
    };
    window.addEventListener("message", onMsg);
    return () => window.removeEventListener("message", onMsg);
  }, [frameOrigin, paint, setSelection, handleDocCommentClick, theme]);

  useEffect(() => {
    paint();
  }, [paint]);

  if (token.isLoading) return <Center>Preparing secure render…</Center>;
  if (token.error) return <Center>Could not create render token.</Center>;
  return (
    <iframe
      ref={ref}
      title="Rendered document"
      style={{ height: "100%", width: "100%", background: theme === "dark" ? "#0b0d12" : "#fff" }}
      src={frameSrc}
      sandbox="allow-scripts allow-same-origin allow-popups allow-popups-to-escape-sandbox"
    />
  );
}
