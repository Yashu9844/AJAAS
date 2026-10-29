// Node checks for the pure logic of the product UI. Run from frontend/:  node src/modules/projects/__checks__/run.mjs
import assert from "node:assert/strict";
import { check, finish, load } from "./harness.mjs";

const errors = await load("api/errors.mjs");
const http = await load("api/http.mjs");
const query = await load("api/query.mjs");
const rank = await load("lib/rank-neighbours.mjs");
const opt = await load("lib/optimistic.mjs");
const dates = await load("lib/dates.mjs");
const mentions = await load("lib/mentions.mjs");
const paths = await load("lib/paths.mjs");
const format = await load("lib/format.mjs");
const jwt = await load("lib/jwt.mjs");

const json = (status, body, headers = {}) => new Response(status === 204 ? null : body === undefined ? "" : JSON.stringify(body), { status, headers });
function fakeHttp(responses, over = {}) {
  const calls = [];
  const state = { token: "t1", refreshed: 0, lost: 0, slept: [] };
  const deps = {
    baseUrl: () => "http://acme.localhost:8091",
    accessToken: () => state.token,
    refresh: async () => {
      state.refreshed++;
      if (over.refreshFails) return false;
      state.token = "t2";
      return true;
    },
    onSessionLost: () => state.lost++,
    sleep: async (ms) => void state.slept.push(ms),
    fetchFn: async (url, init) => {
      calls.push({ url, init });
      const next = responses.shift();
      if (next instanceof Error) throw next;
      return next;
    },
  };
  return { client: http.createHttp(deps), calls, state };
}

await check("errors: typed error from the API envelope", () => {
  const e = errors.parseErrorBody(409, JSON.stringify({ error: { code: "VERSION_CONFLICT", message: "changed", current_version: 7 } }));
  assert.equal(e.code, "VERSION_CONFLICT");
  assert.equal(e.details.currentVersion, 7);
  assert.ok(errors.isVersionConflict(e));
  assert.match(errors.describeError(e), /Someone else changed/);
});
await check("errors: non-JSON and 403/404 mapping", () => {
  const e = errors.parseErrorBody(502, "<html>bad gateway</html>");
  assert.equal(e.code, "HTTP_ERROR");
  assert.match(errors.describeError(errors.parseErrorBody(403, "{}")), /permission/);
  assert.match(errors.describeError(errors.parseErrorBody(404, "{}")), /not found/);
  assert.equal(errors.parseErrorBody(429, "{}", 3).details.retryAfterSeconds, 3);
});
await check("http: unwraps {data, meta} and sends bearer + tenant base URL", async () => {
  const { client, calls } = fakeHttp([json(200, { data: [{ id: 1 }], meta: { next_cursor: "abc", limit: 20 } })]);
  const res = await client.request("/projects", { query: { q: "a b", status: "active", empty: "" } });
  assert.deepEqual(res.data, [{ id: 1 }]);
  assert.equal(res.meta.next_cursor, "abc");
  assert.equal(calls[0].url, "http://acme.localhost:8091/api/v1/projects?q=a%20b&status=active");
  assert.equal(calls[0].init.headers.Authorization, "Bearer t1");
});
await check("http: 401 refreshes once and retries with the new token", async () => {
  const { client, calls, state } = fakeHttp([json(401, { error: { code: "TOKEN_EXPIRED", message: "x" } }), json(200, { data: { ok: true } })]);
  const res = await client.request("/my-work");
  assert.deepEqual(res.data, { ok: true });
  assert.equal(state.refreshed, 1);
  assert.equal(calls.length, 2);
  assert.equal(calls[1].init.headers.Authorization, "Bearer t2");
});
await check("http: failed refresh signals a lost session; no endless retry", async () => {
  const a = fakeHttp([json(401, {})], { refreshFails: true });
  await assert.rejects(a.client.request("/my-work"), (e) => e.code === "SESSION_LOST");
  assert.equal(a.state.lost, 1);
  const b = fakeHttp([json(401, {}), json(401, {})]);
  await assert.rejects(b.client.request("/my-work"), (e) => e.code === "SESSION_LOST");
  assert.equal(b.state.refreshed, 1);
  assert.equal(b.calls.length, 2);
});
await check("http: public calls never refresh", async () => {
  const { client, state } = fakeHttp([json(401, { error: { code: "INVALID_CREDENTIALS", message: "bad" } })]);
  await assert.rejects(client.request("/auth/login", { method: "POST", body: {}, noAuth: true }), (e) => e.code === "INVALID_CREDENTIALS");
  assert.equal(state.refreshed, 0);
});
await check("http: 429 waits for Retry-After and retries once", async () => {
  const { client, state, calls } = fakeHttp([json(429, {}, { "Retry-After": "4" }), json(200, { data: 1 })]);
  assert.equal((await client.request("/x")).data, 1);
  assert.deepEqual(state.slept, [4000]);
  assert.equal(calls.length, 2);
  assert.equal(http.retryAfterSeconds(null), 2);
  assert.equal(http.retryAfterSeconds("999"), 30);
  const again = fakeHttp([json(429, {}), json(429, { error: { code: "RATE_LIMITED", message: "slow" } })]);
  await assert.rejects(again.client.request("/x"), (e) => e.status === 429 && e.code === "RATE_LIMITED");
});
await check("http: network failure and empty bodies", async () => {
  const n = fakeHttp([new TypeError("fetch failed")]);
  await assert.rejects(n.client.request("/x"), (e) => e.code === "NETWORK_ERROR");
  const empty = fakeHttp([json(204)]);
  assert.equal((await empty.client.request("/x", { method: "DELETE" })).data, null);
});
await check("http: PATCH carries the version in the body and a stale one surfaces VERSION_CONFLICT", async () => {
  const { client, calls } = fakeHttp([json(409, { error: { code: "VERSION_CONFLICT", message: "m", current_version: 5 } })]);
  await assert.rejects(client.request("/tasks/1", { method: "PATCH", body: { title: "t", version: 3 } }), (e) => errors.isVersionConflict(e) && e.details.currentVersion === 5);
  assert.deepEqual(JSON.parse(calls[0].init.body), { title: "t", version: 3 });
});
await check("query: path segments cannot alter the route", () => {
  assert.equal(query.seg("a/b?c"), "a%2Fb%3Fc");
  assert.equal(query.buildQuery({ a: 1, b: false, c: true, d: undefined }), "?a=1&c=true");
});

