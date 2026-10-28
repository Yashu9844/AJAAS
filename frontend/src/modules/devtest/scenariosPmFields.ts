import { getPath } from "./api";
import { str, type Runner, type Scenario } from "./runner";
import { login, mkUser, PASSWORD, permId, tn } from "./scenarios0";
import type { Exchange, Session } from "./types";

// M2 custom-field scenarios (docs/TEST_CONSOLE.md): typed project fields and per-task values, scripted against the live
// backend. Each run creates its own users and project in session A's tenant (A must be a tenant_admin).

const list = (ex: Exchange, path = "data") => (getPath(ex.resBody, path) as Record<string, unknown>[] | undefined) ?? [];

async function projectOwner(r: Runner, A: Session, uid: string): Promise<Session> {
  const create = await permId(r, A, "projects", "create");
  const role = await r.call("Role {projects:create}", tn(A, "POST", "/roles", { name: `dt-pmf-${uid}`, permission_ids: [create] }), 201);
  const u = await mkUser(r, A, uid, "pmf", [str(role, "data.id")]);
  const L = await login(r, "Login owner", A.slug, u.email, PASSWORD);
  return { ...A, accessToken: L.at };
}

export const S_PM_FIELDS: Scenario[] = [
  {
    key: "pm.fields.flow",
    module: "projects",
    title: "Typed custom fields, task values, https-only URLs, versions (phase 4)",
    description:
      "Defines select, URL and user fields on a project, sets and clears task values, shows that bad options, http URLs and non-member users are refused, that outsiders see nothing, and that a stale task version is refused after values changed it.",
    needs: "A",
    run: async (r, { A, uid }) => {
      const O = await projectOwner(r, A, uid);
      const outsider = await mkUser(r, A, uid, "pmfx");
      const member = await mkUser(r, A, uid, "pmfm");
      const OL = await login(r, "Login outsider", A.slug, outsider.email, PASSWORD);
      const key = `F${uid}`.toUpperCase().replace(/[^A-Z0-9]/g, "").slice(0, 10);
      const p = await r.call("Create a project", tn(O, "POST", "/projects", { key, name: `Fields ${uid}` }), 201);
      const pid = str(p, "data.id");
      await r.call("Add a member", tn(O, "POST", `/projects/${pid}/members`, { user_id: member.id, role: "member" }), 201);
      const t = await r.call("Create a task", tn(O, "POST", `/projects/${pid}/tasks`, { title: "With fields" }), 201);
      const tid = str(t, "data.id");
      await r.call("A select field needs options → 400", tn(O, "POST", `/projects/${pid}/custom-fields`, { name: "Env", type: "select" }), 400);
      await r.call("Define Env (select)", tn(O, "POST", `/projects/${pid}/custom-fields`, { name: "Env", type: "select", options: ["dev", "prod"] }), 201);
      await r.call("Define Spec (url)", tn(O, "POST", `/projects/${pid}/custom-fields`, { name: "Spec", type: "url" }), 201);
      await r.call("Define Reviewer (user)", tn(O, "POST", `/projects/${pid}/custom-fields`, { name: "Reviewer", type: "user" }), 201);
      await r.call("The same name twice → 409", tn(O, "POST", `/projects/${pid}/custom-fields`, { name: "env", type: "text" }), 409);
      await r.call("An outsider sees nothing → 404", tn({ ...A, accessToken: OL.at }, "GET", `/projects/${pid}/custom-fields`), 404);
      await r.call("Set values", tn(O, "PUT", `/tasks/${tid}/custom-fields`, { values: { env: "prod", spec: "https://example.com/spec", reviewer: member.id } }), 200);
      await r.call("http URL → 400", tn(O, "PUT", `/tasks/${tid}/custom-fields`, { values: { spec: "http://example.com" } }), 400);
      await r.call("Not an option → 400", tn(O, "PUT", `/tasks/${tid}/custom-fields`, { values: { env: "staging" } }), 400);
      await r.call("A user who is not a member → 400", tn(O, "PUT", `/tasks/${tid}/custom-fields`, { values: { reviewer: outsider.id } }), 400);
      await r.call("All three fields are listed", tn(O, "GET", `/tasks/${tid}/custom-fields`), 200, (e) => (list(e).length === 3 ? null : "expected 3 fields"));
      await r.call("The task version is stale after the edit → 409", tn(O, "PUT", `/tasks/${tid}/custom-fields`, { values: { env: "dev" }, version: 1 }), 409);
      await r.call("Clear a value", tn(O, "PUT", `/tasks/${tid}/custom-fields`, { values: { env: null } }), 200);
    },
  },
];
