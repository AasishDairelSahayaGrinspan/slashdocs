import assert from "node:assert/strict";
import test from "node:test";

import { splitChangeSummary } from "../src/lib/change-summary.ts";

test("splits semicolon-joined prose into items", () => {
  assert.deepEqual(
    splitChangeSummary(
      "Removed the Themes matrix section; stripped reviewer rationale from 46 source lines; removed all audit citations",
    ),
    [
      "Removed the Themes matrix section",
      "stripped reviewer rationale from 46 source lines",
      "removed all audit citations",
    ],
  );
});

test("prefers newlines and strips bullet markers", () => {
  assert.deepEqual(splitChangeSummary("- one\n• two\n* three"), [
    "one",
    "two",
    "three",
  ]);
});

test("does not split a newline summary on a semicolon inside a line", () => {
  assert.deepEqual(
    splitChangeSummary("- fixed the parser; it dropped newlines\n- added a test"),
    ["fixed the parser; it dropped newlines", "added a test"],
  );
});

test("leaves a single-sentence summary as one item", () => {
  assert.deepEqual(splitChangeSummary("Address comments"), ["Address comments"]);
});

test("keeps a spaceless semicolon intact so quoted code survives", () => {
  assert.deepEqual(splitChangeSummary("Fixed foo();bar() ordering"), [
    "Fixed foo();bar() ordering",
  ]);
});

test("accepted cost: a mid-sentence semicolon still splits", () => {
  assert.deepEqual(splitChangeSummary("Fixed the parser; it was dropping newlines"), [
    "Fixed the parser",
    "it was dropping newlines",
  ]);
});

test("returns nothing for a blank summary", () => {
  assert.deepEqual(splitChangeSummary(""), []);
  assert.deepEqual(splitChangeSummary("   \n  "), []);
});
