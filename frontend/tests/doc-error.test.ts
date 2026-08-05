import assert from "node:assert/strict";
import test from "node:test";

import { documentErrorState } from "../src/lib/doc-error.ts";

test("a healthy document has no error state", () => {
  const result = documentErrorState({
    failed: false,
    status: undefined,
    hasCachedCopy: true,
  });

  assert.equal(result, null);
});

test("a first load that failed takes over the page", () => {
  const result = documentErrorState({
    failed: true,
    status: undefined,
    hasCachedCopy: false,
  });

  assert.equal(result?.title, "Could not load document");
});

test("a failed background refresh leaves the cached document on screen", () => {
  const result = documentErrorState({
    failed: true,
    status: undefined,
    hasCachedCopy: true,
  });

  assert.equal(result, null);
});

test("a server error on refresh leaves the cached document on screen", () => {
  const result = documentErrorState({ failed: true, status: 500, hasCachedCopy: true });

  assert.equal(result, null);
});

test("losing access takes over the page even with a cached copy", () => {
  const result = documentErrorState({ failed: true, status: 403, hasCachedCopy: true });

  assert.equal(result?.title, "You don't have access to this document");
});

test("a deleted document takes over the page even with a cached copy", () => {
  const result = documentErrorState({ failed: true, status: 404, hasCachedCopy: true });

  assert.equal(result?.title, "Document not found");
});

test("a 403 on the very first load still takes over the page", () => {
  const result = documentErrorState({
    failed: true,
    status: 403,
    hasCachedCopy: false,
  });

  assert.equal(result?.title, "You don't have access to this document");
});
