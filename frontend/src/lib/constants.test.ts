import { describe, expect, it } from "vitest";
import { canEditDocument } from "@/lib/constants";

describe("canEditDocument", () => {
  it("allows editors", () => {
    expect(canEditDocument("editor")).toBe(true);
  });

  it("allows owners", () => {
    expect(canEditDocument("owner")).toBe(true);
  });

  it("rejects commenters", () => {
    expect(canEditDocument("commenter")).toBe(false);
  });

  it("rejects viewers", () => {
    expect(canEditDocument("viewer")).toBe(false);
  });

  it("rejects an absent role", () => {
    expect(canEditDocument(undefined)).toBe(false);
  });

  it("rejects an unrecognised role", () => {
    expect(canEditDocument("superuser")).toBe(false);
  });
});
