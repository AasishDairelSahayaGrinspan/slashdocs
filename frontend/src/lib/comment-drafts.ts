import { useEffect, useRef, useState } from "react";
import type { Anchor, CommentMention } from "@/api";
import { emptyMentionDraft, type MentionDraft } from "./comment-mentions.ts";
import { createPersistentStore } from "./persistent-store.ts";

export type StoredCommentDraft = {
  quote: string;
  anchor?: Partial<Anchor>;
  body: string;
  mentions: CommentMention[];
  savedAt: number;
};

// Keyed by document under a single storage key: createPersistentStore has no
// remove API, so a key per document would leak one key per document forever.
export type CommentDraftMap = Record<string, StoredCommentDraft>;

export const COMMENT_DRAFT_TTL_MS = 7 * 24 * 60 * 60 * 1000;

const COMMENT_DRAFTS_KEY = "aidocs.comment-drafts";

function toStoredDraft(value: unknown): StoredCommentDraft | null {
  if (!value || typeof value !== "object" || Array.isArray(value)) return null;
  // Safe: every field is validated below before it is used.
  const v = value as Partial<StoredCommentDraft>;
  if (typeof v.quote !== "string" || !v.quote) return null;
  return {
    quote: v.quote,
    anchor: v.anchor && typeof v.anchor === "object" ? v.anchor : undefined,
    body: typeof v.body === "string" ? v.body : "",
    mentions: Array.isArray(v.mentions) ? v.mentions : [],
    savedAt: typeof v.savedAt === "number" ? v.savedAt : 0,
  };
}

export function parseCommentDraftMap(parsed: unknown): CommentDraftMap {
  if (!parsed || typeof parsed !== "object" || Array.isArray(parsed)) return {};
  const map: CommentDraftMap = {};
  for (const [docId, value] of Object.entries(parsed)) {
    const entry = toStoredDraft(value);
    if (entry) map[docId] = entry;
  }
  return map;
}

export function pruneCommentDrafts(map: CommentDraftMap, now: number): CommentDraftMap {
  return Object.fromEntries(
    Object.entries(map).filter(
      ([, entry]) => now - entry.savedAt <= COMMENT_DRAFT_TTL_MS,
    ),
  );
}

export function putCommentDraft(
  map: CommentDraftMap,
  docId: string,
  entry: StoredCommentDraft,
  now: number,
): CommentDraftMap {
  return pruneCommentDrafts({ ...map, [docId]: entry }, now);
}

// Returns the same map when absent, so callers can skip a pointless write.
export function removeCommentDraft(
  map: CommentDraftMap,
  docId: string,
): CommentDraftMap {
  if (!(docId in map)) return map;
  const next = { ...map };
  delete next[docId];
  return next;
}

export function readCommentDraft(
  map: CommentDraftMap,
  docId: string,
  now: number,
): StoredCommentDraft | undefined {
  const entry = map[docId];
  if (!entry || now - entry.savedAt > COMMENT_DRAFT_TTL_MS) return undefined;
  return entry;
}

// Selecting new text mid-draft must not discard what the user already typed.
export function draftFromSelection(
  current: StoredCommentDraft | undefined,
  quote: string,
  anchor: Partial<Anchor> | undefined,
  now: number,
): StoredCommentDraft {
  return {
    quote,
    anchor,
    body: current?.body || "",
    mentions: current?.mentions || [],
    savedAt: now,
  };
}

// A whole-document selection would rewrite the entire map on every keystroke
// and can exhaust the 5MB origin budget, which is also what makes the
// clearCommentDrafts write below fail. ponytail: such drafts stay in memory
// only rather than being truncated, so quote and anchor never disagree.
export const MAX_PERSISTED_QUOTE = 4000;

// An accidental selection should not resurrect a composer on the next load.
export function isPersistableDraft(
  entry: StoredCommentDraft | undefined,
): entry is StoredCommentDraft {
  if (!entry || !entry.body.trim()) return false;
  return entry.quote.length <= MAX_PERSISTED_QUOTE;
}

const store = createPersistentStore<CommentDraftMap>(
  COMMENT_DRAFTS_KEY,
  {},
  parseCommentDraftMap,
);

// createPersistentStore does not guard write(), and setItem throws on a full
// quota or blocked storage. Losing persistence beats breaking the composer.
function save(next: CommentDraftMap) {
  try {
    store.write(next);
  } catch {}
}

// Louder than save(): failing here leaves one account's draft readable by the
// next account to sign in on this browser, so it must not vanish silently.
export function clearCommentDrafts() {
  try {
    store.write({});
  } catch (e) {
    console.error("[comment-drafts] sign-out clear failed; a draft may survive", e);
  }
}

// Mirrors PendingSelection in pages/doc-context so lib/ stays off pages/.
type SelectionInput = { quote: string; anchor?: Partial<Anchor> } | null;

export type CommentDraftHandle = {
  quote: string;
  anchor?: Partial<Anchor>;
  draft: MentionDraft;
  // True when this draft came back from storage rather than a live selection,
  // so the composer can open without stealing focus from the document.
  restored: boolean;
  setDraft: (draft: MentionDraft) => void;
  clearDraft: () => void;
};

// React state stays authoritative while the user types; storage is a mirror
// written after paint, so no keystroke waits on — or is lost to — localStorage.
export function useCommentDraft(
  docId: string,
  selection: SelectionInput,
): CommentDraftHandle {
  const [entry, setEntry] = useState<StoredCommentDraft | undefined>(() =>
    readCommentDraft(store.read(), docId, Date.now()),
  );
  // The bridge posts a brand-new object on every settled selection gesture,
  // even when the same text is reselected, so identity rather than value is
  // what separates a new selection from one already consumed.
  const consumed = useRef<SelectionInput>(null);
  const hydratedFor = useRef(docId);
  // ponytail: a ref, not state — autoFocus only takes effect when the composer
  // mounts, so nothing needs a re-render when this flips.
  const restored = useRef(true);

  if (hydratedFor.current !== docId) {
    // DocumentPage does not remount between documents, so re-read here and drop
    // the outgoing selection rather than quoting the previous document.
    hydratedFor.current = docId;
    consumed.current = selection;
    restored.current = true;
    setEntry(readCommentDraft(store.read(), docId, Date.now()));
  }

  useEffect(() => {
    if (consumed.current === selection) return;
    consumed.current = selection;
    if (!selection?.quote) return;
    const { quote, anchor } = selection;
    restored.current = false;
    setEntry((current) => draftFromSelection(current, quote, anchor, Date.now()));
  }, [selection]);

  useEffect(() => {
    const map = store.read();
    const next = isPersistableDraft(entry)
      ? putCommentDraft(map, docId, entry, Date.now())
      : removeCommentDraft(map, docId);
    if (next !== map) save(next);
  }, [entry, docId]);

  return {
    quote: entry?.quote || "",
    anchor: entry?.anchor,
    draft: entry ? { body: entry.body, mentions: entry.mentions } : emptyMentionDraft(),
    restored: restored.current,
    setDraft: (next) =>
      setEntry((current) =>
        current
          ? {
              ...current,
              body: next.body,
              mentions: next.mentions,
              savedAt: Date.now(),
            }
          : current,
      ),
    clearDraft: () => setEntry(undefined),
  };
}
