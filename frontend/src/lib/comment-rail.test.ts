import { describe, expect, it } from "vitest";
import { layoutCommentRail } from "./comment-rail";

describe("layoutCommentRail", () => {
  it("places unanchored cards at the start before anchored cards", () => {
    const result = layoutCommentRail(
      [
        { id: "anchored", anchorTop: 20, height: 80 },
        { id: "lost", anchorTop: 0, height: 60, atStart: true },
      ],
      600,
    );

    expect(result.map(({ id, y }) => ({ id, y }))).toEqual([
      { id: "lost", y: 12 },
      { id: "anchored", y: 84 },
    ]);
  });

  it("keeps cards ordered and separated when highlights are close together", () => {
    const result = layoutCommentRail(
      [
        { id: "second", anchorTop: 115, height: 90 },
        { id: "first", anchorTop: 100, height: 80 },
      ],
      800,
    );

    expect(result[0]).toMatchObject({ id: "first", y: 100 });
    expect(result[1]).toMatchObject({ id: "second", y: 192 });
  });

  it("keeps a single card inside the viewport near the bottom", () => {
    const [result] = layoutCommentRail(
      [{ id: "bottom", anchorTop: 590, height: 100 }],
      640,
    );

    expect(result.y).toBe(528);
  });
});
