import assert from "node:assert/strict";
import test from "node:test";

import { focusEventSetup } from "../src/lib/query-focus.ts";

test("subscribes to both events, nudges with no argument, and cleans up", () => {
  type Sub = { type: string; listener: EventListener; capture: unknown };
  const added: Sub[] = [];
  const removed: Sub[] = [];
  const calls: unknown[][] = [];
  const target: EventTarget = {
    addEventListener: (type, listener, options) => {
      added.push({ type, listener: listener as EventListener, capture: options });
    },
    removeEventListener: (type, listener, options) => {
      removed.push({ type, listener: listener as EventListener, capture: options });
    },
    dispatchEvent: () => true,
  };

  const cleanup = focusEventSetup(target)((...args: unknown[]) => {
    calls.push(args);
  });
  for (const entry of added) entry.listener(new Event(entry.type));
  cleanup();

  // `focus` covers a tab that is visible but not focused, which never fires
  // `visibilitychange`. Both subscribe in the bubble phase (capture `false`).
  assert.deepEqual(
    added.map((entry) => ({ type: entry.type, capture: entry.capture })),
    [
      { type: "visibilitychange", capture: false },
      { type: "focus", capture: false },
    ],
  );
  // A boolean would route TanStack Query to setFocused(), permanently switching
  // isFocused() from tab visibility to a caller-supplied boolean.
  assert.deepEqual(calls, [[], []]);
  // Same types and listener references are torn down, so nothing leaks.
  assert.deepEqual(
    removed.map((entry) => ({ type: entry.type, listener: entry.listener })),
    added.map((entry) => ({ type: entry.type, listener: entry.listener })),
  );
});
