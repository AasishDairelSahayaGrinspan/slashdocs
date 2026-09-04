import { describe, expect, it } from "vitest";
import {
  activeAuthorToken,
  hasDocumentSearchOperators,
  replaceAuthorToken,
} from "./search-query";

describe("document search query helpers", () => {
  it("detects structured search operators", () => {
    expect(hasDocumentSearchOperators("incident labels:api,review")).toBe(true);
    expect(hasDocumentSearchOperators('in:"Product Specs"')).toBe(true);
    expect(hasDocumentSearchOperators("plain title")).toBe(false);
  });

  it("finds the author token at the cursor and skips regexes", () => {
    expect(activeAuthorToken("incident author:ali labels:api", 19)).toEqual({
      start: 9,
      end: 19,
      query: "ali",
    });
    expect(activeAuthorToken("author:/@example/", 10)).toBeNull();
  });

  it("replaces the whole active token and keeps typing position", () => {
    const token = activeAuthorToken("incident author:ali", 19);
    expect(token).not.toBeNull();
    expect(
      replaceAuthorToken("incident author:ali", token!, "alice@example.com"),
    ).toEqual({
      value: "incident author:alice@example.com ",
      cursor: 34,
    });
  });
});