await check("rank: drop target neighbours", () => {
  const ids = ["a", "b", "c"];
  assert.deepEqual(rank.computeDropTarget(ids, 0), { before_id: "a" });
  assert.deepEqual(rank.computeDropTarget(ids, 2), { before_id: "c" });
  assert.deepEqual(rank.computeDropTarget(ids, 3), { after_id: "c" });
  assert.deepEqual(rank.computeDropTarget(ids, 99), { after_id: "c" });
  assert.deepEqual(rank.computeDropTarget([], 0), {});
  assert.deepEqual(rank.computeDropTarget(ids, -4), { before_id: "a" });
});
await check("rank: pointer to insertion index", () => {
  assert.equal(rank.insertionIndexFromPointer([10, 30, 50], 5), 0);
  assert.equal(rank.insertionIndexFromPointer([10, 30, 50], 31), 2);
  assert.equal(rank.insertionIndexFromPointer([10, 30, 50], 500), 3);
  assert.equal(rank.insertionIndexFromPointer([], 5), 0);
});

const card = (id, col, cat = "todo") => ({ id, column_id: col, category: cat, version: 1, rank: id, title: id });
const board = () => ({
  project_id: "p",
  columns: [
    { id: "c1", name: "To do", category: "todo", wip_limit: 2, wip_hard: false, count: 3, tasks: [card("a", "c1"), card("b", "c1"), card("c", "c1")], next_cursor: null },
    { id: "c2", name: "Done", category: "done", wip_limit: null, wip_hard: false, count: 1, tasks: [card("d", "c2", "done")], next_cursor: null },
  ],
});
await check("optimistic: move between columns updates counts, category and leaves the snapshot intact", () => {
  const before = board();
  const snapshot = JSON.stringify(before);
  const after = opt.applyMove(before, "a", "c2", 0);
  assert.deepEqual(after.columns[1].tasks.map((t) => t.id), ["a", "d"]);
  assert.equal(after.columns[1].tasks[0].category, "done");
  assert.equal(after.columns[1].tasks[0].column_id, "c2");
  assert.deepEqual([after.columns[0].count, after.columns[1].count], [2, 2]);
  assert.equal(JSON.stringify(before), snapshot, "input must not be mutated (it is the rollback snapshot)");
});
await check("optimistic: reorder inside a column and unknown ids", () => {
  const after = opt.applyMove(board(), "a", "c1", 2);
  assert.deepEqual(after.columns[0].tasks.map((t) => t.id), ["b", "c", "a"]);
  assert.equal(after.columns[0].count, 3);
  assert.equal(opt.applyMove(board(), "zzz", "c1", 0), null);
  assert.equal(opt.applyMove(board(), "a", "nope", 0), null);
});
await check("optimistic: server values are copied back; WIP and paging helpers", () => {
  const moved = opt.applyMove(board(), "a", "c2", 0);
  const fixed = opt.reconcileMoved(moved, { id: "a", version: 4, rank: "R", completed_at: "2026-01-01T00:00:00Z", column_id: "c2", column_category: "done", updated_at: "u" });
  const c = opt.findCard(fixed, "a").card;
  assert.equal(c.version, 4);
  assert.equal(c.rank, "R");
  assert.ok(opt.isOverWipLimit(board().columns[0]));
  assert.ok(!opt.isOverWipLimit(board().columns[1]));
  const paged = opt.appendColumnPage(board(), "c2", [card("d", "c2"), card("e", "c2")], "next");
  assert.deepEqual(paged.columns[1].tasks.map((t) => t.id), ["d", "e"]);
  assert.equal(paged.columns[1].next_cursor, "next");
});

