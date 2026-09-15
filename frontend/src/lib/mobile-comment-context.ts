export type MobileCommentPosition = {
  top: number;
  bottom: number;
  left: number;
  blockTop: number;
  blockBottom: number;
};

const SAME_BLOCK_TOLERANCE = 2;

export function sameCommentBlock(
  a: MobileCommentPosition | undefined,
  b: MobileCommentPosition | undefined,
) {
  return Boolean(
    a &&
    b &&
    Math.abs(a.blockTop - b.blockTop) <= SAME_BLOCK_TOLERANCE &&
    Math.abs(a.blockBottom - b.blockBottom) <= SAME_BLOCK_TOLERANCE,
  );
}

function distanceToRange(point: number, top: number, bottom: number) {
  if (point < top) return top - point;
  if (point > bottom) return point - bottom;
  return 0;
}

function compareDocumentOrder(
  [, a]: readonly [string, MobileCommentPosition],
  [, b]: readonly [string, MobileCommentPosition],
) {
  if (Math.abs(a.top - b.top) > SAME_BLOCK_TOLERANCE) return a.top - b.top;
  if (Math.abs(a.left - b.left) > SAME_BLOCK_TOLERANCE) return a.left - b.left;
  return a.bottom - b.bottom;
}

export function commentIDsInSameBlock(
  activeID: string | undefined,
  positions: ReadonlyMap<string, MobileCommentPosition>,
) {
  if (!activeID) return [];
  const active = positions.get(activeID);
  if (!active) return [];
  return Array.from(positions.entries())
    .filter(([, position]) => sameCommentBlock(active, position))
    .sort(compareDocumentOrder)
    .map(([id]) => id);
}

export function commentForMobileScroll({
  activeID,
  positions,
  scrollTop,
  viewportHeight,
}: {
  activeID: string | undefined;
  positions: ReadonlyMap<string, MobileCommentPosition>;
  scrollTop: number;
  viewportHeight: number;
}) {
  if (!activeID || !positions.has(activeID) || viewportHeight <= 0) return activeID;

  // The lower part of the viewport is occupied by the tray. Use a stable
  // reading band above it, so a highlight becomes current only after it is
  // comfortably visible instead of while it briefly touches an edge.
  const bandTop = scrollTop + viewportHeight * 0.2;
  const bandBottom = scrollTop + viewportHeight * 0.68;
  const readingLine = scrollTop + viewportHeight * 0.42;
  const candidates = Array.from(positions.entries()).filter(
    ([, position]) =>
      position.blockBottom >= bandTop && position.blockTop <= bandBottom,
  );
  if (candidates.length === 0) return activeID;

  candidates.sort((a, b) => {
    const distance =
      distanceToRange(readingLine, a[1].blockTop, a[1].blockBottom) -
      distanceToRange(readingLine, b[1].blockTop, b[1].blockBottom);
    return Math.abs(distance) > 0.5 ? distance : compareDocumentOrder(a, b);
  });

  const [nextID, next] = candidates[0];
  const active = positions.get(activeID)!;
  // Several anchors in one paragraph form one scroll stop. Keep the exact
  // thread the reviewer selected and let the tray controls or highlight taps
  // move between siblings; vertical movement must not make them flicker.
  if (sameCommentBlock(active, next)) return activeID;

  const activeInBand = active.blockBottom >= bandTop && active.blockTop <= bandBottom;
  if (activeInBand) {
    const margin = Math.max(24, viewportHeight * 0.04);
    const activeDistance = distanceToRange(
      readingLine,
      active.blockTop,
      active.blockBottom,
    );
    const nextDistance = distanceToRange(readingLine, next.blockTop, next.blockBottom);
    if (nextDistance + margin >= activeDistance) return activeID;
  }
  return nextID;
}
