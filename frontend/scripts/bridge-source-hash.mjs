import { createHash } from "node:crypto";
import { readFile } from "node:fs/promises";
import { dirname, resolve } from "node:path";
import { fileURLToPath } from "node:url";

const here = dirname(fileURLToPath(import.meta.url));
const source = await readFile(resolve(here, "../bridge/bridge.ts"));
process.stdout.write(createHash("sha256").update(source).digest("hex"));
