import { getPath } from "./api";
import { str, type Runner, type Scenario } from "./runner";
import { gl, login, mkUser, PASSWORD, permId, tn } from "./scenarios0";
import type { Exchange, Session } from "./types";

// M2 planning and views scenarios (sprints, plan moves, board, My work, search, saved views, calendar token): the
// specification goldens PG6, PG7 (small scale), PG9 and PG12 scripted against the live backend. Tasks are created through
// POST /projects/:id/tasks. Each run creates its own users and project in session A's tenant (A must be a tenant_admin).

const list = (ex: Exchange, path = "data") => (getPath(ex.resBody, path) as Record<string, unknown>[] | undefined) ?? [];
const num = (ex: Exchange, path: string) => Number(getPath(ex.resBody, path));

/** A fresh user holding exactly the given tenant permissions, logged in. */
async function limited(r: Runner, A: Session, uid: string, tag: string, perms: [string, string][]): Promise<{ S: Session; userId: string }> {
  const ids: string[] = [];
  for (const [res, act] of perms) ids.push(await permId(r, A, res, act));
  const role = await r.call(`Role {${perms.map((p) => p.join(":")).join(", ") || "nothing"}}`, tn(A, "POST", "/roles", { name: `dt-${tag}-${uid}`, permission_ids: ids }), 201);
  const u = await mkUser(r, A, uid, tag, [str(role, "data.id")]);
  const L = await login(r, `Login ${tag}`, A.slug, u.email, PASSWORD);
  return { S: { ...A, accessToken: L.at }, userId: u.id };
}

