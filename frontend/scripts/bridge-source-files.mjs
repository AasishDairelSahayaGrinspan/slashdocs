import { createHash } from "node:crypto";
import { readFile, readdir } from "node:fs/promises";
import { dirname, relative, resolve, sep } from "node:path";
import { fileURLToPath } from "node:url";

const here = dirname(fileURLToPath(import.meta.url));
const bridgeRoot = resolve(here, "../bridge");

// Fingerprints the sandbox bridge so the server can detect a stale bundle:
// every regular file under frontend/bridge/ (any extension esbuild might
// bundle) plus the resolved version of every production npm dependency,
// since those are bundled into bridge.js too. Each entry contributes its
// POSIX relative path/name and bytes, NUL-separated, in sorted order so the
// digest is deterministic across platforms.
export async function bridgeSourceHash() {
  const entries = await readdir(bridgeRoot, { recursive: true, withFileTypes: true });
  const paths = [];
  for (const entry of entries) {
    if (entry.isSymbolicLink()) {
      // A symlink would be bundled by esbuild but silently skipped by
      // isFile(), letting its edits bypass the fingerprint.
      throw new Error(
        `refusing to fingerprint symlink under frontend/bridge/: ${entry.name}`,
      );
    }
    if (!entry.isFile()) continue;
    paths.push(
      relative(bridgeRoot, resolve(entry.parentPath, entry.name)).split(sep).join("/"),
    );
  }
  paths.sort();

  const hash = createHash("sha256");
  for (const path of paths) {
    hash.update(path);
    hash.update("\0");
    hash.update(await readFile(resolve(bridgeRoot, path)));
    hash.update("\0");
  }

  // Mix in resolved production dependency versions from the lockfile: a
  // dependency bump changes the shipped bundle without touching any bridge
  // source, and must invalidate an outdated bundle the same way.
  const lock = JSON.parse(await readFile(resolve(here, "../package-lock.json"), "utf8"));
  const dependencyVersions = Object.entries(lock.packages || {})
    .filter(([name, info]) => name.startsWith("node_modules/") && !info.dev)
    .map(([name, info]) => `${name}@${info.version || ""}`)
    .sort();
  for (const dependency of dependencyVersions) {
    hash.update(dependency);
    hash.update("\0");
  }
  return hash.digest("hex");
}
