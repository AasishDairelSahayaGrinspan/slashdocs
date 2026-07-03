import { describe, expect, test } from "vitest";
import { readFileSync, readdirSync, existsSync, statSync } from "node:fs";
import { join, dirname } from "node:path";
import { fileURLToPath } from "node:url";

const frontendRoot = join(dirname(fileURLToPath(import.meta.url)), "..", "..");
const publicDir = join(frontendRoot, "public");
const srcDir = join(frontendRoot, "src");

function walk(dir: string): string[] {
  return readdirSync(dir).flatMap((entry) => {
    const full = join(dir, entry);
    if (statSync(full).isDirectory()) return walk(full);
    if (/\.test\.tsx?$/.test(entry)) return [];
    return /\.(tsx|ts|html)$/.test(entry) ? [full] : [];
  });
}

function rootAssetRefs(content: string): string[] {
  // matches src="/x.svg" and href="/x.svg" style references to public assets
  return [...content.matchAll(/(?:src|href)="(\/[^"]+\.(?:svg|png|ico))"/g)].map(
    (m) => m[1],
  );
}

describe("icon assets", () => {
  test("every icon referenced in source resolves to a file in public/", () => {
    const files = [join(frontendRoot, "index.html"), ...walk(srcDir)];
    const missing: string[] = [];
    for (const file of files) {
      for (const ref of rootAssetRefs(readFileSync(file, "utf8"))) {
        if (!existsSync(join(publicDir, ref))) {
          missing.push(`${file}: ${ref}`);
        }
      }
    }
    expect(missing).toEqual([]);
  });

  test("favicon.svg is the sparkdoc icon, not the legacy document icon", () => {
    const favicon = readFileSync(join(publicDir, "favicon.svg"), "utf8");
    expect(favicon).toContain("<svg");
    // legacy icon was drawn in #5b7fff; sparkdoc palette is #1364F1 / #0E54CD
    expect(favicon.toLowerCase()).not.toContain("#5b7fff");
    expect(favicon).toContain("#1364F1");
  });

  test("logo.svg (detailed sparkdoc) exists for in-app use", () => {
    const logoPath = join(publicDir, "logo.svg");
    expect(existsSync(logoPath)).toBe(true);
    const logo = readFileSync(logoPath, "utf8");
    expect(logo).toContain("<svg");
    expect(logo).toContain("linearGradient");
  });

  test("large in-app brand marks use logo.svg, small/tab icons use favicon.svg", () => {
    const topbar = readFileSync(
      join(srcDir, "components", "layout", "topbar.tsx"),
      "utf8",
    );
    const login = readFileSync(join(srcDir, "pages", "login.tsx"), "utf8");
    const indexHtml = readFileSync(join(frontendRoot, "index.html"), "utf8");

    expect(topbar).toContain('src="/logo.svg"');
    expect(login).toContain('src="/logo.svg"');
    expect(indexHtml).toContain('href="/favicon.svg"');
  });
});
