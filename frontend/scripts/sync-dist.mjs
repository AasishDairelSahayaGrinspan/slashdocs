import { cpSync, existsSync, mkdirSync, readdirSync, rmSync } from "node:fs";
import { dirname, join, resolve } from "node:path";
import { fileURLToPath } from "node:url";

const here = dirname(fileURLToPath(import.meta.url));
const frontendRoot = resolve(here, "..");
const src = join(frontendRoot, "dist");
const dest = resolve(frontendRoot, "../api/internal/server/frontend_dist");
const assetDest = join(dest, "assets");

if (!existsSync(src)) {
  throw new Error(`missing frontend dist directory: ${src}`);
}

mkdirSync(assetDest, { recursive: true });

for (const entry of readdirSync(dest)) {
  if (entry === ".gitkeep" || entry === "assets") continue;
  rmSync(join(dest, entry), { recursive: true, force: true });
}

for (const entry of readdirSync(assetDest)) {
  if (entry === ".gitkeep") continue;
  rmSync(join(assetDest, entry), { recursive: true, force: true });
}

cpSync(src, dest, { recursive: true });
