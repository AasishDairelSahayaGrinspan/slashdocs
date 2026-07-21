import { defineConfig, mergeConfig } from "vitest/config";
import react from "@vitejs/plugin-react";
import viteConfig from "./vite.config";

// Vitest ignores vite.config.ts entirely once this file exists, so anything
// shared (notably the `@` alias) is inherited rather than restated -- a
// hand-copied alias is the classic "tests pass while the app breaks" vector.
// Plugins are the deliberate exception: the test run keeps @vitejs/plugin-react
// and drops Tailwind, since these tests assert on behaviour and accessible
// names, never on computed styles.
export default mergeConfig(
  { ...viteConfig, plugins: [] },
  defineConfig({
    plugins: [react()],
    test: {
      environment: "jsdom",
      setupFiles: ["./src/test/setup.ts"],
      include: ["src/**/*.{test,spec}.{ts,tsx}"],
      // restoreMocks only unwinds vi.spyOn; clearMocks also resets the
      // vi.fn()s inside vi.mock factories, which otherwise accumulate calls
      // across tests in a file and let one test assert on another's call.
      clearMocks: true,
      restoreMocks: true,
    },
  }),
);
