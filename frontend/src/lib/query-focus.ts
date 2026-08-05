// TanStack Query's focus manager listens to `visibilitychange` only, so a tab
// left visible on a second monitor never refetches. Adding window `focus`
// covers "clicked back into the window".
//
// The wrapper drops the argument on purpose: a boolean would route to
// focusManager.setFocused() and permanently pin isFocused() to that value; no
// argument routes to onFocus(), a plain "check now" nudge.
export function focusEventSetup(target: EventTarget) {
  return (onFocus: () => void) => {
    const listener = () => onFocus();
    target.addEventListener("visibilitychange", listener, false);
    target.addEventListener("focus", listener, false);
    return () => {
      target.removeEventListener("visibilitychange", listener, false);
      target.removeEventListener("focus", listener, false);
    };
  };
}
