import { describe, expect, it } from "vitest";
import { formatDate, formatRelativeTime, formatTimestamp } from "./format-date";

describe("formatTimestamp", () => {
  it("returns an empty string for missing or invalid input", () => {
    expect(formatTimestamp(undefined)).toBe("");
    expect(formatTimestamp("")).toBe("");
    expect(formatTimestamp("not-a-date")).toBe("");
  });

  it("formats a valid timestamp", () => {
    expect(formatTimestamp("2026-10-11T10:30:00Z")).not.toBe("");
  });
});

describe("formatDate", () => {
  it("returns an empty string for missing or invalid input", () => {
    expect(formatDate(undefined)).toBe("");
    expect(formatDate("")).toBe("");
    expect(formatDate("not-a-date")).toBe("");
  });

  it("formats a valid date without the time", () => {
    const iso = "2026-10-11T10:30:00Z";
    expect(formatDate(iso)).not.toBe("");
    expect(formatDate(iso)).not.toContain(":");
  });
});

describe("formatRelativeTime", () => {
  const now = new Date("2026-10-11T12:00:00Z");
  const isoAgo = (ms: number) => new Date(now.getTime() - ms).toISOString();

  it("returns an empty string for missing or invalid input", () => {
    expect(formatRelativeTime(undefined, now)).toBe("");
    expect(formatRelativeTime("", now)).toBe("");
    expect(formatRelativeTime("not-a-date", now)).toBe("");
  });

  it("labels sub-minute timestamps as just now", () => {
    expect(formatRelativeTime(isoAgo(20_000), now)).toBe("just now");
  });

  it("labels minutes, hours, and days", () => {
    expect(formatRelativeTime(isoAgo(5 * 60_000), now)).toBe("5m ago");
    expect(formatRelativeTime(isoAgo(3 * 3_600_000), now)).toBe("3h ago");
    expect(formatRelativeTime(isoAgo(2 * 86_400_000), now)).toBe("2d ago");
    expect(formatRelativeTime(isoAgo(7 * 86_400_000), now)).toBe("7d ago");
  });

  it("falls back to a medium date beyond a week", () => {
    const out = formatRelativeTime(isoAgo(12 * 86_400_000), now);
    expect(out).not.toBe("");
    expect(out).not.toMatch(/ago$/);
  });

  it("falls back to a full timestamp for future dates", () => {
    const future = new Date(now.getTime() + 3_600_000).toISOString();
    const out = formatRelativeTime(future, now);
    expect(out).not.toBe("");
    expect(out).not.toMatch(/ago$/);
  });
});
