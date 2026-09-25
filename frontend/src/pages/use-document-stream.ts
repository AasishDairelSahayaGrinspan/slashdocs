import { useCallback, useEffect, useMemo, useRef, useState } from "react";
import { useQueryClient } from "@tanstack/react-query";
import {
  api,
  type Anchor,
  type DocumentKVEvent,
  type DocumentPresence,
  type Principal,
} from "@/api";
import { queryKeys } from "@/lib/queryKeys";
import { samePrincipal } from "@/lib/principals";

// The server lists a viewer for 45 seconds after their last report.
const PRESENCE_TTL = 45_000;
const HEARTBEAT_INTERVAL = 20_000;
const REPORT_INTERVAL = 2_000;
const RECONNECT_DELAY = 10_000;

export type Viewer = {
  principal: Principal;
  version?: string;
  anchor?: Partial<Anchor>;
  seenAt: number;
};

export type DocumentKVListener = (event: DocumentKVEvent) => void;

// useDocumentStream holds the page's one connection to the document event
// stream. It passes KV changes to subscribers, refreshes comments and the
// document when they change, and reports and tracks who is viewing.
export function useDocumentStream({
  docId,
  version,
  principal,
  onRemoteDocumentChange,
}: {
  docId: string;
  version: string;
  principal?: Principal;
  onRemoteDocumentChange: () => void;
}) {
  const q = useQueryClient();
  const [session] = useState(() => crypto.randomUUID());
  const kvListeners = useRef(new Set<DocumentKVListener>());
  const [sessions, setSessions] = useState<ReadonlyMap<string, Viewer>>(new Map());
  const [attempt, setAttempt] = useState(0);
  const lastRevision = useRef(0);
  const principalRef = useRef(principal);
  principalRef.current = principal;
  const onRemoteDocumentChangeRef = useRef(onRemoteDocumentChange);
  onRemoteDocumentChangeRef.current = onRemoteDocumentChange;

  useEffect(() => {
    const params = new URLSearchParams({ session });
    // The browser resumes dropped streams itself. Only a stream it gave up on
    // is opened again here, and it continues from the last event it saw.
    if (lastRevision.current)
      params.set("after_revision", String(lastRevision.current));
    const source = new EventSource(
      `/v1/documents/${encodeURIComponent(docId)}/kv/events?${params}`,
      { withCredentials: true },
    );
    let reconnect: number | undefined;
    const read = (message: MessageEvent<string>): DocumentKVEvent | null => {
      lastRevision.current = Math.max(
        lastRevision.current,
        Number(message.lastEventId) || 0,
      );
      try {
        return JSON.parse(message.data) as DocumentKVEvent;
      } catch {
        return null;
      }
    };
    source.onopen = () => setSessions(new Map());
    source.onmessage = (message) => {
      const event = read(message);
      if (event) kvListeners.current.forEach((listener) => listener(event));
    };
    source.addEventListener("comments", (message) => {
      read(message);
      q.invalidateQueries({ queryKey: queryKeys.comments(docId) });
    });
    source.addEventListener("document", (message) => {
      const event = read(message);
      if (!samePrincipal(event?.updated_by, principalRef.current)) {
        onRemoteDocumentChangeRef.current();
      }
      q.invalidateQueries({ queryKey: queryKeys.document(docId) });
      q.invalidateQueries({ queryKey: queryKeys.versions(docId) });
    });
    source.addEventListener("presence", (message) => {
      const event = read(message);
      const presence = event?.data as DocumentPresence | undefined;
      const viewer = event?.updated_by;
      if (!presence?.session || presence.session === session || !viewer) return;
      const key = `${viewer.type}:${viewer.id}:${presence.session}`;
      setSessions((current) => {
        const next = new Map(current);
        if (presence.left) next.delete(key);
        else
          next.set(key, {
            principal: { ...viewer, picture_url: presence.picture_url },
            version: presence.version,
            anchor: presence.anchor,
            seenAt: Date.now(),
          });
        return next;
      });
    });
    source.onerror = () => {
      if (source.readyState !== EventSource.CLOSED) return;
      reconnect = window.setTimeout(() => setAttempt((n) => n + 1), RECONNECT_DELAY);
    };
    return () => {
      source.close();
      window.clearTimeout(reconnect);
    };
  }, [docId, session, attempt, q]);

  // Viewers whose tab closed without a goodbye drop out after the TTL.
  useEffect(() => {
    const timer = window.setInterval(() => {
      setSessions((current) => {
        const oldest = Date.now() - PRESENCE_TTL;
        const live = [...current].filter(([, viewer]) => viewer.seenAt > oldest);
        return live.length === current.size ? current : new Map(live);
      });
    }, 5_000);
    return () => window.clearInterval(timer);
  }, []);

  // Report this viewer's place: at once on a version change, at most every
  // REPORT_INTERVAL while scrolling, and as a heartbeat otherwise.
  const place = useRef<{ version: string; anchor?: Partial<Anchor> }>({ version });
  const lastReport = useRef(0);
  const pendingReport = useRef<number | undefined>(undefined);
  const report = useCallback(() => {
    window.clearTimeout(pendingReport.current);
    pendingReport.current = undefined;
    lastReport.current = Date.now();
    api.postDocumentPresence(docId, { session, ...place.current }).catch(() => {});
  }, [docId, session]);
  const scheduleReport = useCallback(() => {
    if (pendingReport.current !== undefined) return;
    const wait = Math.max(0, lastReport.current + REPORT_INTERVAL - Date.now());
    pendingReport.current = window.setTimeout(report, wait);
  }, [report]);
  const reportViewport = useCallback(
    (anchor?: Partial<Anchor>) => {
      place.current = { ...place.current, anchor };
      scheduleReport();
    },
    [scheduleReport],
  );
  useEffect(() => {
    place.current = { version };
    scheduleReport();
  }, [version, scheduleReport]);
  useEffect(() => {
    const timer = window.setInterval(report, HEARTBEAT_INTERVAL);
    return () => {
      window.clearInterval(timer);
      window.clearTimeout(pendingReport.current);
      pendingReport.current = undefined;
    };
  }, [report]);

  const subscribeKV = useCallback((listener: DocumentKVListener) => {
    kvListeners.current.add(listener);
    return () => {
      kvListeners.current.delete(listener);
    };
  }, []);

  // One avatar per person, at their most recently active tab.
  const viewers = useMemo(() => {
    const byPrincipal = new Map<string, Viewer>();
    for (const viewer of sessions.values()) {
      if (samePrincipal(viewer.principal, principal)) continue;
      const key = `${viewer.principal.type}:${viewer.principal.id}`;
      if ((byPrincipal.get(key)?.seenAt ?? 0) <= viewer.seenAt) {
        byPrincipal.set(key, viewer);
      }
    }
    return [...byPrincipal.values()];
  }, [sessions, principal]);

  return { viewers, subscribeKV, reportViewport };
}
