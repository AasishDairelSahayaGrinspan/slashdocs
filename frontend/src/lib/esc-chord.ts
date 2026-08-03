// Esc-leader keyboard chords for the document viewer.
//
// Pressing Esc arms a short window during which a single follow-up key runs an
// action (Esc t = theme, Esc f = full screen, Esc c = comments, Esc h = home).
// If the window expires unused, Esc falls through to its ordinary meaning, so
// Esc still exits full screen. This module is deliberately pure: all timing,
// DOM wiring and side effects live in use-esc-chord.ts.

export const CHORD_WINDOW_MS = 800;
export const CHORD_RESOLVED_MS = 600;

export type ChordKey = "t" | "f" | "c" | "h";

export const CHORD_KEYS: ReadonlyArray<{ key: ChordKey; hint: string }> = [
  { key: "t", hint: "theme" },
  { key: "f", hint: "full screen" },
  { key: "c", hint: "comments" },
  { key: "h", hint: "home" },
];

const KEYS: ReadonlySet<string> = new Set(CHORD_KEYS.map((k) => k.key));

export type ChordState =
  | { phase: "idle" }
  | { phase: "armed" }
  | { phase: "resolved"; key: ChordKey };

export const IDLE: ChordState = { phase: "idle" };

export type ChordEvent =
  | { type: "keydown"; key: string; modified?: boolean }
  // The arming window elapsed with no follow-up key.
  | { type: "expire" }
  // The resolved pill finished showing.
  | { type: "clear" }
  // Drop any pending chord without side effects, e.g. because a dialog took
  // over the Escape key or the reader focused a text field.
  | { type: "cancel" };

export type ChordEffect =
  // Run the chord action for this key.
  | { type: "run"; key: ChordKey }
  // Let Escape mean whatever it ordinarily means here.
  | { type: "escape" }
  | null;

// chordKeyFromEvent maps a KeyboardEvent.key onto a chord key, case
// insensitively, or null when the key is not part of the chord.
export function chordKeyFromEvent(key: string): ChordKey | null {
  const lower = key.toLowerCase();
  return KEYS.has(lower) ? (lower as ChordKey) : null;
}

// reduceChord advances the chord state machine. `available` is the subset of
// keys bound right now: full screen has no comments panel, so "c" drops out.
export function reduceChord(
  state: ChordState,
  event: ChordEvent,
  available: ReadonlyArray<ChordKey>,
): { state: ChordState; effect: ChordEffect } {
  if (event.type === "cancel") {
    return { state: state.phase === "idle" ? state : IDLE, effect: null };
  }
  if (event.type === "clear") {
    return { state: state.phase === "resolved" ? IDLE : state, effect: null };
  }
  if (event.type === "expire") {
    // An expire queued before the chord resolved must not undo the resolution.
    if (state.phase !== "armed") return { state, effect: null };
    return { state: IDLE, effect: { type: "escape" } };
  }

  const armed = state.phase === "armed";
  if (event.modified) {
    // A modifier combination is never part of the chord; it cancels a pending
    // one and is otherwise left alone.
    return { state: armed ? IDLE : state, effect: null };
  }
  if (event.key === "Escape") {
    // While armed, a second Escape means the reader is trying to get out, so
    // stop waiting and fire the ordinary Escape action immediately.
    if (armed) return { state: IDLE, effect: { type: "escape" } };
    return { state: { phase: "armed" }, effect: null };
  }
  if (!armed) return { state, effect: null };

  const key = chordKeyFromEvent(event.key);
  // A miss cancels silently: the reader meant a chord, so Escape must not also
  // fire and yank them out of full screen.
  if (!key || !available.includes(key)) return { state: IDLE, effect: null };
  return { state: { phase: "resolved", key }, effect: { type: "run", key } };
}

const TEXT_INPUT_TYPES: ReadonlySet<string> = new Set([
  "text",
  "search",
  "email",
  "url",
  "tel",
  "password",
  "number",
]);

// isChordBlocked reports whether Esc must keep its instant native meaning:
// while the reader is typing, or while a Radix dialog, sheet or menu is open
// and owns the Escape key.
export function isChordBlocked(target: EventTarget | null): boolean {
  const el = target instanceof Element ? target : null;
  if (el) {
    const tag = el.tagName;
    if (tag === "TEXTAREA") return true;
    if (tag === "INPUT") {
      const type = (el.getAttribute("type") || "text").toLowerCase();
      if (TEXT_INPUT_TYPES.has(type)) return true;
    }
    if (el.closest('[contenteditable="true"], [contenteditable=""]')) return true;
  }
  const doc = el?.ownerDocument || globalThis.document;
  return !!doc?.querySelector(
    '[role="dialog"][data-state="open"], [role="menu"][data-state="open"]',
  );
}
