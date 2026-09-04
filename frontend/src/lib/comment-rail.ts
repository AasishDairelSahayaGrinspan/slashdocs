export type CommentRailItem = {
  id: string;
  anchorTop: number;
  height: number;
  atStart?: boolean;
};

export type CommentRailPlacement = CommentRailItem & { y: number };

export type CommentRailLayoutOptions = {
  padding?: number;
  gap?: number;
};

// Places unanchored cards at the start of the document timeline, then anchored
// cards near their highlight. Cards are pushed down only enough to avoid overlap.
export function layoutCommentRail(
  items: CommentRailItem[],
  contentHeight: number,
  options: CommentRailLayoutOptions = {},
): CommentRailPlacement[] {
  const padding = options.padding ?? 12;
  const gap = options.gap ?? 12;
  const atStart = items.filter((item) => item.atStart);
  const anchored = items
    .filter((item) => !item.atStart)
    .sort((a, b) => a.anchorTop - b.anchorTop || a.id.localeCompare(b.id));
  const placements: CommentRailPlacement[] = [];
  let cursor = padding;

  for (const item of atStart) {
    placements.push({ ...item, y: cursor });
    cursor += Math.max(1, item.height) + gap;
  }

  for (const item of anchored) {
    const height = Math.max(1, item.height);
    const maxY = Math.max(cursor, contentHeight - padding - height);
    const desired = Math.min(maxY, Math.max(padding, item.anchorTop));
    const y = Math.max(cursor, desired);
    placements.push({ ...item, y });
    cursor = y + height + gap;
  }

  return placements;
}
