import { getPath } from "./api";
import { str, type Runner, type Scenario } from "./runner";
import { gl, login, mkUser, PASSWORD, permId, tn } from "./scenarios0";
import type { Exchange, Session } from "./types";

// M2 automation scenarios (docs/TEST_CONSOLE.md): workflow rules, recurring tasks and public project links, scripted against
// the live backend. Each run creates its own users and project in session A's tenant (A must be a tenant_admin). The rule
// engine reads the project outbox every 5 s (after a 3 s settle delay), so the rule scenario waits for it.

const list = (ex: Exchange, path = "data") => (getPath(ex.resBody, path) as Record<string, unknown>[] | undefined) ?? [];
const sleep = (ms: number) => new Promise((resolve) => setTimeout(resolve, ms));

async function projectOwner(r: Runner, A: Session, uid: string): Promise<{ s: Session; id: string }> {
  const create = await permId(r, A, "projects", "create");
  const role = await r.call("Role {projects:create}", tn(A, "POST", "/roles", { name: `dt-pma-${uid}`, permission_ids: [create] }), 201);
  const u = await mkUser(r, A, uid, "pma", [str(role, "data.id")]);
  const L = await login(r, "Login owner", A.slug, u.email, PASSWORD);
  return { s: { ...A, accessToken: L.at }, id: u.id };
}

async function columnOf(r: Runner, O: Session, pid: string, category: string): Promise<string> {
  const cols = await r.call("List the columns", tn(O, "GET", `/projects/${pid}/columns`), 200);
  return str({ ...cols, resBody: { data: list(cols).find((c) => c.category === category) } }, "data.id");
}

