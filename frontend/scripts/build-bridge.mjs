import { build } from "esbuild";
import { dirname, resolve } from "node:path";
import { fileURLToPath } from "node:url";

const here = dirname(fileURLToPath(import.meta.url));
const frontendRoot = resolve(here, "..");

await build({
  entryPoints: [resolve(frontendRoot, "bridge/bridge.ts")],
  bundle: true,
  platform: "browser",
  format: "iife",
  target: "es2022",
  // The legacy classic script runs in sloppy mode. Frontend TypeScript's
  // strict type-checking must not make esbuild inject a runtime strict-mode
  // directive and change that behavior.
  tsconfigRaw: { compilerOptions: { alwaysStrict: false } },
  minifyWhitespace: true,
  minifySyntax: true,
  // Keep identifiers stable so the erased TypeScript port can be compared
  // exactly with the legacy bridge after both pass through this build.
  minifyIdentifiers: false,
  legalComments: "none",
  sourcemap: false,
  outfile: resolve(frontendRoot, "../api/internal/server/bridge_dist/bridge.js"),
});
