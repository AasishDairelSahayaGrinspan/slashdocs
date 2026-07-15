import { build } from "esbuild";
import { createHash } from "node:crypto";
import { readFile, writeFile } from "node:fs/promises";
import { dirname, resolve } from "node:path";
import { fileURLToPath } from "node:url";

const here = dirname(fileURLToPath(import.meta.url));
const frontendRoot = resolve(here, "..");
const sourcePath = resolve(frontendRoot, "bridge/bridge.ts");
const outputDirectory = resolve(frontendRoot, "../api/internal/server/bridge_dist");
const outputPath = resolve(outputDirectory, "bridge.js");
const source = await readFile(sourcePath);
const sourceHash = createHash("sha256").update(source).digest("hex");

await build({
  entryPoints: [sourcePath],
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
  outfile: outputPath,
});

const bundle = await readFile(outputPath);
const bundleHash = createHash("sha256").update(bundle).digest("hex");
await Promise.all([
  writeFile(resolve(outputDirectory, "bridge.source.sha256"), sourceHash + "\n"),
  writeFile(resolve(outputDirectory, "bridge.sha256"), bundleHash + "\n"),
]);
