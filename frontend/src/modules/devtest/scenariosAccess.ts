import { getPath } from "./api";
import { str, type Scenario } from "./runner";
import { login, mkUser, PASSWORD, permId, tn } from "./scenarios0";
import type { Exchange, Session } from "./types";

// Access model scenarios (docs/ARCHITECTURE.md §17): predefined roles, scopes, inheritance, personal grants and
// blocks, automatic Employee/Manager roles. Each creates its own throwaway data in session A's tenant.

type RoleRow = { id: string; name: string; template_key?: string; parent_role_id?: string; is_automatic?: boolean };

const roles = (ex: Exchange): RoleRow[] => (getPath(ex.resBody, "data") as RoleRow[] | undefined) ?? [];
const byKey = (rows: RoleRow[], key: string) => rows.find((x) => x.template_key === key);
const has = (ex: Exchange, path: string, field: string, value: unknown) =>
  ((getPath(ex.resBody, path) as Record<string, unknown>[] | undefined) ?? []).some((x) => x[field] === value);

export const S0_ACCESS: Scenario[] = [
  {
    key: "s0.scopes",
    module: 0,
    title: "Predefined roles, scopes and inheritance",
    description:
      "Checks the 12 predefined roles and their chain (Executive → HR Admin → HR Manager → Manager → Employee), shows that an own-scope grant never passes a company-wide route, that a child role inherits its parent, that cycles are refused, and that predefined roles cannot be deleted.",
    needs: "A",
    run: async (r, { A, uid }) => {
      const list = await r.call("List roles", tn(A, "GET", "/roles", undefined, { per_page: "100" }), 200);
      const rows = roles(list);
      for (const k of ["employee", "manager", "hr_manager", "hr_admin", "executive", "payroll_admin", "payroll_approver", "finance", "recruiter", "interviewer", "it_admin", "auditor"]) {
        r.need(byKey(rows, k), `predefined role ${k} missing`);
      }
      const chain: [string, string][] = [["executive", "hr_admin"], ["hr_admin", "hr_manager"], ["hr_manager", "manager"], ["manager", "employee"]];
      for (const [child, parent] of chain) {
        r.need(byKey(rows, child)?.parent_role_id === byKey(rows, parent)?.id, `${child} must inherit ${parent}`);
      }
      await r.call("Delete predefined HR Admin → 403", tn(A, "DELETE", `/roles/${byKey(rows, "hr_admin")?.id}`), 403);

      const usersRead = await permId(r, A, "users", "read");
      const rolesRead = await permId(r, A, "roles", "read");
      const own = await r.call("Role with users:read at OWN scope", tn(A, "POST", "/roles", { name: `dt-own-${uid}`, grants: [{ permission_id: usersRead, scope: "own" }] }), 201);
      const u1 = await mkUser(r, A, uid, "own", [str(own, "data.id")]);
      const L1 = await login(r, "Login own-scope user", A.slug, u1.email, PASSWORD);
      const S1: Session = { ...A, accessToken: L1.at };
      await r.call("own scope never passes a company-wide route: GET /users → 403", tn(S1, "GET", "/users"), 403);

      const parent = await r.call("Parent role {users:read @ tenant}", tn(A, "POST", "/roles", { name: `dt-par-${uid}`, grants: [{ permission_id: usersRead, scope: "tenant" }] }), 201);
      const pid = str(parent, "data.id");
      const child = await r.call("Child role {roles:read} inheriting parent", tn(A, "POST", "/roles", { name: `dt-chi-${uid}`, permission_ids: [rolesRead], parent_role_id: pid }), 201);
      const cid = str(child, "data.id");
      const u2 = await mkUser(r, A, uid, "child", [cid]);
      const L2 = await login(r, "Login child-role user", A.slug, u2.email, PASSWORD);
      await r.call("inherited users:read: GET /users → 200", tn({ ...A, accessToken: L2.at }, "GET", "/users"), 200);
      await r.call("Explain shows the inherited source", tn(A, "GET", `/users/${u2.id}/access`), 200, (e) =>
        JSON.stringify(e.resBody).includes('"kind":"inherited"') ? null : "no inherited source in explanation",
      );
      await r.call("Parent inheriting its child is a cycle → 409", tn(A, "PATCH", `/roles/${pid}`, { parent_role_id: cid }), 409);
      await r.call("Clone the HR Manager role", tn(A, "POST", `/roles/${byKey(rows, "hr_manager")?.id}/clone`, { name: `dt-clone-${uid}` }), 201);
    },
  },
  {
    key: "s0.grants",
    module: 0,
    title: "Personal grants (with expiry) and blocks",
    description:
      "Gives a plain user users:read directly with an expiry, checks it applies immediately, revokes it, then shows a block beating a role, and that tenant_admin cannot be blocked.",
    needs: "A",
    run: async (r, { A, uid }) => {
      const usersRead = await permId(r, A, "users", "read");
      const u = await mkUser(r, A, uid, "plain");
      const L = await login(r, "Login plain user", A.slug, u.email, PASSWORD);
      const S: Session = { ...A, accessToken: L.at };
      await r.call("no access yet: GET /users → 403", tn(S, "GET", "/users"), 403);
      await r.call("Grant with a past expiry → 400", tn(A, "POST", `/users/${u.id}/grants`, { permission_id: usersRead, expires_at: "2020-01-01T00:00:00Z" }), 400);
      const tomorrow = new Date(Date.now() + 86400000).toISOString();
      await r.call("Grant users:read until tomorrow", tn(A, "POST", `/users/${u.id}/grants`, { permission_id: usersRead, expires_at: tomorrow, reason: "cover" }), 201);
      await r.call("granted: GET /users → 200 (no re-login needed)", tn(S, "GET", "/users"), 200);
      await r.call("Revoke the grant", tn(A, "DELETE", `/users/${u.id}/grants/${usersRead}`), 200);
      await r.call("revoked: GET /users → 403", tn(S, "GET", "/users"), 403);

      const role = await r.call("Role {users:read}", tn(A, "POST", "/roles", { name: `dt-rd-${uid}`, permission_ids: [usersRead] }), 201);
      await r.call("Assign the role", tn(A, "POST", `/users/${u.id}/roles`, { role_ids: [str(role, "data.id")] }), 200);
      await r.call("role access: GET /users → 200", tn(S, "GET", "/users"), 200);
      await r.call("Block users:read", tn(A, "POST", `/users/${u.id}/blocks`, { permission_id: usersRead, reason: "audit" }), 201);
      await r.call("blocked: GET /users → 403 although the role grants it", tn(S, "GET", "/users"), 403);
      await r.call("Explain shows blocked", tn(A, "GET", `/users/${u.id}/access`), 200, (e) => (has(e, "data.permissions", "blocked", true) ? null : "block not shown"));
      await r.call("Unblock", tn(A, "DELETE", `/users/${u.id}/blocks/${usersRead}`), 200);
      await r.call("unblocked: GET /users → 200", tn(S, "GET", "/users"), 200);
      const me = await r.call("Who am I (admin)", { method: "GET", scope: "global", path: "/auth/me", token: A.accessToken }, 200);
      const adminId = str(me, "data.user_id");
      await r.call("Block tenant_admin → 409", tn(A, "POST", `/users/${adminId}/blocks`, { permission_id: usersRead }), 409);
    },
  },
  {
    key: "s0.auto",
    module: 0,
    title: "Automatic Employee and Manager roles",
    description:
      "Creates two users with employee profiles and a reporting line; logging in as the boss assigns Employee and Manager automatically; they cannot be assigned or removed by hand; ending the reporting line and syncing removes Manager.",
    needs: "A",
    run: async (r, { A, uid }) => {
      const boss = await mkUser(r, A, uid, "boss");
      const rep = await mkUser(r, A, uid, "rep");
      const joining = "2026-01-15T00:00:00Z";
      for (const [u, code] of [[boss, `B${uid}`], [rep, `R${uid}`]] as const) {
        await r.call(`Employee profile ${code}`, tn(A, "POST", "/employees", { user_id: u.id, employee_code: code.toUpperCase(), first_name: "Dt", last_name: code, employment_type: "full_time", joining_date: joining }), 201);
      }
      const dept = await r.call("Department", tn(A, "POST", "/departments", { name: `Dt dept ${uid}` }), 201);
      const map = await r.call("Reporting line rep → boss", tn(A, "POST", "/mappings", { user_id: rep.id, department_id: str(dept, "data.id"), is_primary: true, manager_user_id: boss.id }), 201);
      const L = await login(r, "Login as boss (syncs automatic roles)", A.slug, boss.email, PASSWORD);
      await r.call("boss holds Employee + Manager (automatic)", { method: "GET", scope: "global", path: "/auth/me", token: L.at }, 200, (e) =>
        has(e, "data.roles", "name", "Manager") && has(e, "data.roles", "name", "Employee") && has(e, "data.roles", "source", "automatic") ? null : "automatic roles missing",
      );
      const list = await r.call("List roles", tn(A, "GET", "/roles", undefined, { per_page: "100" }), 200);
      const mgr = byKey(roles(list), "manager")?.id;
      await r.call("Assign Manager by hand → 409", tn(A, "POST", `/users/${rep.id}/roles`, { role_ids: [mgr] }), 409);
      await r.call("Remove Manager by hand → 409", tn(A, "DELETE", `/users/${boss.id}/roles/${mgr}`), 409);
      await r.call("End the reporting line", tn(A, "POST", `/mappings/${str(map, "data.id")}/deactivate`), 200);
      await r.call("Sync automatic roles", tn(A, "POST", "/access/automatic-roles/sync"), 200);
      await r.call("boss is Employee only now", tn(A, "GET", `/users/${boss.id}/access`), 200, (e) => {
        const raw = JSON.stringify(e.resBody);
        return !raw.includes('"name":"Manager"') && raw.includes('"name":"Employee"') ? null : "manager role still present";
      });
    },
  },
];
