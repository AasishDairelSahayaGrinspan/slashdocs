import { describe, expect, it } from "vitest";
import { APIError } from "@/api";
import { errorMessage, isVersionConflict } from "@/lib/errors";

describe("isVersionConflict", () => {
  it("recognises a stale base version", () => {
    const err = new APIError(409, "version_conflict", "base_version_id is stale", {
      current_version_id: "ver_123",
    });
    expect(isVersionConflict(err)).toBe(true);
  });

  it("ignores other 409s", () => {
    expect(isVersionConflict(new APIError(409, "already_exists", "nope"))).toBe(false);
  });

  it("ignores other API errors", () => {
    expect(
      isVersionConflict(new APIError(403, "forbidden", "editor role required")),
    ).toBe(false);
  });

  it("ignores plain errors", () => {
    expect(isVersionConflict(new Error("version_conflict"))).toBe(false);
  });

  it("ignores non-errors", () => {
    expect(isVersionConflict(undefined)).toBe(false);
  });
});

describe("errorMessage", () => {
  it("prefers the error message", () => {
    expect(errorMessage(new Error("boom"), "fallback")).toBe("boom");
  });

  it("falls back when there is no message", () => {
    expect(errorMessage({}, "fallback")).toBe("fallback");
  });
});
