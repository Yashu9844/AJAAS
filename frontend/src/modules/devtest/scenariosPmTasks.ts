import { getPath } from "./api";
import { str, type Runner, type Scenario } from "./runner";
import { login, mkUser, PASSWORD, permId, tn } from "./scenarios0";
import type { CallSpec, Exchange, Session } from "./types";

// M2 task core scenarios (docs/TEST_CONSOLE.md): TK-01..TK-14 and the goldens PG1-PG5, PG12 scripted against the live backend.
// Each run creates its own users and projects in session A's tenant (A must be a tenant_admin).

const list = (ex: Exchange, path = "data") => (getPath(ex.resBody, path) as Record<string, unknown>[] | undefined) ?? [];
const num = (ex: Exchange, path: string) => Number(getPath(ex.resBody, path));
const withHeader = (c: CallSpec, headers: Record<string, string>): CallSpec => ({ ...c, headers: { ...(c.headers ?? {}), ...headers } });

/** A fresh user holding exactly the given tenant permissions, logged in. */
async function limited(r: Runner, A: Session, uid: string, tag: string, perms: [string, string][]): Promise<{ S: Session; userId: string }> {
  const ids: string[] = [];
  for (const [res, act] of perms) ids.push(await permId(r, A, res, act));
  const role = await r.call(`Role {${perms.map((p) => p.join(":")).join(", ") || "nothing"}}`, tn(A, "POST", "/roles", { name: `dt-${tag}-${uid}`, permission_ids: ids }), 201);
  const u = await mkUser(r, A, uid, tag, [str(role, "data.id")]);
  const L = await login(r, `Login ${tag}`, A.slug, u.email, PASSWORD);
  return { S: { ...A, accessToken: L.at }, userId: u.id };
}

interface Fixture {
  owner: Session;
  member: Session;
  viewer: Session;
  memberId: string;
  pid: string;
  todo: string;
  done: string;
  prog: string;
}

/** A project (key from uid) with an owner, a member and a viewer, plus its todo/done/in-progress column ids. */
async function fixture(r: Runner, A: Session, uid: string, prefix: string): Promise<Fixture> {
  const key = `${prefix}${uid}`.toUpperCase().replace(/[^A-Z0-9]/g, "").slice(0, 10);
  const owner = await limited(r, A, uid, `${prefix}o`, [["projects", "create"]]);
  const member = await limited(r, A, uid, `${prefix}m`, []);
  const viewer = await limited(r, A, uid, `${prefix}v`, []);
  const p = await r.call("Create the project", tn(owner.S, "POST", "/projects", { key, name: `Tasks ${uid}` }), 201);
  const pid = str(p, "data.id");
  await r.call("Add a member", tn(owner.S, "POST", `/projects/${pid}/members`, { user_id: member.userId, role: "member" }), 201);
  await r.call("Add a viewer", tn(owner.S, "POST", `/projects/${pid}/members`, { user_id: viewer.userId, role: "viewer" }), 201);
  const cols = await r.call("Columns", tn(owner.S, "GET", `/projects/${pid}/columns`), 200);
  const col = (k: string) => String(list(cols).find((c) => c.key === k)?.id ?? "");
  r.need(col("todo") && col("done"), "default columns missing");
  return { owner: owner.S, member: member.S, viewer: viewer.S, memberId: member.userId, pid, todo: col("todo"), done: col("done"), prog: col("in-progress") };
}