export const S_PM_PLANNING: Scenario[] = [
  {
    key: "pm.planning.sprints",
    module: "projects",
    title: "Scrum sprints, plan moves, one active sprint, completion report (PG6, PG12)",
    description:
      "Creates a scrum project and three tasks, plans two of them into a sprint (a stale version conflicts per task), starts the sprint, shows that a second active sprint is refused with 409 SPRINT_ACTIVE (PG6), completes it moving unfinished work to the next sprint and reads the stored report. A kanban project answers 409 PROJECT_MODE.",
    needs: "A",
    run: async (r: Runner, { A, uid }) => {
      const key = `S${uid}`.toUpperCase().replace(/[^A-Z0-9]/g, "").slice(0, 10);
      const owner = await limited(r, A, uid, "spo", [["projects", "create"]]);
      const viewer = await limited(r, A, uid, "spv", []);
      const p = await r.call("Create a scrum project", tn(owner.S, "POST", "/projects", { key, name: `Sprints ${uid}`, mode: "scrum" }), 201);
      const pid = str(p, "data.id");
      await r.call("Add a viewer", tn(owner.S, "POST", `/projects/${pid}/members`, { user_id: viewer.userId, role: "viewer" }), 201);
      const kanban = await r.call("Create a kanban project", tn(owner.S, "POST", "/projects", { key: `K${key}`.slice(0, 10), name: `Kanban ${uid}` }), 201);
      await r.call("Sprints need scrum mode: 409 PROJECT_MODE", tn(owner.S, "POST", `/projects/${str(kanban, "data.id")}/sprints`, { name: "x" }), 409);

      const tasks: string[] = [];
      for (const title of ["Design", "Build", "Ship"]) {
        const t = await r.call(`Create task ${title}`, tn(owner.S, "POST", `/projects/${pid}/tasks`, { title, estimate_minutes: 30 }), 201);
        tasks.push(str(t, "data.id"));
      }
      const s1 = await r.call("Create sprint 1", tn(owner.S, "POST", `/projects/${pid}/sprints`, { name: "Sprint 1", start_on: "2030-03-04", end_on: "2030-03-15" }), 201);
      const s2 = await r.call("Create sprint 2", tn(owner.S, "POST", `/projects/${pid}/sprints`, { name: "Sprint 2", start_on: "2030-03-18", end_on: "2030-03-29" }), 201);
      const sid1 = str(s1, "data.id");
      const sid2 = str(s2, "data.id");
      await r.call("A viewer cannot manage sprints: 403", tn(viewer.S, "POST", `/projects/${pid}/sprints`, { name: "Nope" }), 403);
      await r.call("The backlog holds the three tasks", tn(viewer.S, "GET", `/projects/${pid}/backlog`), 200, (e) => (list(e).length === 3 ? null : "expected 3 backlog tasks"));
      await r.call("Plan two tasks into sprint 1", tn(owner.S, "POST", `/projects/${pid}/plan`, { task_ids: [tasks[0], tasks[1]], sprint_id: sid1 }), 200, (e) =>
        list(e).every((x) => x.status === "moved") ? null : "both tasks must move",
      );
      await r.call("A stale version conflicts for that task only", tn(owner.S, "POST", `/projects/${pid}/plan`, { task_ids: [tasks[0]], sprint_id: sid2, versions: { [tasks[0]]: 1 } }), 200, (e) =>
        list(e)[0]?.status === "conflict" ? null : "expected a per-task conflict",
      );
      await r.call("The backlog shrinks to one task", tn(owner.S, "GET", `/projects/${pid}/backlog`), 200, (e) => (list(e).length === 1 ? null : "expected 1 backlog task"));
      await r.call("Start sprint 1", tn(owner.S, "POST", `/sprints/${sid1}/start`), 200);
      await r.call("A second active sprint: 409 SPRINT_ACTIVE (PG6)", tn(owner.S, "POST", `/sprints/${sid2}/start`), 409, (e) =>
        str(e, "error.code") === "SPRINT_ACTIVE" ? null : "expected SPRINT_ACTIVE",
      );
      await r.call("Live report counts the commitment", tn(owner.S, "GET", `/sprints/${sid1}/report`), 200, (e) => (num(e, "data.report.committed_count") === 2 ? null : "committed_count should be 2"));
      await r.call("Complete sprint 1, moving unfinished tasks to sprint 2", tn(owner.S, "POST", `/sprints/${sid1}/complete`, { move_unfinished_to: sid2 }), 200, (e) =>
        num(e, "data.report.moved_unfinished_count") === 2 ? null : "both tasks were unfinished",
      );
      await r.call("The stored report is no longer live", tn(owner.S, "GET", `/sprints/${sid1}/report`), 200, (e) => (getPath(e.resBody, "data.live") === false ? null : "report must be stored"));
      await r.call("Sprint 2 now holds two tasks", tn(viewer.S, "GET", `/sprints/${sid2}`), 200, (e) => (num(e, "data.task_count") === 2 ? null : "task_count should be 2"));
    },
  },
  {
    key: "pm.planning.views",
    module: "projects",
    title: "Board, My work, search, saved views and calendar token (PG7 small scale, PG9, PG12)",
    description:
      "Shows the board with per-column counts and paging, My work grouped by due date with badge counts, tenant-scoped search that never finds a private project's tasks (PG9), saved-view validation (unknown filter refused, personal views stay private) and a calendar feed token that is shown once, works without a bearer token and stops working when revoked.",
    needs: "A",
    run: async (r: Runner, { A, uid }) => {
      const key = `V${uid}`.toUpperCase().replace(/[^A-Z0-9]/g, "").slice(0, 10);
      const owner = await limited(r, A, uid, "vwo", [["projects", "create"]]);
      const stranger = await limited(r, A, uid, "vws", []);
      const p = await r.call("Create a scrum project", tn(owner.S, "POST", "/projects", { key, name: `Views ${uid}`, mode: "scrum" }), 201);
      const pid = str(p, "data.id");
      const word = `zebra${uid}`.replace(/[^a-z0-9]/gi, "").toLowerCase();
      const due = new Date(Date.now() + 2 * 86400000).toISOString();
      await r.call("Create an assigned task with a due date", tn(owner.S, "POST", `/projects/${pid}/tasks`, { title: `Plan ${word}`, due_at: due, assignee_ids: [owner.userId] }), 201);
      await r.call("Create an unassigned task", tn(owner.S, "POST", `/projects/${pid}/tasks`, { title: "Unassigned chore" }), 201);

      await r.call("Board: columns with counts, one card per column page", tn(owner.S, "GET", `/projects/${pid}/board?per_column=1`), 200, (e) =>
        list(e, "data.columns").length === 4 ? null : "expected 4 columns",
      );
      await r.call("My work lists the assigned task", tn(owner.S, "GET", "/my-work"), 200, (e) => (num(e, "data.counts.total") === 1 ? null : "total should be 1"));
      await r.call("My-work counts for the badge", tn(owner.S, "GET", "/my-work/counts"), 200, (e) => (num(e, "data.total") === 1 ? null : "total should be 1"));
      await r.call("Search finds the task by word", tn(owner.S, "GET", `/search?q=${word}`), 200, (e) => (list(e).length === 1 ? null : "expected 1 hit"));
      await r.call("Search finds the task by ticket id", tn(owner.S, "GET", `/search?q=${key}-1`), 200, (e) => (list(e).length === 1 ? null : "expected the ticket hit"));
      await r.call("A stranger finds nothing in a private project (PG9)", tn(stranger.S, "GET", `/search?q=${word}`), 200, (e) => (list(e).length === 0 ? null : "private project leaked"));
      await r.call("An empty query: 400", tn(owner.S, "GET", "/search?q="), 400);
      const from = due.slice(0, 10);
      await r.call("Project calendar for the due day", tn(owner.S, "GET", `/projects/${pid}/calendar?from=${from}&to=${from}`), 200, (e) => (list(e, "data.tasks").length === 1 ? null : "expected 1 task"));
      await r.call("A window over 62 days: 400", tn(owner.S, "GET", `/projects/${pid}/calendar?from=2030-01-01&to=2030-06-01`), 400);
      await r.call("Gantt page", tn(owner.S, "GET", `/projects/${pid}/gantt`), 200);

      await r.call("Save a personal view", tn(owner.S, "POST", "/saved-views", { name: "My open work", layout: "list", project_id: pid, filters: { mine: true }, sort: [{ field: "due", dir: "asc" }] }), 201);
      await r.call("An unknown filter key is refused", tn(owner.S, "POST", "/saved-views", { name: "Evil", layout: "list", filters: { sql: "1=1" } }), 400);
      await r.call("The stranger cannot see the owner's view", tn(stranger.S, "GET", "/saved-views"), 200, (e) => (list(e).length === 0 ? null : "personal view leaked"));

      const tok = await r.call("Create a calendar token (shown once)", tn(owner.S, "POST", "/calendar-tokens", { label: "console" }), 201);
      const raw = str(tok, "data.token");
      await r.call("List tokens never shows the secret", tn(owner.S, "GET", "/calendar-tokens"), 200, (e) => (JSON.stringify(e.resBody).includes(raw) ? "secret leaked" : null));
      await r.call("The feed works with the token only (no bearer token)", gl("GET", `/calendar/feed.ics?token=${raw}`, undefined, null), 200);
      await r.call("Revoke the token", tn(owner.S, "DELETE", `/calendar-tokens/${str(tok, "data.id")}`), 200);
      await r.call("A revoked token: 404", gl("GET", `/calendar/feed.ics?token=${raw}`, undefined, null), 404);
    },
  },
];
