import * as React from "react";
import {
  CHORD_RESOLVED_MS,
  CHORD_WINDOW_MS,
  IDLE,
  isChordBlocked,
  reduceChord,
  type ChordEvent,
  type ChordKey,
  type ChordState,
} from "./esc-chord";

// useEscChord wires the pure chord reducer to real keystrokes and timers, and
// returns the state the keycast pill renders from.
//
// Keystrokes that happen inside the rendered document's iframe reach us the
// same way, because RenderFrame re-dispatches the bridge's forwarded keys as
// synthetic window events.
export function useEscChord({
  available,
  onRun,
  onEscape,
  enabled = true,
}: {
  available: ReadonlyArray<ChordKey>;
  onRun: (key: ChordKey) => void;
  onEscape: () => void;
  enabled?: boolean;
}): ChordState {
  const [state, setState] = React.useState<ChordState>(IDLE);

  // Latest values, read from a stable listener so re-registering the keydown
  // handler is never needed (and can never drop a keystroke mid-swap).
  const stateRef = React.useRef<ChordState>(IDLE);
  const availableRef = React.useRef(available);
  availableRef.current = available;
  const onRunRef = React.useRef(onRun);
  onRunRef.current = onRun;
  const onEscapeRef = React.useRef(onEscape);
  onEscapeRef.current = onEscape;
  const timerRef = React.useRef<number>(undefined);

  const dispatch = React.useCallback((event: ChordEvent) => {
    const { state: next, effect } = reduceChord(
      stateRef.current,
      event,
      availableRef.current,
    );
    if (next !== stateRef.current) {
      stateRef.current = next;
      setState(next);
    }
    window.clearTimeout(timerRef.current);
    timerRef.current = undefined;
    if (next.phase === "armed") {
      timerRef.current = window.setTimeout(
        () => dispatch({ type: "expire" }),
        CHORD_WINDOW_MS,
      );
    } else if (next.phase === "resolved") {
      timerRef.current = window.setTimeout(
        () => dispatch({ type: "clear" }),
        CHORD_RESOLVED_MS,
      );
    }
    if (effect?.type === "run") onRunRef.current(effect.key);
    if (effect?.type === "escape") onEscapeRef.current();
  }, []);

  React.useEffect(() => {
    if (!enabled) {
      dispatch({ type: "cancel" });
      return;
    }
    const onKeyDown = (event: KeyboardEvent) => {
      // While a text field or dialog owns Escape, leave the key entirely
      // alone: no arming, no delay, no pill.
      if (isChordBlocked(event.target)) {
        dispatch({ type: "cancel" });
        return;
      }
      dispatch({
        type: "keydown",
        key: event.key,
        modified: event.metaKey || event.ctrlKey || event.altKey,
      });
    };
    // Capture phase matters. Radix's dismissable layers listen on `document`
    // in the bubble phase and close synchronously (React flushes discrete
    // events), so a bubble-phase listener would see the dialog already gone
    // and arm a chord off the very Escape that dismissed it - which in full
    // screen then exited full screen 800ms later too. Running first means the
    // guard still sees the open dialog.
    window.addEventListener("keydown", onKeyDown, true);
    return () => {
      window.removeEventListener("keydown", onKeyDown, true);
      window.clearTimeout(timerRef.current);
    };
  }, [enabled, dispatch]);

  return state;
}
