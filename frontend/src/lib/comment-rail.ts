export type CommentRailItem = {
  id: string;
  anchorTop: number;
  height: number;
  pinned?: boolean;
};

export type CommentRailPlacement = CommentRailItem & { y: number };

export type CommentRailLayoutOptions = {
  padding?: number;
  gap?: number;
};

// Places unanchored cards first, then anchored cards near their highlight.
// Anchored cards are pushed down only as far as necessary to avoid overlap.
export function layoutCommentRail(
  items: CommentRailItem[],
  viewportHeight: number,
  options: CommentRailLayoutOptions = {},
): CommentRailPlacement[] {
  const padding = options.padding ?? 12;
  const gap = options.gap ?? 12;
  const pinned = items.filter((item) => item.pinned);
  const anchored = items
    .filter((item) => !item.pinned)
    .sort((a, b) => a.anchorTop - b.anchorTop || a.id.localeCompare(b.id));
  const placements: CommentRailPlacement[] = [];
  let cursor = padding;

  for (const item of pinned) {
    placements.push({ ...item, y: cursor });
    cursor += Math.max(1, item.height) + gap;
  }

  for (const item of anchored) {
    const height = Math.max(1, item.height);
    const maxY = Math.max(cursor, viewportHeight - padding - height);
    const desired = Math.min(maxY, Math.max(padding, item.anchorTop));
    const y = Math.max(cursor, desired);
    placements.push({ ...item, y });
    cursor = y + height + gap;
  }

  return placements;
}
