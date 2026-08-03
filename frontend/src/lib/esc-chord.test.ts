import { describe, expect, it } from "vitest";
import {
  CHORD_KEYS,
  IDLE,
  chordKeyFromEvent,
  isChordBlocked,
  reduceChord,
  type ChordKey,
  type ChordState,
} from "./esc-chord";

const ALL: ChordKey[] = ["t", "f", "c", "h"];

function press(state: ChordState, key: string, available: ChordKey[] = ALL) {
  return reduceChord(state, { type: "keydown", key }, available);
}

function armed(available: ChordKey[] = ALL) {
  return press(IDLE, "Escape", available).state;
}

describe("reduceChord", () => {
  it("arms on Escape without running anything", () => {
    const { state, effect } = press(IDLE, "Escape");
    expect(state.phase).toBe("armed");
    expect(effect).toBeNull();
  });

  it("ignores keys while idle", () => {
    const { state, effect } = press(IDLE, "t");
    expect(state).toEqual(IDLE);
    expect(effect).toBeNull();
  });

  it.each(ALL)("resolves Escape then %s into that action", (key) => {
    const { state, effect } = press(armed(), key);
    expect(state).toEqual({ phase: "resolved", key });
    expect(effect).toEqual({ type: "run", key });
  });

  it("accepts an uppercase second key", () => {
    const { effect } = press(armed(), "T");
    expect(effect).toEqual({ type: "run", key: "t" });
  });

  it("treats an unavailable key as a miss, cancelling silently", () => {
    const { state, effect } = press(armed(["t", "f", "h"]), "c", ["t", "f", "h"]);
    expect(state).toEqual(IDLE);
    expect(effect).toBeNull();
  });

  it("cancels silently on an unmapped key, with no Escape fall-through", () => {
    const { state, effect } = press(armed(), "q");
    expect(state).toEqual(IDLE);
    expect(effect).toBeNull();
  });

  it("falls through to Escape when the window expires unused", () => {
    const { state, effect } = reduceChord(armed(), { type: "expire" }, ALL);
    expect(state).toEqual(IDLE);
    expect(effect).toEqual({ type: "escape" });
  });

  it("fires Escape immediately on a second Escape so mashing still exits", () => {
    const { state, effect } = press(armed(), "Escape");
    expect(state).toEqual(IDLE);
    expect(effect).toEqual({ type: "escape" });
  });

  it("clears the resolved pill back to idle", () => {
    const resolved = press(armed(), "t").state;
    const { state, effect } = reduceChord(resolved, { type: "clear" }, ALL);
    expect(state).toEqual(IDLE);
    expect(effect).toBeNull();
  });

  it("re-arms when Escape is pressed while a resolved pill is still showing", () => {
    const resolved = press(armed(), "t").state;
    const { state, effect } = press(resolved, "Escape");
    expect(state.phase).toBe("armed");
    expect(effect).toBeNull();
  });

  it("ignores an expire that arrives after the chord already resolved", () => {
    const resolved = press(armed(), "t").state;
    const { state, effect } = reduceChord(resolved, { type: "expire" }, ALL);
    expect(state).toEqual(resolved);
    expect(effect).toBeNull();
  });

  it("cancels a pending chord with no side effects", () => {
    const { state, effect } = reduceChord(armed(), { type: "cancel" }, ALL);
    expect(state).toEqual(IDLE);
    expect(effect).toBeNull();
  });

  it("is a no-op to cancel while idle", () => {
    const { state, effect } = reduceChord(IDLE, { type: "cancel" }, ALL);
    expect(state).toBe(IDLE);
    expect(effect).toBeNull();
  });

  it("does not run an action for a modified second key", () => {
    const { state, effect } = reduceChord(
      armed(),
      { type: "keydown", key: "t", modified: true },
      ALL,
    );
    expect(state).toEqual(IDLE);
    expect(effect).toBeNull();
  });

  it("does not arm on a modified Escape", () => {
    const { state, effect } = reduceChord(
      IDLE,
      { type: "keydown", key: "Escape", modified: true },
      ALL,
    );
    expect(state).toEqual(IDLE);
    expect(effect).toBeNull();
  });

  it("exposes the chord keys in display order", () => {
    expect(CHORD_KEYS.map((k) => k.key)).toEqual(ALL);
  });
});

describe("chordKeyFromEvent", () => {
  it("maps the mapped keys and rejects everything else", () => {
    expect(chordKeyFromEvent("f")).toBe("f");
    expect(chordKeyFromEvent("H")).toBe("h");
    expect(chordKeyFromEvent("Escape")).toBeNull();
    expect(chordKeyFromEvent("z")).toBeNull();
  });
});

describe("isChordBlocked", () => {
  function withDOM(html: string, run: () => void) {
    document.body.innerHTML = html;
    try {
      run();
    } finally {
      document.body.innerHTML = "";
    }
  }

  it("is unblocked on a plain document", () => {
    withDOM("<p>hello</p>", () => {
      expect(isChordBlocked(document.querySelector("p"))).toBe(false);
    });
  });

  it("blocks while focus is in a text field", () => {
    withDOM("<input /><textarea></textarea>", () => {
      expect(isChordBlocked(document.querySelector("input"))).toBe(true);
      expect(isChordBlocked(document.querySelector("textarea"))).toBe(true);
    });
  });

  it("blocks inside a contenteditable region", () => {
    withDOM('<div contenteditable="true"><span>x</span></div>', () => {
      expect(isChordBlocked(document.querySelector("span"))).toBe(true);
    });
  });

  it("does not block on a non-text input such as a checkbox", () => {
    withDOM('<input type="checkbox" />', () => {
      expect(isChordBlocked(document.querySelector("input"))).toBe(false);
    });
  });

  it("blocks while a Radix dialog or menu is open", () => {
    withDOM('<div role="dialog" data-state="open"></div>', () => {
      expect(isChordBlocked(document.body)).toBe(true);
    });
    withDOM('<div role="menu" data-state="open"></div>', () => {
      expect(isChordBlocked(document.body)).toBe(true);
    });
  });

  it("does not block on a closed overlay", () => {
    withDOM('<div role="dialog" data-state="closed"></div>', () => {
      expect(isChordBlocked(document.body)).toBe(false);
    });
  });
});
