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
