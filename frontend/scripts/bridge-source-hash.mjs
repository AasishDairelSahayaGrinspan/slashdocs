import { bridgeSourceHash } from "./bridge-source-files.mjs";

process.stdout.write(await bridgeSourceHash());
