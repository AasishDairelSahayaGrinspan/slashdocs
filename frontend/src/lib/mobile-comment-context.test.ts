import { describe, expect, it } from "vitest";
import {
  commentForMobileScroll,
  commentIDsInSameBlock,
  type MobileCommentPosition,
} from "./mobile-comment-context";

function position(
  top: number,
  bottom: number,
  blockTop = top,
  blockBottom = bottom,
  left = 0,
): MobileCommentPosition {
  return { top, bottom, left, blockTop, blockBottom };
}

describe("mobile comment context", () => {
  it("keeps the selected thread stable inside a shared paragraph", () => {
    const positions = new Map([
      ["first", position(420, 440, 400, 520, 20)],
      ["second", position(460, 480, 400, 520, 80)],
      ["third", position(500, 520, 400, 520, 20)],
    ]);

    expect(
      commentForMobileScroll({
        activeID: "second",
        positions,
        scrollTop: 260,
        viewportHeight: 600,
      }),
    ).toBe("second");
    expect(commentIDsInSameBlock("second", positions)).toEqual([
      "first",
      "second",
      "third",
    ]);
  });

  it("switches to a different block once it owns the reading band", () => {
    const positions = new Map([
      ["earlier", position(100, 130, 90, 150)],
      ["current", position(960, 990, 930, 1020)],
    ]);

    expect(
      commentForMobileScroll({
        activeID: "earlier",
        positions,
        scrollTop: 700,
        viewportHeight: 600,
      }),
    ).toBe("current");
  });

  it("does not create a tray merely because the document scrolls", () => {
    expect(
      commentForMobileScroll({
        activeID: undefined,
        positions: new Map([["comment", position(200, 240)]]),
        scrollTop: 0,
        viewportHeight: 600,
      }),
    ).toBeUndefined();
  });
});
