import { getPath } from "./api";
import { str, type Runner, type Scenario } from "./runner";
import { login, mkUser, PASSWORD, permId, tn } from "./scenarios0";
import type { Exchange, Session } from "./types";

// M2 Projects & Tasks scenarios (docs/TEST_CONSOLE.md): the specification goldens scripted against the live backend.
// Each run creates its own users and projects in session A's tenant (A must be a tenant_admin).

const list = (ex: Exchange, path = "data") => (getPath(ex.resBody, path) as Record<string, unknown>[] | undefined) ?? [];

/** A fresh user holding exactly the given tenant permissions, logged in. */
async function limited(r: Runner, A: Session, uid: string, tag: string, perms: [string, string][]): Promise<{ S: Session; userId: string }> {
  const ids: string[] = [];
  for (const [res, act] of perms) ids.push(await permId(r, A, res, act));
  const role = await r.call(`Role {${perms.map((p) => p.join(":")).join(", ") || "nothing"}}`, tn(A, "POST", "/roles", { name: `dt-${tag}-${uid}`, permission_ids: ids }), 201);
  const u = await mkUser(r, A, uid, tag, [str(role, "data.id")]);
  const L = await login(r, `Login ${tag}`, A.slug, u.email, PASSWORD);
  return { S: { ...A, accessToken: L.at }, userId: u.id };
}

export const S_PROJECTS: Scenario[] = [
  {
    key: "pm.projects.setup",
    module: "projects",
    title: "Project, members, visibility, versions, setup data, archive (PG1, PG4)",
    description:
      "Creates a project as a user holding projects:create and shows its default columns; a private project is invisible to strangers and to other tenants (PG4); a viewer reads but cannot edit; an edit with a stale version is refused with the current version; columns, labels, milestones and a custom role are managed; archiving freezes the project and only a tenant administrator may delete it.",
    needs: "A",
    run: async (r, { A, uid }) => {
      const key = `P${uid}`.toUpperCase().replace(/[^A-Z0-9]/g, "").slice(0, 10);
      const owner = await limited(r, A, uid, "pmo", [["projects", "create"]]);
      const stranger = await limited(r, A, uid, "pms", []);
      await r.call("Creating needs projects:create → 403", tn(stranger.S, "POST", "/projects", { key: "NOPE", name: "x" }), 403);
      const p = await r.call("Create a project (owner)", tn(owner.S, "POST", "/projects", { key, name: `Project ${uid}`, description: "Console scenario" }), 201, (e) =>
        str(e, "data.my_role") === "owner" && str(e, "data.key") === key ? null : "creator must be the owner",
      );
      const pid = str(p, "data.id");
      await r.call("Duplicate key → 409", tn(owner.S, "POST", "/projects", { key, name: "Again" }), 409);
      await r.call("Default columns exist", tn(owner.S, "GET", `/projects/${pid}/columns`), 200, (e) => (list(e).length === 4 ? null : "expected 4 default columns"));
      await r.call("A stranger cannot see a private project → 404 (PG4)", tn(stranger.S, "GET", `/projects/${pid}`), 404);
      await r.call("The stranger's list is empty", tn(stranger.S, "GET", "/projects"), 200, (e) => (list(e).length === 0 ? null : "list must be empty"));
      await r.call("Add the stranger as a viewer", tn(owner.S, "POST", `/projects/${pid}/members`, { user_id: stranger.userId, role: "viewer" }), 201);
      await r.call("The viewer reads", tn(stranger.S, "GET", `/projects/${pid}`), 200);
      await r.call("The viewer cannot edit → 403", tn(stranger.S, "PATCH", `/projects/${pid}`, { name: "Hacked" }), 403);
      await r.call("Already a member → 409", tn(owner.S, "POST", `/projects/${pid}/members`, { user_id: stranger.userId, role: "member" }), 409);
      await r.call("Edit with the current version", tn(owner.S, "PATCH", `/projects/${pid}`, { name: `Renamed ${uid}`, version: 1 }), 200, (e) => (Number(getPath(e.resBody, "data.version")) === 2 ? null : "version should be 2"));
      await r.call("Stale version → 409 VERSION_CONFLICT", tn(owner.S, "PATCH", `/projects/${pid}`, { name: "Late", version: 1 }), 409, (e) =>
        Number(getPath(e.resBody, "error.current_version")) === 2 ? null : "error must carry current_version 2",
      );
      await r.call("Add a column", tn(owner.S, "POST", `/projects/${pid}/columns`, { name: "In review", category: "in_progress", wip_limit: 3 }), 201);
      await r.call("A project label", tn(owner.S, "POST", `/projects/${pid}/labels`, { name: "bug", color: "#FF0000" }), 201);
      await r.call("A milestone", tn(owner.S, "POST", `/projects/${pid}/milestones`, { name: "Beta", due_on: "2030-01-31" }), 201);
      await r.call("A custom role needs project_roles:manage → 403", tn(owner.S, "POST", "/project-roles", { name: `R${uid}`, permissions: ["project.read", "task.read"] }), 403);
      await r.call("Tenant admin defines a custom role", tn(A, "POST", "/project-roles", { name: `Reviewer ${uid}`, permissions: ["project.read", "task.read", "task.comment"] }), 201);
      await r.call("Archive", tn(owner.S, "POST", `/projects/${pid}/archive`), 200);
      await r.call("An archived project is frozen → 409", tn(owner.S, "POST", `/projects/${pid}/columns`, { name: "Late" }), 409);
      await r.call("The owner cannot delete → 403", tn(owner.S, "DELETE", `/projects/${pid}`), 403);
      await r.call("Tenant admin deletes the archived project", tn(A, "DELETE", `/projects/${pid}`), 200);
      await r.call("It is gone → 404", tn(owner.S, "GET", `/projects/${pid}`), 404);
    },
  },
];