export const S_PM_TASKS: Scenario[] = [
  {
    key: "pm.tasks.core",
    module: "projects",
    title: "Task create, idempotency, versions, move, ticket lookup, archive (PG1, PG2, PG4, TK-01..TK-12)",
    description:
      "Creates a task with a ticket number, replays the create with the same Idempotency-Key and gets the original (PG1), edits with If-Match and gets VERSION_CONFLICT with the current version for a stale edit (TK-03), moves it to done (completed_at set), reorders, looks it up by ticket id, hides it from strangers and other tenants (PG4), archives, restores and deletes it.",
    needs: "A",
    run: async (r, { A, uid }) => {
      const f = await fixture(r, A, uid, "T");
      const base = `/projects/${f.pid}/tasks`;
      const stranger = await limited(r, A, uid, "Ts", []);
      const idem = `idem-${uid}`;
      const first = await r.call("Create a task", withHeader(tn(f.member, "POST", base, { title: "Ship it", priority: "high" }), { "Idempotency-Key": idem }), 201, (e) =>
        str(e, "data.key").endsWith("-1") && str(e, "data.column_category") === "todo" ? null : "expected ticket number 1 in a todo column",
      );
      const id = str(first, "data.id");
      await r.call("Replay with the same Idempotency-Key returns the original (PG1)", withHeader(tn(f.member, "POST", base, { title: "Ship it", priority: "high" }), { "Idempotency-Key": idem }), 201, (e) =>
        str(e, "data.id") === id ? null : "the replay must return the first task",
      );
      await r.call("A viewer cannot create → 403", tn(f.viewer, "POST", base, { title: "x" }), 403);
      await r.call("An empty title → 400", tn(f.member, "POST", base, { title: "" }), 400);
      await r.call("A stranger cannot read it → 404 (PG4)", tn(stranger.S, "GET", `/tasks/${id}`), 404);
      await r.call("Ticket lookup is case-insensitive", tn(f.viewer, "GET", `/tickets/${str(first, "data.key").toLowerCase()}`), 200, (e) => (str(e, "data.id") === id ? null : "wrong task"));
      await r.call("Edit with If-Match", withHeader(tn(f.member, "PATCH", `/tasks/${id}`, { title: "Shipped" }), { "If-Match": '"1"' }), 200, (e) =>
        num(e, "data.version") === 2 ? null : "version should be 2",
      );
      await r.call("Stale version → 409 VERSION_CONFLICT (PG2)", withHeader(tn(f.member, "PATCH", `/tasks/${id}`, { title: "Late" }), { "If-Match": "1" }), 409, (e) =>
        num(e, "error.current_version") === 2 ? null : "error must carry current_version 2",
      );
      await r.call("The version is required → 400", tn(f.member, "PATCH", `/tasks/${id}`, { title: "x" }), 400);
      await r.call("Move to done sets completed_at", tn(f.member, "POST", `/tasks/${id}/move`, { column_id: f.done }), 200, (e) =>
        str(e, "data.completed_at") ? null : "completed_at must be set",
      );
      const second = await r.call("A second task in done", tn(f.member, "POST", base, { title: "Second", column_id: f.done }), 201);
      await r.call("Reorder after the second", tn(f.member, "POST", `/tasks/${id}/reorder`, { after_id: str(second, "data.id") }), 200);
      await r.call("Reorder without a neighbour → 400", tn(f.member, "POST", `/tasks/${id}/reorder`, {}), 400);
      await r.call("Archive", tn(f.member, "POST", `/tasks/${id}/archive`), 200);
      await r.call("Archived tasks leave the default list", tn(f.member, "GET", base), 200, (e) => (list(e).length === 1 ? null : "expected one live task"));
      await r.call("Restore", tn(f.member, "POST", `/tasks/${id}/restore`), 200);
      await r.call("Duplicate", tn(f.member, "POST", `/tasks/${id}/duplicate`, { copy_labels: true }), 201, (e) => (str(e, "data.title").endsWith("(copy)") ? null : "title suffix"));
      await r.call("A member cannot delete → 403", tn(f.member, "DELETE", `/tasks/${id}`), 403);
      await r.call("The owner deletes", tn(f.owner, "DELETE", `/tasks/${id}`), 200);
      await r.call("Deleted → 404", tn(f.owner, "GET", `/tasks/${id}`), 404);
    },
  },
  {
    key: "pm.tasks.list",
    module: "projects",
    title: "Filters, sorts and keyset paging (TK-13, PG5, PG7)",
    description:
      "Creates twelve tasks concurrently and checks that their numbers are unique and gapless (PG5), pages the project with limit 5 and a cursor without gaps or repeats, filters by priority and rejects an unknown sort.",
    needs: "A",
    run: async (r, { A, uid }) => {
      const f = await fixture(r, A, uid, "L");
      const base = `/projects/${f.pid}/tasks`;
      const made = await Promise.all(
        Array.from({ length: 12 }, (_, i) =>
          r.call(`Create t${i}`, tn(f.member, "POST", base, { title: `t${i}`, priority: i % 2 ? "high" : "low" }), 201),
        ),
      );
      const numbers = made.map((e) => num(e, "data.number")).sort((a, b) => a - b);
      r.need(numbers.join() === Array.from({ length: 12 }, (_, i) => i + 1).join(), `numbers must be 1..12 once each, got ${numbers.join()}`);
      const seen = new Set<string>();
      let cursor = "";
      for (let page = 0; page < 10; page++) {
        const q: Record<string, string> = { limit: "5", sort: "number" };
        if (cursor) q.cursor = cursor;
        const ex = await r.call(`Page ${page + 1}`, tn(f.viewer, "GET", base, undefined, q), 200);
        for (const t of list(ex)) {
          const tid = String(t.id);
          r.need(!seen.has(tid), "a task appeared on two pages");
          seen.add(tid);
        }
        cursor = str(ex, "meta.next_cursor");
        if (!cursor) break;
      }
      r.need(seen.size === 12, `paging returned ${seen.size} of 12 tasks`);
      await r.call("Filter by priority", tn(f.viewer, "GET", base, undefined, { priority: "high" }), 200, (e) => (list(e).length === 6 ? null : "expected 6 high tasks"));
      await r.call("An unknown sort → 400", tn(f.viewer, "GET", base, undefined, { sort: "nope" }), 400);
    },
  },
  {
    key: "pm.tasks.relations",
    module: "projects",
    title: "Relations, dependency cycles, blockers and subtasks (PG3, TK-06, TK-07)",
    description:
      "A blocks B; B blocking A is refused with DEPENDENCY_CYCLE (PG3). A blocked task cannot move to done, a member cannot force it, the owner can. A parent with an open subtask cannot be completed until the subtask is done, and progress rolls up.",
    needs: "A",
    run: async (r, { A, uid }) => {
      const f = await fixture(r, A, uid, "R");
      const base = `/projects/${f.pid}/tasks`;
      const a = str(await r.call("Task a", tn(f.member, "POST", base, { title: "a" }), 201), "data.id");
      const b = str(await r.call("Task b", tn(f.member, "POST", base, { title: "b" }), 201), "data.id");
      await r.call("a blocks b", tn(f.member, "POST", `/tasks/${a}/relations`, { target_task_id: b, type: "blocks" }), 201);
      await r.call("b blocks a → 409 DEPENDENCY_CYCLE (PG3)", tn(f.member, "POST", `/tasks/${b}/relations`, { target_task_id: a, type: "blocks" }), 409, (e) =>
        str(e, "error.code") === "DEPENDENCY_CYCLE" ? null : "expected DEPENDENCY_CYCLE",
      );
      await r.call("The same relation again → 409", tn(f.member, "POST", `/tasks/${a}/relations`, { target_task_id: b, type: "blocks" }), 409);
      await r.call("b shows as blocked", tn(f.viewer, "GET", `/tasks/${b}`), 200, (e) => (getPath(e.resBody, "data.blocked") === true ? null : "blocked must be true"));
      await r.call("A blocked task cannot be completed → 409", tn(f.member, "POST", `/tasks/${b}/move`, { column_id: f.done }), 409);
      await r.call("A member cannot force → 403", tn(f.member, "POST", `/tasks/${b}/move`, { column_id: f.done, force: true }), 403);
      await r.call("The owner forces it", tn(f.owner, "POST", `/tasks/${b}/move`, { column_id: f.done, force: true }), 200);
      const parent = str(await r.call("Parent", tn(f.member, "POST", base, { title: "parent" }), 201), "data.id");
      const kid = str(await r.call("Subtask", tn(f.member, "POST", base, { title: "kid", parent_task_id: parent }), 201), "data.id");
      await r.call("Open subtasks block completion → 409", tn(f.member, "POST", `/tasks/${parent}/move`, { column_id: f.done }), 409);
      await r.call("Finish the subtask", tn(f.member, "POST", `/tasks/${kid}/move`, { column_id: f.done }), 200);
      await r.call("The parent completes with progress 100", tn(f.member, "POST", `/tasks/${parent}/move`, { column_id: f.done }), 200, (e) =>
        num(e, "data.progress") === 100 ? null : "progress must be 100",
      );
      await r.call("Subtasks list", tn(f.viewer, "GET", `/tasks/${parent}/subtasks`), 200, (e) => (list(e).length === 1 ? null : "one subtask"));
    },
  },
  {
    key: "pm.tasks.bulk",
    module: "projects",
    title: "Assignees, bulk update, move to another project, activity (TK-08, TK-09, CM-04, PG12)",
    description:
      "Assigns a member (the view exposes the active flag), bulk-updates one real and one unknown task and gets a per-task result list, moves a task to another project (new ticket number, the old ticket id still resolves) and reads the task and project activity with a cursor.",
    needs: "A",
    run: async (r, { A, uid }) => {
      const f = await fixture(r, A, uid, "B");
      const base = `/projects/${f.pid}/tasks`;
      const a = str(await r.call("Task a", tn(f.member, "POST", base, { title: "a" }), 201), "data.id");
      await r.call("Assign the member", tn(f.member, "POST", `/tasks/${a}/assignees`, { user_ids: [f.memberId] }), 200, (e) =>
        getPath(e.resBody, "data.assignees.0.active") === true ? null : "assignees expose the active flag",
      );
      await r.call("A stranger as assignee → 400", tn(f.member, "POST", `/tasks/${a}/assignees`, { user_ids: ["00000000-0000-4000-8000-000000000000"] }), 400);
      await r.call("Unassign", tn(f.member, "DELETE", `/tasks/${a}/assignees/${f.memberId}`), 200);
      await r.call("Bulk: one succeeds, one fails", tn(f.member, "POST", `${base}/bulk`, { task_ids: [a, "00000000-0000-4000-8000-000000000000"], priority: "urgent" }), 200, (e) =>
        num(e, "data.succeeded") === 1 && num(e, "data.failed") === 1 ? null : "expected 1 success and 1 failure",
      );
      const dst = await r.call("A second project", tn(f.owner, "POST", "/projects", { key: `D${uid}`.toUpperCase().replace(/[^A-Z0-9]/g, "").slice(0, 10), name: "Destination" }), 201);
      const moved = await r.call("Move the task to the second project", tn(f.owner, "POST", `/tasks/${a}/move-project`, { project_id: str(dst, "data.id") }), 200, (e) =>
        str(e, "data.key").endsWith("-1") ? null : "new number in the target",
      );
      const oldKey = str(await r.call("Read the old key", tn(f.owner, "GET", `/projects/${f.pid}`), 200), "data.key") + "-1";
      await r.call("The old ticket id still resolves", tn(f.owner, "GET", `/tickets/${oldKey}`), 200, (e) => (str(e, "data.id") === str(moved, "data.id") ? null : "alias must resolve"));
      await r.call("Task activity (oldest first)", tn(f.owner, "GET", `/tasks/${a}/activity`, undefined, { order: "asc" }), 200, (e) =>
        list(e).length >= 4 && String(list(e)[0].type) === "created" ? null : "expected the history from created",
      );
      await r.call("Project activity pages with a cursor", tn(f.owner, "GET", `/projects/${f.pid}/activity`, undefined, { limit: "2" }), 200, (e) =>
        list(e).length === 2 && str(e, "meta.next_cursor") ? null : "expected 2 rows and a next_cursor",
      );
    },
  },
];
