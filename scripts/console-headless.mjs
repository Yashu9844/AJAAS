// Headless runner for the test-console scenarios (docs/TEST_CONSOLE.md): runs the same scenario code the /dev-test
// pages run, against a live backend, without a browser.
//
// Usage (from the repo root, backend running on :8080, a tenant with a tenant_admin):
//   node scripts/console-headless.mjs <tenant-slug> <admin-email> <admin-password> [filter]
// Env: API_PORT (8080), PLATFORM_KEY (dev key). Scenarios needing two tenants (needs "AB") run only in the browser.
// The M0 negatives fail logins on purpose; repeated runs within 15 min trip the login limiter (clear ratelimit:* in dev Redis).
// It transpiles frontend/src/modules/devtest/*.ts with the project's TypeScript into a temp directory, maps
// <slug>.localhost to 127.0.0.1 (sending the right Host header), signs session A in and prints PASS/FAIL per step.
import { execFileSync } from "node:child_process";
import fs from "node:fs";
import http from "node:http";
import os from "node:os";
import path from "node:path";
import { createRequire } from "node:module";

const [slug, email, password, filter = ""] = process.argv.slice(2);
if (!slug || !email || !password) {
  console.error("usage: node scripts/console-headless.mjs <tenant-slug> <admin-email> <admin-password> [filter]");
  process.exit(2);
}
const port = process.env.API_PORT ?? "8080";
const repo = path.resolve(path.dirname(new URL(import.meta.url).pathname.replace(/^\/([A-Za-z]:)/, "$1")), "..");
const front = path.join(repo, "frontend");
const out = fs.mkdtempSync(path.join(os.tmpdir(), "devtest-"));
const src = path.join(front, "src", "modules", "devtest");
const files = ["types.ts", "api.ts", "store.ts", "runner.ts", "scenarios0.ts", "scenariosAccess.ts", "scenarios1.ts", "scenarios2.ts", "scenariosHr.ts"].map((f) => path.join(src, f));
execFileSync(process.execPath, [path.join(front, "node_modules", "typescript", "bin", "tsc"), "--outDir", out, "--module", "commonjs",
  "--target", "es2020", "--esModuleInterop", "--skipLibCheck", "--strict", "--moduleResolution", "node", ...files], { stdio: "inherit" });
fs.symlinkSync(path.join(front, "node_modules"), path.join(out, "node_modules"), "junction");

// fetch over node:http so <slug>.localhost reaches 127.0.0.1 with the right Host header.
globalThis.fetch = (url, init = {}) =>
  new Promise((resolve, reject) => {
    const u = new URL(url);
    const headers = { ...(init.headers ?? {}), Host: u.host };
    const req = http.request({ host: "127.0.0.1", port: u.port, path: u.pathname + u.search, method: init.method ?? "GET", headers }, (res) => {
      const chunks = [];
      res.on("data", (c) => chunks.push(c));
      res.on("end", () => {
        const body = Buffer.concat(chunks).toString("utf8");
        resolve({ status: res.statusCode, statusText: res.statusMessage, ok: res.statusCode < 400,
          headers: { get: (k) => res.headers[k.toLowerCase()] ?? null }, text: async () => body, json: async () => JSON.parse(body) });
      });
    });
    req.on("error", reject);
    if (init.body) req.write(init.body);
    req.end();
  });

const require = createRequire(path.join(out, "index.js"));
const { getState, setSession } = require(path.join(out, "store.js"));
const { runScenario } = require(path.join(out, "runner.js"));
const { S0 } = require(path.join(out, "scenarios0.js"));
const { S0_ACCESS } = require(path.join(out, "scenariosAccess.js"));
const { S1 } = require(path.join(out, "scenarios1.js"));
const { S2 } = require(path.join(out, "scenarios2.js"));
const hr = require(path.join(out, "scenariosHr.js"));

const login = await fetch(`http://localhost:${port}/api/v1/auth/login`, { method: "POST", headers: { "Content-Type": "application/json" },
  body: JSON.stringify({ tenant_slug: slug, email, password }) });
const lb = await login.json();
if (login.status !== 200) {
  console.error("login failed", lb);
  process.exit(1);
}
setSession("A", { slug, accessToken: lb.data.access_token, refreshToken: lb.data.refresh_token, expiresAt: lb.data.expires_at, user: lb.data.user, tenantId: lb.data.tenant_id });
const ep = { host: "localhost", port, platformKey: process.env.PLATFORM_KEY ?? "dev-platform-admin-key-change-me" };
const all = [...S0.filter((s) => s.needs !== "AB"), ...S0_ACCESS, ...S1.filter((s) => s.needs !== "AB"), ...S2.filter((s) => s.needs !== "AB"), ...hr.S_ATTENDANCE, ...hr.S_LEAVE, ...hr.S_PAYROLL, ...hr.S_RECRUITMENT, ...hr.S_PERFORMANCE, ...hr.S_CORE, ...hr.S_EMPLOYEE_RECORDS]
  .filter((s) => !filter || s.key.includes(filter));
let failed = 0;
for (const sc of all) {
  const steps = await runScenario(sc, ep, () => {});
  const bad = steps.filter((s) => s.state === "fail");
  failed += bad.length ? 1 : 0;
  console.log(`${bad.length ? "FAIL" : "PASS"}  ${sc.key}  ${sc.title}  (${steps.length} steps)`);
  for (const s of bad) console.log(`      ✗ ${s.name}: expected ${s.expected}, got ${s.actual} — ${s.note}`);
}
console.log(`\n${all.length - failed}/${all.length} scenarios passed`);
void getState;
process.exit(failed ? 1 : 0);