await check("dates: month grid is 42 cells starting on Monday and the window fits the 62-day limit", () => {
  const cells = dates.monthGrid(2026, 9);
  assert.equal(cells.length, 42);
  assert.equal(new Date(`${cells[0].key}T12:00:00`).getDay(), 1);
  assert.equal(cells.filter((c) => c.inMonth).length, 31);
  const w = dates.gridWindow(cells);
  assert.ok((new Date(w.to) - new Date(w.from)) / 86400000 <= 62);
  assert.deepEqual(dates.shiftMonth(2026, 0, -1), { year: 2025, month: 11 });
});
await check("dates: due date round trip and overdue rules", () => {
  assert.equal(dates.isoToDateKey(dates.dateKeyToDueAt("2026-10-20")), "2026-10-20");
  assert.equal(dates.dateKeyToDueAt("bad"), null);
  const now = new Date("2026-10-29T10:00:00Z").getTime();
  assert.ok(dates.isOverdue("2026-10-20T00:00:00Z", "todo", now));
  assert.ok(!dates.isOverdue("2026-10-20T00:00:00Z", "done", now));
  assert.ok(!dates.isOverdue(null, "todo", now));
});
await check("mentions: tokens stay plain markdown text", () => {
  const id = "5a0a266c-f926-459b-b42c-dc3f098d278a";
  const segs = mentions.splitMentions(`hi @[Ada L](user:${id}) and <b>x</b>`);
  assert.deepEqual(segs.map((s) => s.kind), ["text", "mention", "text"]);
  assert.equal(segs[1].userId, id);
  assert.equal(segs[2].text, " and <b>x</b>");
  assert.equal(mentions.mentionToken("A]b[", id), `@[Ab](user:${id})`);
});
await check("paths: redirect targets are same-site workspace paths only", () => {
  assert.equal(paths.safeNextPath("/workspace/projects"), "/workspace/projects");
  for (const bad of ["//evil.com", "https://evil.com", "/other", "/workspace/../x://y", "/workspace\\evil", null, "/workspace/sign-in"]) {
    assert.equal(paths.safeNextPath(bad), "/workspace", String(bad));
  }
  assert.equal(paths.taskPath("ABC-12"), "/workspace/tasks/ABC-12");
  assert.equal(paths.taskPath("../x"), "/workspace");
  assert.equal(paths.projectPath("not-an-id"), "/workspace/projects");
});
await check("format: minutes parsing and display", () => {
  assert.equal(format.parseMinutes("1h 30m"), 90);
  assert.equal(format.parseMinutes("45m"), 45);
  assert.equal(format.parseMinutes("90"), 90);
  assert.equal(format.parseMinutes("2h"), 120);
  assert.equal(format.parseMinutes("soon"), null);
  assert.equal(format.parseMinutes(""), null);
  assert.equal(format.formatMinutes(90), "1h 30m");
  assert.equal(format.formatMinutes(0), "0m");
});
await check("jwt: identity claims are read, garbage is refused", () => {
  const b64 = (o) => Buffer.from(JSON.stringify(o)).toString("base64url");
  const token = `${b64({ alg: "x" })}.${b64({ sub: "u1", email: "a@b.c", roles: ["tenant_admin"], exp: 2000000000 })}.sig`;
  assert.deepEqual(jwt.identityFromToken(token), { userId: "u1", email: "a@b.c", roles: ["tenant_admin"], expiresAtMs: 2000000000000 });
  assert.equal(jwt.identityFromToken("nonsense"), null);
});


await import("./run-planning.mjs");
finish();
