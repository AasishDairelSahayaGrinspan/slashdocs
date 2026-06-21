import { useCallback, useEffect, useMemo, useRef } from "react";
import { useQuery } from "@tanstack/react-query";
import { api, type Anchor, type Document } from "@/api";
import { queryKeys } from "@/lib/queryKeys";
import { COMMENT_STATUS } from "@/lib/constants";
import { Center } from "@/components/ui/misc";
import { useResolvedTheme } from "@/lib/theme";
import { useConfig } from "@/lib/useConfig";
import {
  DriveAccessError,
  fetchDriveFile,
  getDriveAccessToken,
} from "@/lib/drive";
import { useDoc } from "./doc-context";

// Renders the sandboxed document iframe and exchanges selection and paint
// messages with the in-iframe bridge. Message origins are validated on both
// ends, and painting is driven by the bridge's ready signal.
//
// Native docs load a backend render-token URL into the iframe `src`. Drive
// docs have no version/blob: the browser fetches the file HTML live from
// Drive, POSTs it to /v1/drive/render for sandbox wrapping, and sets the
// wrapper as the iframe `srcdoc`. The postMessage bridge wiring is identical
// for both paths; only `src` vs `srcdoc` and the validated `frameOrigin`
// differ (Drive srcdoc content shares the app origin, so frameOrigin is the
// app origin rather than a token URL origin).
export function RenderFrame({ doc }: { doc: Document }) {
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
  const isDrive = doc.source === "drive";

  const cfg = useConfig();

  const token = useQuery({
    queryKey: queryKeys.render(version),
    queryFn: () => api.renderToken(docId, version),
    staleTime: 4 * 60_000,
    enabled: !!version && !isDrive,
  });

  // Drive docs have no version/blob: fetch the file HTML client-side, then
  // wrap it through the backend sandbox wrapper. Enabled only for drive docs
  // once the OAuth client id is known.
  const driveQuery = useQuery({
    queryKey: ["drive-render", doc.id],
    enabled: isDrive && !!cfg.data?.google_oauth_client_id,
    staleTime: 60_000,
    // Drive errors (403/404, GIS popup dismissal) are deterministic; retrying
    // only delays the message and can re-pop the consent popup.
    retry: false,
    queryFn: async () => {
      const accessToken = await getDriveAccessToken(
        cfg.data!.google_oauth_client_id,
      );
      const html = await fetchDriveFile(
        doc.drive_file_id!,
        accessToken,
        doc.drive_resource_key,
      );
      return api.driveRender(doc.id, html);
    },
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

  // For native docs the frame origin is the render-token URL's origin. For
  // Drive srcdoc content the origin is the embedding app origin (sandbox
  // allows same-origin), so validate messages against window.location.origin.
  const frameOrigin = useMemo(() => {
    if (isDrive) return window.location.origin;
    if (!token.data?.url) return null;
    try {
      return new URL(token.data.url, window.location.href).origin;
    } catch {
      return null;
    }
  }, [isDrive, token.data]);

  // Reset readiness whenever we load a different frame.
  useEffect(() => {
    readyRef.current = false;
  }, [frameOrigin, token.data?.url, driveQuery.data]);

  // The resolved theme is baked into the iframe URL so the bridge can apply
  // it before the document's first paint (no flash); live toggles are pushed
  // over postMessage below. (Drive srcdoc has no URL query to bake into.)
  const frameSrc = useMemo(() => {
    const url = token.data?.url;
    if (!url) return undefined;
    const sep = url.includes("?") ? "&" : "?";
    return `${url}${sep}aidocs_theme=${theme}`;
    // theme is intentionally excluded: changing it must not reload the frame.
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [token.data?.url]);

  const paint = useCallback(() => {
    if (!readyRef.current || !frameOrigin) return;
    ref.current?.contentWindow?.postMessage(
      { type: "aidocs:paint", comments: paintPayload, active: activeComment },
      frameOrigin,
    );
  }, [paintPayload, activeComment, frameOrigin]);

  // Push live theme changes to the bridge once the frame is ready.
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

  // Push fresh paint data once the frame is ready and whenever it changes.
  useEffect(() => {
    paint();
  }, [paint]);

  if (isDrive) {
    // isPending (not isLoading) so the placeholder also shows while the query
    // is disabled waiting on config — isLoading is false when disabled, which
    // would otherwise render a blank iframe (srcDoc=undefined).
    if (driveQuery.isPending) return <Center>Loading from Google Drive…</Center>;
    if (driveQuery.error instanceof DriveAccessError) {
      if (driveQuery.error.status === 403)
        return <Center>You don't have access to this file in Google Drive.</Center>;
      if (driveQuery.error.status === 404)
        return <Center>This file no longer exists in Drive.</Center>;
    }
    if (driveQuery.error)
      return <Center>Could not render this Drive document.</Center>;
    return (
      <iframe
        ref={ref}
        title="Rendered document"
        className="h-full w-full"
        style={{ background: theme === "dark" ? "#0b0d12" : "#fff" }}
        srcDoc={driveQuery.data}
        sandbox="allow-scripts allow-same-origin allow-popups allow-popups-to-escape-sandbox"
      />
    );
  }

  // The parent (DocPage) only mounts RenderFrame for native docs when a
  // current version exists, so version is always set on this path.
  if (token.isLoading) return <Center>Preparing secure render…</Center>;
  if (token.error) return <Center>Could not create render token.</Center>;
  return (
    <iframe
      ref={ref}
      title="Rendered document"
      // Match the frame to the resolved theme so dark readers don't see a
      // white flash before the document paints.
      className="h-full w-full"
      style={{ background: theme === "dark" ? "#0b0d12" : "#fff" }}
      src={frameSrc}
      sandbox="allow-scripts allow-same-origin allow-popups allow-popups-to-escape-sandbox"
    />
  );
}
