import { useCallback, useEffect, useMemo, useRef, useState } from "react";
import { useSearchParams } from "react-router-dom";
import { toast } from "sonner";
import { currentVersionID, versionID, type Document, type Version } from "@/api";
import { displayedVersion, versionAnnouncement } from "@/lib/doc-version";

export const DOC_REFRESH = {
  refetchOnWindowFocus: true,
  staleTime: 30_000,
} as const;

export function useDocumentVersion(
  doc: Document | undefined,
  versionList: { items: Version[] } | undefined,
  { versionsUnavailable, hold }: { versionsUnavailable: boolean; hold: boolean },
) {
  const [searchParams, setSearchParams] = useSearchParams();
  const [selected, setSelected] = useState<string>();
  const versions = useMemo(() => versionList?.items || [], [versionList]);
  const realLatest =
    currentVersionID(doc || {}) || versionID(versions[versions.length - 1] || {});

  // A comment's anchor is captured against the version on screen when the draft
  // opens. If a new version publishes mid-draft, letting `latest` follow it
  // would submit the old anchor against new text, landing on the wrong sentence.
  // Hold `latest` at the draft's version until the draft ends (send or cancel).
  // State, not a ref: it's read during render, and setting state during render
  // is the concurrent-safe way — a ref written mid-render can survive a render
  // React discarded.
  const [heldAt, setHeldAt] = useState<string>();
  if (hold) {
    if (heldAt === undefined) setHeldAt(realLatest);
  } else if (heldAt !== undefined) {
    setHeldAt(undefined);
  }
  const latest = hold && heldAt ? heldAt : realLatest;
  const newerVersionPending = hold && !!heldAt && !!realLatest && realLatest !== heldAt;

  const { version: current, pinned } = displayedVersion({
    selected,
    queryVersion: searchParams.get("version") || undefined,
    latest,
  });

  // Selecting clears the URL pin; picking the newest clears the selection too,
  // so a later publish isn't blocked by a stale id.
  const selectVersion = useCallback(
    (version: string) => {
      setSelected(version === latest ? undefined : version);
      setSearchParams(
        (params) => {
          const next = new URLSearchParams(params);
          next.delete("version");
          return next;
        },
        { replace: true },
      );
    },
    [latest, setSearchParams],
  );

  const announced = useRef<string | undefined>(undefined);
  useEffect(() => {
    const result = versionAnnouncement({
      previousLatest: announced.current,
      latest,
      userPinned: pinned,
      versions,
      versionsUnavailable,
    });
    if (result.advance) {
      announced.current = latest;
      if (result.announce !== null) toast.success(result.announce);
    }
  }, [latest, pinned, versions, versionsUnavailable]);

  return { current, latest, pinned, versions, selectVersion, newerVersionPending };
}
