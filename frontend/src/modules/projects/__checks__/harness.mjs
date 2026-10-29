// Shared harness for the node checks of the pure logic of the product UI (API client, errors, optimistic board moves, rank neighbours, dates,
// mentions, paths). No framework: the TypeScript sources are transpiled with the repo's own typescript package into a
// temp folder and exercised with node:assert.
// Run from frontend/:  node src/modules/projects/__checks__/run.mjs
import fs from "node:fs";
import os from "node:os";
import path from "node:path";
import { createRequire } from "node:module";
import { pathToFileURL, fileURLToPath } from "node:url";

const here = path.dirname(fileURLToPath(import.meta.url));
const root = path.resolve(here, "..");
const require = createRequire(path.resolve(here, "../../../../package.json"));
const ts = require("typescript");

const out = fs.mkdtempSync(path.join(os.tmpdir(), "pm-checks-"));
for (const dir of ["api", "lib", "types"]) {
  fs.mkdirSync(path.join(out, dir), { recursive: true });
  for (const file of fs.readdirSync(path.join(root, dir)).filter((f) => f.endsWith(".ts"))) {
    const src = fs.readFileSync(path.join(root, dir, file), "utf8");
    let js = ts.transpileModule(src, { compilerOptions: { module: ts.ModuleKind.ESNext, target: ts.ScriptTarget.ES2022 } }).outputText;
    js = js.replace(/(from\s+["'])(\.{1,2}\/[^"']+)(["'])/g, "$1$2.mjs$3");
    fs.writeFileSync(path.join(out, dir, file.replace(/\.ts$/, ".mjs")), js);
  }
}
export const load = (rel) => import(pathToFileURL(path.join(out, rel)).href);
export const say = (line) => process.stdout.write(`${line}
`);
let passed = 0;
export const check = async (name, fn) => {
  try {
    await fn();
    passed++;
    say(`ok   ${name}`);
  } catch (e) {
    say(`FAIL ${name}\n${e.stack}`);
    process.exitCode = 1;
  }
};


/** Removes the temp folder and prints the total. */
export function finish() {
  fs.rmSync(out, { recursive: true, force: true });
  say(`
${passed} checks passed${process.exitCode ? " (with failures)" : ""}`);
}

