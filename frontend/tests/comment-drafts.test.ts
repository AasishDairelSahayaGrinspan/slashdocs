import assert from "node:assert/strict";
import test from "node:test";

import {
  COMMENT_DRAFT_TTL_MS,
  MAX_PERSISTED_QUOTE,
  draftFromSelection,
  isPersistableDraft,
  parseCommentDraftMap,
  pruneCommentDrafts,
  putCommentDraft,
  readCommentDraft,
  removeCommentDraft,
  type CommentDraftMap,
  type StoredCommentDraft,
} from "../src/lib/comment-drafts.ts";

const NOW = 1_700_000_000_000;

function draft(overrides: Partial<StoredCommentDraft> = {}): StoredCommentDraft {
  return {
    quote: "a quoted sentence",
    body: "half-written thought",
    mentions: [],
    savedAt: NOW,
    ...overrides,
  };
}

test("parseCommentDraftMap rejects anything that is not a plain object", () => {
  for (const value of [null, undefined, "x", 42, [], true]) {
    assert.deepEqual(parseCommentDraftMap(value), {});
  }
});

test("parseCommentDraftMap drops entries without a usable quote", () => {
  const parsed = parseCommentDraftMap({
    doc_missing: { body: "orphan" },
    doc_empty: { quote: "", body: "orphan" },
    doc_wrong_type: { quote: 7, body: "orphan" },
    doc_not_object: "nope",
    doc_ok: draft(),
  });
  assert.deepEqual(Object.keys(parsed), ["doc_ok"]);
});

test("parseCommentDraftMap coerces off-contract fields instead of trusting them", () => {
  const parsed = parseCommentDraftMap({
    doc_a: { quote: "q", body: 12, mentions: "not-an-array", anchor: "nope" },
  });
  assert.deepEqual(parsed.doc_a, {
    quote: "q",
    anchor: undefined,
    body: "",
    mentions: [],
    savedAt: 0,
  });
});

test("parseCommentDraftMap round-trips a valid entry including its anchor", () => {
  const anchor = { quote: "q", start_offset: 4, end_offset: 9 };
  const parsed = parseCommentDraftMap({ doc_a: draft({ anchor }) });
  assert.deepEqual(parsed.doc_a, draft({ anchor }));
});

test("putCommentDraft inserts and overwrites without mutating the input", () => {
  const map: CommentDraftMap = { doc_a: draft({ body: "first" }) };
  const snapshot = structuredClone(map);

  const inserted = putCommentDraft(map, "doc_b", draft({ body: "second" }), NOW);
  assert.deepEqual(Object.keys(inserted).sort(), ["doc_a", "doc_b"]);

  const overwritten = putCommentDraft(map, "doc_a", draft({ body: "replaced" }), NOW);
  assert.equal(overwritten.doc_a.body, "replaced");

  assert.deepEqual(map, snapshot);
});

test("putCommentDraft sweeps entries nobody touched inside the TTL", () => {
  const stale = draft({ savedAt: NOW - COMMENT_DRAFT_TTL_MS - 1 });
  const fresh = draft({ savedAt: NOW - COMMENT_DRAFT_TTL_MS + 1 });
  const next = putCommentDraft(
    { doc_stale: stale, doc_fresh: fresh },
    "doc_new",
    draft(),
    NOW,
  );
  assert.deepEqual(Object.keys(next).sort(), ["doc_fresh", "doc_new"]);
});

test("pruneCommentDrafts keeps an entry sitting exactly on the TTL boundary", () => {
  const map = { doc_a: draft({ savedAt: NOW - COMMENT_DRAFT_TTL_MS }) };
  assert.deepEqual(Object.keys(pruneCommentDrafts(map, NOW)), ["doc_a"]);
});

test("removeCommentDraft returns the identical map when the key is absent", () => {
  // The mirror effect skips its write on this reference check, so it is
  // load-bearing rather than a micro-optimisation.
  const map: CommentDraftMap = { doc_a: draft() };
  assert.equal(removeCommentDraft(map, "doc_missing"), map);
});

test("removeCommentDraft deletes into a fresh object", () => {
  const map: CommentDraftMap = { doc_a: draft(), doc_b: draft() };
  const snapshot = structuredClone(map);
  const next = removeCommentDraft(map, "doc_a");
  assert.deepEqual(Object.keys(next), ["doc_b"]);
  assert.deepEqual(map, snapshot);
});

test("readCommentDraft returns undefined for an unknown or expired document", () => {
  const map: CommentDraftMap = {
    doc_expired: draft({ savedAt: NOW - COMMENT_DRAFT_TTL_MS - 1 }),
  };
  assert.equal(readCommentDraft(map, "doc_unknown", NOW), undefined);
  assert.equal(readCommentDraft(map, "doc_expired", NOW), undefined);
});

test("readCommentDraft hands back the stored entry untouched", () => {
  const entry = draft();
  assert.equal(readCommentDraft({ doc_a: entry }, "doc_a", NOW), entry);
});

test("draftFromSelection carries the typed body onto a newly selected quote", () => {
  const current = draft({ quote: "sentence A", body: "typed so far", savedAt: 1 });
  const next = draftFromSelection(current, "sentence B", { quote: "sentence B" }, NOW);
  assert.deepEqual(next, {
    quote: "sentence B",
    anchor: { quote: "sentence B" },
    body: "typed so far",
    mentions: [],
    savedAt: NOW,
  });
});

test("draftFromSelection starts empty when nothing was being composed", () => {
  const next = draftFromSelection(undefined, "sentence A", undefined, NOW);
  assert.deepEqual(next, {
    quote: "sentence A",
    anchor: undefined,
    body: "",
    mentions: [],
    savedAt: NOW,
  });
});

test("isPersistableDraft persists only a body-bearing draft of sane size", () => {
  // Gates the mirror effect: a false here means the stored entry is dropped.
  assert.equal(isPersistableDraft(undefined), false);
  assert.equal(isPersistableDraft(draft({ body: "" })), false);
  assert.equal(isPersistableDraft(draft({ body: "   " })), false);
  assert.equal(isPersistableDraft(draft()), true);
});

test("isPersistableDraft drops a draft whose quote would blow the storage budget", () => {
  const atLimit = draft({ quote: "q".repeat(MAX_PERSISTED_QUOTE) });
  const overLimit = draft({ quote: "q".repeat(MAX_PERSISTED_QUOTE + 1) });
  assert.equal(isPersistableDraft(atLimit), true);
  assert.equal(isPersistableDraft(overLimit), false);
});
