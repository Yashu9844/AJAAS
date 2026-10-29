import { getPath } from "./api";
import { str, type Runner, type Scenario } from "./runner";
import { login, mkUser, PASSWORD, permId, tn } from "./scenarios0";
import type { Exchange, Session } from "./types";

// M2 AI-hook scenarios (docs/TEST_CONSOLE.md): the read-only seams a separate AI module uses - tool catalogue filtered by
// the caller's permissions, context packs, the document feed - and the agent label. Each run creates its own users and project.

const list = (ex: Exchange, path = "data") => (getPath(ex.resBody, path) as Record<string, unknown>[] | undefined) ?? [];
const names = (ex: Exchange) => list(ex).map((t) => String(t.name));

async function owner(r: Runner, A: Session, uid: string): Promise<Session> {
  const create = await permId(r, A, "projects", "create");
  const role = await r.call("Role {projects:create}", tn(A, "POST", "/roles", { name: `dt-ai-${uid}`, permission_ids: [create] }), 201);
  const u = await mkUser(r, A, uid, "aio", [str(role, "data.id")]);
  const L = await login(r, "Login owner", A.slug, u.email, PASSWORD);
  return { ...A, accessToken: L.at };
}

export const S_PM_AIHOOKS: Scenario[] = [
  {
    key: "pm.aihooks.flow",
    module: "projects",
    title: "Tool catalogue by permission, context packs, document feed, agent label (AI readiness)",
    description:
      "A viewer is offered read tools only while the owner also gets write and destructive ones; a stranger sees nothing of a private project; project and task context packs come back compact with a token estimate; the document feed pages with a cursor; an AI label on a request is accepted but never widens permissions.",
    needs: "A",
    run: async (r, { A, uid }) => {
      const O = await owner(r, A, uid);
      const viewer = await mkUser(r, A, uid, "aiv");
      const VL = await login(r, "Login viewer", A.slug, viewer.email, PASSWORD);
      const V: Session = { ...A, accessToken: VL.at };
      const stranger = await mkUser(r, A, uid, "ais");
      const SL = await login(r, "Login stranger", A.slug, stranger.email, PASSWORD);
      const S: Session = { ...A, accessToken: SL.at };
      const key = `A${uid}`.toUpperCase().replace(/[^A-Z0-9]/g, "").slice(0, 10);
      const p = await r.call("Create a private project", tn(O, "POST", "/projects", { key, name: `AI ready ${uid}` }), 201);
      const pid = str(p, "data.id");
      await r.call("Add the viewer", tn(O, "POST", `/projects/${pid}/members`, { user_id: viewer.id, role: "viewer" }), 201);
      const t = await r.call("Create a task", tn(O, "POST", `/projects/${pid}/tasks`, { title: "First", description: "Context for a model" }), 201);
      await r.call("Second task", tn(O, "POST", `/projects/${pid}/tasks`, { title: "Second" }), 201);
      await r.call("Owner tools include write and destructive ones", tn(O, "GET", "/ai-hooks/tools", undefined, { project_id: pid }), 200, (e) =>
        names(e).includes("create_task") && names(e).includes("delete_task") ? null : "owner should see create_task and delete_task",
      );
      await r.call("The viewer is offered reads only", tn(V, "GET", "/ai-hooks/tools", undefined, { project_id: pid }), 200, (e) =>
        names(e).includes("get_task") && !names(e).includes("create_task") && !names(e).includes("delete_task") ? null : "viewer must not see write tools",
      );
      await r.call("A stranger sees no tools of a private project → 404", tn(S, "GET", "/ai-hooks/tools", undefined, { project_id: pid }), 404);
      await r.call("Project context pack", tn(V, "GET", `/ai-hooks/context/projects/${pid}`), 200, (e) =>
        str(e, "data.kind") === "project" && Number(getPath(e.resBody, "data.approx_tokens")) > 0 ? null : "pack needs kind and approx_tokens",
      );
      await r.call("Task context pack by ticket key", tn(V, "GET", `/ai-hooks/context/tasks/${str(t, "data.key")}`), 200);
      await r.call("No pack for a stranger → 404", tn(S, "GET", `/ai-hooks/context/tasks/${str(t, "data.key")}`), 404);
      const page = await r.call("Document feed, first page of 1", tn(O, "GET", "/ai-hooks/documents", undefined, { project_id: pid, limit: "1" }), 200, (e) =>
        list(e).length === 1 && getPath(e.resBody, "meta.next_cursor") ? null : "expected 1 document and a cursor",
      );
      await r.call("Feed continues after the cursor", tn(O, "GET", "/ai-hooks/documents", undefined, { project_id: pid, cursor: String(getPath(page.resBody, "meta.next_cursor")) }), 200, (e) =>
        list(e).length === 1 ? null : "expected the second document",
      );
      await r.call("project_id is required → 400", tn(O, "GET", "/ai-hooks/documents"), 400);
      await r.call("Readable projects for the viewer", tn(V, "GET", "/ai-hooks/readable-projects"), 200, (e) => (list(e).length >= 1 ? null : "expected the project"));
      await r.call("An agent label never widens permissions → 403", { ...tn(V, "POST", `/projects/${pid}/tasks`, { title: "x" }), headers: { "X-JAAS-Agent": "console-test" } }, 403);
    },
  },
];
