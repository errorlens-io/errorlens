// Marks the build directories so Node knows which module format each uses.
import { writeFileSync } from "node:fs";
const dist = process.argv[2];
writeFileSync(`${dist}/cjs/package.json`, JSON.stringify({ type: "commonjs" }) + "\n");
writeFileSync(`${dist}/esm/package.json`, JSON.stringify({ type: "module" }) + "\n");
