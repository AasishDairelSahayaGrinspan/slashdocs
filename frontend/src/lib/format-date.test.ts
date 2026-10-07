import { describe, expect, it } from "vitest";
import { formatRelativeTime, formatTimestamp } from "./format-date";

const NOW = new Date("2026-10-08T12:00:00Z");

function ago(ms: number): string {
  return new Date(NOW.getTime() - ms).toISOString();
}

describe("formatTimestamp", () => {
  it("returns an empty string for missing or invalid input", () => {
    expect(formatTimestamp(undefined)).toBe("");
    expect(formatTimestamp("not-a-date")).toBe("");
  });

  it("renders a localized date and time", () => {
    expect(formatTimestamp("2026-10-08T12:00:00Z")).not.toBe("");
  });
});

describe("formatRelativeTime", () => {
  it("returns an empty string for missing or invalid input", () => {
    expect(formatRelativeTime(undefined, NOW)).toBe("");
    expect(formatRelativeTime("not-a-date", NOW)).toBe("");
  });

  it("collapses sub-minute and future diffs to just now", () => {
    expect(formatRelativeTime(ago(30_000), NOW)).toBe("just now");
    expect(formatRelativeTime(ago(-60_000), NOW)).toBe("just now");
  });

  it("renders minutes, hours, and days", () => {
    expect(formatRelativeTime(ago(5 * 60_000), NOW)).toBe("5m ago");
    expect(formatRelativeTime(ago(3 * 3_600_000), NOW)).toBe("3h ago");
    expect(formatRelativeTime(ago(2 * 86_400_000), NOW)).toBe("2d ago");
  });

  it("renders weeks, months, and years", () => {
    expect(formatRelativeTime(ago(2 * 7 * 86_400_000), NOW)).toBe("2w ago");
    expect(formatRelativeTime(ago(60 * 86_400_000), NOW)).toBe("2mo ago");
    expect(formatRelativeTime(ago(800 * 86_400_000), NOW)).toBe("2y ago");
  });

  it("uses the unit boundaries exactly", () => {
    expect(formatRelativeTime(ago(59_999), NOW)).toBe("just now");
    expect(formatRelativeTime(ago(60_000), NOW)).toBe("1m ago");
    expect(formatRelativeTime(ago(3_599_999), NOW)).toBe("59m ago");
    expect(formatRelativeTime(ago(3_600_000), NOW)).toBe("1h ago");
    expect(formatRelativeTime(ago(6 * 86_400_000), NOW)).toBe("6d ago");
    expect(formatRelativeTime(ago(7 * 86_400_000), NOW)).toBe("1w ago");
  });
});
