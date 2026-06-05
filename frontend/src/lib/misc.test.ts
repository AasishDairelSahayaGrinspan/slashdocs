import { describe, expect, it } from "vitest";
import { APIError } from "@/api";
import { cn } from "./cn";
import { publicURL } from "./config";
import { errorMessage } from "./errors";
import { queryKeys } from "./queryKeys";

describe("small lib helpers", () => {
  it("normalizes class names and errors", () => {
    expect(cn("a", false && "b", "c")).toBe("a c");
    expect(errorMessage(new Error("bad"))).toBe("bad");
    expect(errorMessage(new APIError(400, "bad", "api bad"))).toBe("api bad");
    expect(errorMessage("nope", "fallback")).toBe("fallback");
  });

  it("resolves public URL and query keys", () => {
    window.__AIDOCS_PUBLIC_URL__ = "https://aidocs.example.com/";
    expect(publicURL()).toBe("https://aidocs.example.com");
    window.__AIDOCS_PUBLIC_URL__ = "__AIDOCS_PUBLIC_URL_VALUE__";
    expect(publicURL()).toBe(window.location.origin);
    expect(queryKeys.documents()).toEqual(["documents"]);
    expect(queryKeys.comments("doc_1")).toEqual(["comments", "doc_1"]);
  });
});
