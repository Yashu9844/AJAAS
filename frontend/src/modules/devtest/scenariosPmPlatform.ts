import { getPath } from "./api";
import { str, type Scenario } from "./runner";
import { login, mkUser, PASSWORD, permId, tn } from "./scenarios0";
import type { Exchange } from "./types";

// M2 platform scenarios (docs/TEST_CONSOLE.md): group membership rules that follow HR data, and deactivation.
// Each run creates its own users, department and project in session A's tenant (A must be a tenant_admin).

const list = (ex: Exchange, path = "data") => (getPath(ex.resBody, path) as Record<string, unknown>[] | undefined) ?? [];

export const S_PM_PLATFORM: Scenario[] = [
  {
    key: "pm.platform.groups",
    module: "projects",
    title: "Group rules follow HR placement; deactivation leaves projects (PR-04, section 7)",
    description:
      "Places two people in a department under a manager, adds a department rule and a reports-of rule to a private project, shows that group members appear (and cannot be removed one by one), that a group member can read the project, and that deactivating a login takes the member out while removing the rules empties the project again.",
    needs: "A",
    run: async (r, { A, uid }) => {
      const create = await permId(r, A, "projects", "create");
      const role = await r.call("Role {projects:create}", tn(A, "POST", "/roles", { name: `dt-pmg-${uid}`, permission_ids: [create] }), 201);
      const owner = await mkUser(r, A, uid, "pmgo", [str(role, "data.id")]);
      const OL = await login(r, "Login owner", A.slug, owner.email, PASSWORD);
      const O = { ...A, accessToken: OL.at };
      const dev1 = await mkUser(r, A, uid, "pmg1");
      const dev2 = await mkUser(r, A, uid, "pmg2");
      const boss = await mkUser(r, A, uid, "pmgb");
      const dept = await r.call("Department", tn(A, "POST", "/hr/departments", { name: `Engineering ${uid}`, code: `E${uid}`.toUpperCase().slice(0, 10) }), 201);
      const did = str(dept, "data.id");
      for (const u of [dev1, dev2]) {
        await r.call("Place in the department under the manager", tn(A, "POST", "/hr/mappings", { user_id: u.id, department_id: did, manager_user_id: boss.id, is_primary: true }), 201);
      }
      const key = `G${uid}`.toUpperCase().replace(/[^A-Z0-9]/g, "").slice(0, 10);
      const p = await r.call("Create a private project", tn(O, "POST", "/projects", { key, name: `Groups ${uid}` }), 201);
      const pid = str(p, "data.id");
      await r.call("The owner role is not a rule role → 400", tn(O, "POST", `/projects/${pid}/group-rules`, { kind: "department", ref_id: did, role: "owner" }), 400);
      const rule = await r.call("Department rule (member)", tn(O, "POST", `/projects/${pid}/group-rules`, { kind: "department", ref_id: did, role: "member" }), 201);
      const rep = await r.call("Reports-of rule (viewer)", tn(O, "POST", `/projects/${pid}/group-rules`, { kind: "reports_of", ref_id: boss.id, role: "viewer" }), 201);
      await r.call("The same rule twice → 409", tn(O, "POST", `/projects/${pid}/group-rules`, { kind: "department", ref_id: did, role: "member" }), 409);
      await r.call("Members: owner plus both developers", tn(O, "GET", `/projects/${pid}/members`), 200, (e) => (list(e).length === 3 ? null : "expected 3 members"));
      await r.call("A group member cannot be removed alone → 409", tn(O, "DELETE", `/projects/${pid}/members/${dev1.id}`), 409);
      const DL = await login(r, "Login developer", A.slug, dev1.email, PASSWORD);
      await r.call("A group member reads the private project", tn({ ...A, accessToken: DL.at }, "GET", `/projects/${pid}`), 200);
      await r.call("Deactivate a developer login", tn(A, "POST", `/users/${dev2.id}/deactivate`), 200);
      await r.call("The deactivated developer left the project", tn(O, "GET", `/projects/${pid}/members`), 200, (e) => (list(e).length === 2 ? null : "expected 2 members"));
      await r.call("Delete the department rule", tn(O, "DELETE", `/projects/${pid}/group-rules/${str(rule, "data.id")}`), 200);
      await r.call("Delete the reports-of rule", tn(O, "DELETE", `/projects/${pid}/group-rules/${str(rep, "data.id")}`), 200);
      await r.call("Only the owner remains", tn(O, "GET", `/projects/${pid}/members`), 200, (e) => (list(e).length === 1 ? null : "expected 1 member"));
    },
  },
];
