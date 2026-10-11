import { describe, expect, it } from "vitest";
import { formatDate, formatTimestamp } from "./format-date";

describe("formatTimestamp", () => {
  it("returns an empty string for missing or invalid input", () => {
    expect(formatTimestamp(undefined)).toBe("");
    expect(formatTimestamp("")).toBe("");
    expect(formatTimestamp("not-a-date")).toBe("");
  });

  it("formats a valid timestamp with the time", () => {
    const out = formatTimestamp("2026-10-11T10:30:00Z");
    expect(out).not.toBe("");
    expect(out).toContain(":");
  });
});

describe("formatDate", () => {
  it("returns an empty string for missing or invalid input", () => {
    expect(formatDate(undefined)).toBe("");
    expect(formatDate("")).toBe("");
    expect(formatDate("not-a-date")).toBe("");
  });

  it("formats a valid date without the time", () => {
    const out = formatDate("2026-10-11T10:30:00Z");
    expect(out).not.toBe("");
    expect(out).not.toContain(":");
  });
});
