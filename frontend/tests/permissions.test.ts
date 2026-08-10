import assert from "node:assert/strict";
import test from "node:test";

import { canEdit } from "../src/lib/constants.ts";

test("canEdit is false without a role", () => {
  assert.equal(canEdit(undefined), false);
  assert.equal(canEdit(""), false);
});

test("canEdit is false below editor", () => {
  assert.equal(canEdit("viewer"), false);
  assert.equal(canEdit("commenter"), false);
});

test("canEdit is true for editor and owner", () => {
  assert.equal(canEdit("editor"), true);
  assert.equal(canEdit("owner"), true);
});

test("canEdit is false for an unrecognised role", () => {
  assert.equal(canEdit("wizard"), false);
});
