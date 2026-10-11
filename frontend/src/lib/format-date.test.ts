import { describe, expect, it } from "vitest";
import { formatDate, formatTimestamp } from "./format-date";

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
