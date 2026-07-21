import { afterEach } from "vitest";
import { cleanup } from "@testing-library/react";
import "@testing-library/jest-dom/vitest";

// This project doesn't set vitest's `globals: true`, so @testing-library/react
// can't auto-detect a global `afterEach` to hook its automatic DOM cleanup
// into (see its dist/index.js: `typeof afterEach === 'function'`). Without
// this, each test's rendered tree stays mounted into the next test in the
// same file, causing "found multiple elements" failures for any component
// test.
afterEach(() => {
  cleanup();
});

// jsdom implements no CSS media queries at all, so window.matchMedia is
// simply absent -- any component reaching useMediaQuery/useResolvedTheme
// throws on mount. Stand in a query list that never matches: tests assert on
// behaviour and accessible names, so the desktop layout is the one that
// matters, and nothing here flips a breakpoint mid-test.
if (!window.matchMedia) {
  window.matchMedia = (query: string) =>
    ({
      matches: false,
      media: query,
      onchange: null,
      addEventListener: () => {},
      removeEventListener: () => {},
      addListener: () => {},
      removeListener: () => {},
      dispatchEvent: () => false,
    }) as unknown as MediaQueryList;
}
