import { describe, expect, it } from "vitest";
import { formatRelativeTime } from "./format-date";

const now = new Date("2026-10-11T12:00:00Z");
const ago = (ms: number) => new Date(now.getTime() - ms).toISOString();

describe("formatRelativeTime", () => {
  it("returns an empty string for missing or invalid input", () => {
    expect(formatRelativeTime(undefined, now)).toBe("");
    expect(formatRelativeTime("not-a-date", now)).toBe("");
  });

  it("labels very recent versions as just now", () => {
    expect(formatRelativeTime(ago(30_000), now)).toBe("just now");
  });

  it("counts minutes, hours, and days", () => {
    expect(formatRelativeTime(ago(5 * 60_000), now)).toBe("5m ago");
    expect(formatRelativeTime(ago(3 * 60 * 60_000), now)).toBe("3h ago");
    expect(formatRelativeTime(ago(2 * 24 * 60 * 60_000), now)).toBe("2d ago");
  });

  it("falls back to an absolute date beyond a week", () => {
    const iso = ago(30 * 24 * 60 * 60_000);
    expect(formatRelativeTime(iso, now)).toBe(
      new Date(iso).toLocaleDateString(undefined, { dateStyle: "medium" }),
    );
  });

  it("shows the exact timestamp for future dates", () => {
    const future = new Date(now.getTime() + 60_000).toISOString();
    expect(formatRelativeTime(future, now)).toContain("2026");
  });
});