export const S_PM_AUTOMATION: Scenario[] = [
  {
    key: "pm.automation.rules",
    module: "projects",
    title: "Workflow rule: an urgent task is moved to Done after a move, without cascading (phase 4)",
    description:
      "Creates a rule (when a task is moved and its priority is urgent, move it to Done), refuses unknown actions and non-managers, dry-runs it, moves an urgent and a low task by hand, waits for the engine (up to ~30 s) and shows that only the urgent task was moved, once, with one ok run.",
    needs: "A",
    run: async (r, { A, uid }) => {
      const { s: O } = await projectOwner(r, A, uid);
      const member = await mkUser(r, A, uid, "pmam");
      const ML = await login(r, "Login member", A.slug, member.email, PASSWORD);
      const key = `A${uid}`.toUpperCase().replace(/[^A-Z0-9]/g, "").slice(0, 10);
      const p = await r.call("Create a project", tn(O, "POST", "/projects", { key, name: `Automation ${uid}` }), 201);
      const pid = str(p, "data.id");
      await r.call("Add a member", tn(O, "POST", `/projects/${pid}/members`, { user_id: member.id, role: "member" }), 201);
      const todo = await columnOf(r, O, pid, "todo");
      const progress = await columnOf(r, O, pid, "in_progress");
      const done = await columnOf(r, O, pid, "done");
      const rule = {
        name: "Urgent to done",
        trigger: "task.moved",
        conditions: [{ field: "priority", value: "urgent" }],
        actions: [{ type: "move_to_column", column_id: done }],
      };
      await r.call("A member cannot manage rules → 403", tn({ ...A, accessToken: ML.at }, "POST", `/projects/${pid}/rules`, rule), 403);
      await r.call("An unknown action is refused → 400", tn(O, "POST", `/projects/${pid}/rules`, { ...rule, name: "Bad", actions: [{ type: "run_script" }] }), 400);
      const created = await r.call("Create the rule", tn(O, "POST", `/projects/${pid}/rules`, rule), 201);
      const rid = str(created, "data.id");
      await r.call("The same name twice → 409", tn(O, "POST", `/projects/${pid}/rules`, rule), 409);
      const urgent = str(await r.call("Create an urgent task", tn(O, "POST", `/projects/${pid}/tasks`, { title: "Fire", priority: "urgent", column_id: todo }), 201), "data.id");
      const low = str(await r.call("Create a low task", tn(O, "POST", `/projects/${pid}/tasks`, { title: "Calm", priority: "low", column_id: todo }), 201), "data.id");
      await r.call("Dry run: the actions that would run", tn(O, "POST", `/projects/${pid}/rules/${rid}/test`, { task_id: urgent }), 200, (e) =>
        getPath(e.resBody, "data.matches") === true ? null : "expected the urgent task to match",
      );
      await r.call("Move the urgent task by hand", tn(O, "POST", `/tasks/${urgent}/move`, { column_id: progress }), 200);
      await r.call("Move the low task by hand", tn(O, "POST", `/tasks/${low}/move`, { column_id: progress }), 200);
      let moved = false;
      for (let i = 0; i < 10 && !moved; i++) {
        await sleep(3000);
        const t = await r.call(`Wait for the engine (attempt ${i + 1})`, tn(O, "GET", `/tasks/${urgent}`), 200);
        moved = str(t, "data.column_id") === done;
      }
      r.need(moved, "the engine did not move the task; is the backend running with background workers?");
      await r.call("The low task stays where it is", tn(O, "GET", `/tasks/${low}`), 200, (e) => (str(e, "data.column_id") === progress ? null : "the low task was moved"));
      await r.call("Exactly one ok run (no cascade)", tn(O, "GET", `/projects/${pid}/rules/${rid}/runs`), 200, (e) =>
        list(e).length === 1 && list(e)[0].status === "ok" ? null : `expected one ok run, got ${JSON.stringify(list(e).map((x) => x.status))}`,
      );
      await r.call("Delete the rule", tn(O, "DELETE", `/projects/${pid}/rules/${rid}`), 200);
    },
  },
  {
    key: "pm.automation.recurrence",
    module: "projects",
    title: "Recurring task: schedule, validation and run now (phase 4)",
    description:
      "Schedules a weekly copy of a template task, refuses an unknown frequency and non-managers, runs it now and shows the copy is titled like the template, then lists and deletes the recurrence.",
    needs: "A",
    run: async (r, { A, uid }) => {
      const { s: O } = await projectOwner(r, A, uid);
      const member = await mkUser(r, A, uid, "pmar");
      const ML = await login(r, "Login member", A.slug, member.email, PASSWORD);
      const key = `R${uid}`.toUpperCase().replace(/[^A-Z0-9]/g, "").slice(0, 10);
      const pid = str(await r.call("Create a project", tn(O, "POST", "/projects", { key, name: `Recurrence ${uid}` }), 201), "data.id");
      await r.call("Add a member", tn(O, "POST", `/projects/${pid}/members`, { user_id: member.id, role: "member" }), 201);
      const tpl = str(await r.call("Create the template task", tn(O, "POST", `/projects/${pid}/tasks`, { title: "Weekly report" }), 201), "data.id");
      await r.call("Unknown frequency → 400", tn(O, "POST", `/projects/${pid}/recurrences`, { template_task_id: tpl, frequency: "yearly" }), 400);
      await r.call("A member cannot schedule → 403", tn({ ...A, accessToken: ML.at }, "POST", `/projects/${pid}/recurrences`, { template_task_id: tpl, frequency: "weekly" }), 403);
      const rec = await r.call("Schedule it weekly", tn(O, "POST", `/projects/${pid}/recurrences`, { template_task_id: tpl, frequency: "weekly" }), 201);
      const rid = str(rec, "data.id");
      const run = await r.call("Run now", tn(O, "POST", `/projects/${pid}/recurrences/${rid}/run-now`), 201);
      const copy = str(run, "data.task_id");
      await r.call("The copy is titled like the template", tn(O, "GET", `/tasks/${copy}`), 200, (e) => (str(e, "data.title") === "Weekly report" ? null : `title is ${str(e, "data.title")}`));
      await r.call("Pause the recurrence", tn(O, "PATCH", `/projects/${pid}/recurrences/${rid}`, { active: false }), 200);
      await r.call("List recurrences", tn(O, "GET", `/projects/${pid}/recurrences`), 200, (e) => (list(e).length === 1 ? null : "expected 1 recurrence"));
      await r.call("Delete the recurrence", tn(O, "DELETE", `/projects/${pid}/recurrences/${rid}`), 200);
    },
  },
  {
    key: "pm.automation.public",
    module: "projects",
    title: "Public project link: allow-list, revocation, identical 404 (phase 4)",
    description:
      "Creates a read-only link, reads it without any login, checks that no user id or person field appears, revokes it and shows that revoked and unknown tokens answer the same 404.",
    needs: "A",
    run: async (r, { A, uid }) => {
      const { s: O, id: ownerId } = await projectOwner(r, A, uid);
      const key = `P${uid}`.toUpperCase().replace(/[^A-Z0-9]/g, "").slice(0, 10);
      const pid = str(await r.call("Create a project", tn(O, "POST", "/projects", { key, name: `Public ${uid}` }), 201), "data.id");
      await r.call("Create a task", tn(O, "POST", `/projects/${pid}/tasks`, { title: "Visible to the public", assignee_ids: [ownerId] }), 201);
      const link = await r.call("Create a public link (the token is shown once)", tn(O, "POST", `/projects/${pid}/public-links`, {}), 201);
      const token = str(link, "data.token");
      r.need(token, "no token returned");
      const page = await r.call("Read it with no login", gl("GET", `/public/projects/${token}`, undefined, null), 200, (e) => {
        const raw = JSON.stringify(e.resBody);
        const leaked = [ownerId, pid, "assignee", "reporter", "user_id"].filter((x) => raw.includes(x));
        return leaked.length ? `the public page leaks ${leaked.join(", ")}` : null;
      });
      r.need(str(page, "data.project.key") === key, "the public page shows the project key");
      await r.call("The list never shows the secret", tn(O, "GET", `/projects/${pid}/public-links`), 200, (e) => (JSON.stringify(e.resBody).includes(token) ? "the token leaked into the list" : null));
      await r.call("Revoke the link", tn(O, "DELETE", `/projects/${pid}/public-links/${str(link, "data.id")}`), 200);
      const revoked = await r.call("A revoked token → 404", gl("GET", `/public/projects/${token}`, undefined, null), 404);
      await r.call("An unknown token → the same 404", gl("GET", `/public/projects/${"a".repeat(43)}`, undefined, null), 404, (e) =>
        JSON.stringify(e.resBody) === JSON.stringify(revoked.resBody) ? null : "revoked and unknown tokens answer differently",
      );
    },
  },
];
