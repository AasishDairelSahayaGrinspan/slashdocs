/**
 * Which version is shown, and whether the reader is pinned to it. Selecting the
 * version that is already newest means "follow latest", not a pin — otherwise
 * "Back to latest" would freeze them there. A `?version=` in the URL always pins.
 */
export function displayedVersion({
  selected,
  queryVersion,
  latest,
}: {
  selected: string | undefined;
  queryVersion: string | undefined;
  latest: string;
}): { version: string; pinned: boolean } {
  const selectedPin = selected && selected !== latest ? selected : undefined;
  const pin = selectedPin || queryVersion;
  return { version: pin || latest, pinned: !!pin };
}

type VersionAnnouncement =
  | { advance: false }
  // Baseline moves to `latest`; `announce` is the toast, or null when silent
  // (first load, or a pinned reader). Announcing without advancing would replay
  // the toast on the next refresh, so that pairing is unrepresentable.
  | { advance: true; announce: string | null };

/**
 * A pinned reader advances the baseline silently, so returning to latest later
 * doesn't replay what they skipped. If the list is unavailable, announce without
 * a number rather than swapping the document silently.
 */
export function versionAnnouncement({
  previousLatest,
  latest,
  userPinned,
  versions,
  versionsUnavailable,
}: {
  previousLatest: string | undefined;
  latest: string;
  userPinned: boolean;
  versions: readonly { id: string; number: number }[];
  versionsUnavailable: boolean;
}): VersionAnnouncement {
  if (!latest) return { advance: false };
  if (previousLatest === undefined || userPinned) {
    return { announce: null, advance: true };
  }
  if (previousLatest === latest) return { advance: false };
  const number = versions.find((version) => version.id === latest)?.number;
  if (number !== undefined) {
    return { announce: `Updated to version ${number}`, advance: true };
  }
  if (versionsUnavailable) {
    return { announce: "Updated to a newer version", advance: true };
  }
  return { advance: false };
}
