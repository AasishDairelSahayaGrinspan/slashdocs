import { defineConfig } from "vitest/config";
import react from "@vitejs/plugin-react";
import tailwindcss from "@tailwindcss/vite";
import path from "node:path";

export default defineConfig({
  plugins: [react(), tailwindcss()],
  resolve: {
    alias: { "@": path.resolve(__dirname, "./src") },
  },
  server: {},
  test: {
    // Bridge tests live in tests/, because every file under bridge/ is part of
    // the bridge source hash.
    include: ["src/**/*.test.{ts,tsx}", "tests/**/*.test.ts"],
    environment: "jsdom",
    setupFiles: "./src/test/setup.ts",
    restoreMocks: true,
  },
});
