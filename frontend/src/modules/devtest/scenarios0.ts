import { getPath } from "./api";
import { dataOf, expectFields, str, type Runner, type Scenario } from "./runner";
import type { CallSpec, Exchange, Method, Session } from "./types";

// ---- shared helpers (also used by scenarios1/2) ----
export const tn = (S: Session, method: Method, path: string, body?: unknown, query?: Record<string, string>, token: string | null = S.accessToken): CallSpec => ({
  method,
  scope: "tenant",
  slug: S.slug,
  token,
  path,
  body,
  query,
});
export const gl = (method: Method, path: string, body?: unknown, token?: string | null): CallSpec => ({ method, scope: "global", path, body, token });

export const listHas =
  (id: string, path = "data", idField = "id") =>
  (ex: Exchange): string | null => {
    const arr = getPath(ex.resBody, path);
    if (!Array.isArray(arr)) return `${path} is not an array`;
    return arr.some((x) => (x as Record<string, unknown>)[idField] === id) ? null : `id ${id} not found in list (${arr.length} items)`;
  };
export const listLacks =
  (id: string, path = "data", idField = "id") =>
  (ex: Exchange): string | null => {
    const arr = getPath(ex.resBody, path);
    if (!Array.isArray(arr)) return `${path} is not an array`;
    return arr.some((x) => (x as Record<string, unknown>)[idField] === id) ? `foreign id ${id} LEAKED into list` : null;
  };

export const ZERO = "00000000-0000-4000-8000-000000000000";
export const PASSWORD = "Passw0rd!123";

/** creates a throwaway user in session A's tenant; returns ids */
export async function mkUser(r: Runner, A: Session, uid: string, tag: string, roleIds: string[] = []): Promise<{ id: string; email: string }> {
  const email = `dt.${tag}.${uid}@example.com`;
  const ex = await r.call(
    `Create user ${tag}`,
    tn(A, "POST", "/users", { email, password: PASSWORD, first_name: "Dt", last_name: tag, role_ids: roleIds }),
    201,
    (e) => (str(e, "data.id") ? null : "no data.id in response"),
  );
  return { id: str(ex, "data.id"), email };
}

export async function roleId(r: Runner, A: Session, name: string): Promise<string> {
  const ex = await r.call(`Find system role "${name}"`, tn(A, "GET", "/roles", undefined, { per_page: "100" }), 200);
  const arr = (getPath(ex.resBody, "data") as { id: string; name: string }[] | undefined) ?? [];
  return arr.find((x) => x.name === name)?.id ?? "";
}

export async function permId(r: Runner, A: Session, resource: string, action: string): Promise<string> {
  const ex = await r.call(`Find permission ${resource}:${action}`, tn(A, "GET", "/permissions", undefined, { per_page: "100" }), 200);
  const arr = (getPath(ex.resBody, "data") as { id: string; resource: string; action: string }[] | undefined) ?? [];
  return arr.find((x) => x.resource === resource && x.action === action)?.id ?? "";
}

export async function login(r: Runner, name: string, slug: string, email: string, password: string, expect: number | number[] = 200): Promise<{ at: string; rt: string; ex: Exchange }> {
  const ex = await r.call(name, gl("POST", "/auth/login", { tenant_slug: slug, email, password }), expect);
  return { at: str(ex, "data.access_token"), rt: str(ex, "data.refresh_token"), ex };
}

// ============================================================ scenarios
export const S0: Scenario[] = [
  {
    key: "s0.errors",
    module: 0,
    title: "Auth & error handling negatives",
    description:
      "Wrong password, missing fields, unknown tenant, no/garbage token, nonexistent tenant host, nonexistent resource, invalid body, duplicate slug, invalid role/permission ids. Includes the spec rule NFR-SEC005 (GET /tenants must require auth) which the previous audit says is NOT implemented.",
    needs: "A",
    run: async (r, { A, uid }) => {
      r.need(A, "session A required");
      await r.call("Login with wrong password", gl("POST", "/auth/login", { tenant_slug: A.slug, email: A.user?.email, password: "definitely-wrong" }), 401);
      await r.call("Login with empty body (missing required fields)", gl("POST", "/auth/login", {}), 400);
      await r.call("Login with unknown tenant slug", gl("POST", "/auth/login", { tenant_slug: `nope${uid}`, email: "a@b.co", password: "x" }), [401, 404]);
      await r.call("GET /users without Authorization header", tn(A, "GET", "/users", undefined, undefined, null), 401);
      await r.call("GET /users with garbage bearer token", tn(A, "GET", "/users", undefined, undefined, "not.a.jwt"), 401);
      await r.call("GET /users on non-existent tenant host", { ...tn(A, "GET", "/users"), slug: `ghost${uid}` }, 404);
      await r.call("GET /users/{random uuid} (non-existent resource)", tn(A, "GET", `/users/${ZERO}`), 404);
      await r.call("GET /users/not-a-uuid", tn(A, "GET", "/users/not-a-uuid"), 400);
      await r.call("POST /users with invalid body (empty)", tn(A, "POST", "/users", {}), 400);
      await r.call("POST /users with malformed JSON", { ...tn(A, "POST", "/users"), rawBody: "{bad json" }, 400);
      await r.call("POST /tenants with empty body", gl("POST", "/tenants", {}), 400);
      await r.call("POST /tenants duplicate slug", gl("POST", "/tenants", { name: "dup", slug: A.slug }), 409);
      await r.call("Error responses use the standard envelope {error:{code,message}}", tn(A, "GET", "/users/not-a-uuid"), 400, (e) => (str(e, "error.code") === "VALIDATION_ERROR" ? null : `error is not the standard envelope: ${JSON.stringify((e.resBody as { error?: unknown })?.error)}`));
      await r.call("POST /tenants reserved slug 'admin'", gl("POST", "/tenants", { name: "x", slug: "admin" }), [400, 409]);
      await r.call("POST /roles with invalid permission id (not uuid)", tn(A, "POST", "/roles", { name: `bad${uid}`, permission_ids: ["abc"] }), 400);
      await r.call("POST /roles with unknown (valid uuid) permission id", tn(A, "POST", "/roles", { name: `bad2${uid}`, permission_ids: [ZERO] }), [400, 404, 422]);
      await r.call("Assign non-existent role to user", tn(A, "POST", `/users/${A.user?.id}/roles`, { role_ids: [ZERO] }), [400, 404, 422]);
      await r.call("NFR-SEC005: GET /tenants without platform key → 401", { ...gl("GET", "/tenants"), noPlatformKey: true }, 401);
      await r.call("NFR-SEC005: POST /tenants without platform key → 401/403", { ...gl("POST", "/tenants", { name: "anon", slug: `anon${uid}` }), noPlatformKey: true }, [401, 403]);
      await r.call("Tenant routes with a WRONG platform key → 403", { ...gl("GET", "/tenants"), noPlatformKey: true, headers: { "X-Platform-Key": "wrong" } }, 403);
    },
  },
  {
    key: "s0.tenant",
    module: 0,
    title: "Tenant lifecycle (create → read → update → read → suspend → read → activate → read)",
    description: "Creates a fresh tenant and verifies each state change with a follow-up GET. Also checks the tenant host resolver reacts to suspension.",
    needs: "none",
    run: async (r, { uid }) => {
      const slug = `dt${uid}`;
      const c = await r.call("Create tenant", gl("POST", "/tenants", { name: `DevTest ${uid}`, slug, plan: "free" }), 201, expectFields(["data.slug", slug], ["data.status", "active"]));
      const id = str(c, "data.id");
      r.need(id, "tenant id missing in create response");
      await r.call("GET tenant (read-back)", gl("GET", `/tenants/${id}`), 200, expectFields(["data.id", id], ["data.name", `DevTest ${uid}`]));
      await r.call("Update tenant name+plan", gl("PATCH", `/tenants/${id}`, { name: `Renamed ${uid}`, plan: "pro" }), 200);
      await r.call("GET after update", gl("GET", `/tenants/${id}`), 200, expectFields(["data.name", `Renamed ${uid}`], ["data.plan", "pro"]));
      await r.call("List tenants contains new tenant", gl("GET", "/tenants", undefined), 200, listHas(id));
      await r.call("Suspend tenant", gl("POST", `/tenants/${id}/suspend`), 200);
      await r.call("GET after suspend", gl("GET", `/tenants/${id}`), 200, expectFields(["data.status", "suspended"]));
      await r.call("Tenant host of suspended tenant is blocked", { method: "GET", scope: "tenant", slug, path: "/users", token: null }, 403);
      await r.call("Activate tenant", gl("POST", `/tenants/${id}/activate`), 200);
      await r.call("GET after activate", gl("GET", `/tenants/${id}`), 200, expectFields(["data.status", "active"]));
      await r.call("Tenant host resolves again (401 = reached auth, not blocked)", { method: "GET", scope: "tenant", slug, path: "/users", token: null }, 401);
      await r.call("Get non-existent tenant", gl("GET", `/tenants/${ZERO}`), 404);
      const slug2 = `dta${uid}`;
      const admin = { email: `root.${uid}@example.com`, password: PASSWORD, first_name: "Root", last_name: "Admin" };
      await r.call("Create tenant with a weak admin password → 400", gl("POST", "/tenants", { name: "Atomic", slug: slug2, admin: { ...admin, password: "short" } }), 400);
      const t2 = await r.call("Same slug now succeeds (the failed request left nothing behind) and provisions the admin", gl("POST", "/tenants", { name: "Atomic", slug: slug2, admin }), 201, (e) => (str(e, "data.admin_user_id") ? null : "admin_user_id missing"));
      if (str(t2, "data.id")) await login(r, "Provisioned admin can log in and is tenant_admin", slug2, admin.email, PASSWORD, 200);
    },
  },
  {
    key: "s0.user",
    module: 0,
    title: "User lifecycle (create → read → update → read → deactivate → read)",
    description: "Uses session A (tenant_admin). Verifies duplicate email, validation, role assignment shown in reads, and that a deactivated user can no longer log in.",
    needs: "A",
    run: async (r, { A, uid }) => {
      r.need(A, "session A required");
      const member = await roleId(r, A, "member");
      r.need(member, "system role `member` not found in tenant");
      const u = await mkUser(r, A, uid, "user", [member]);
      r.need(u.id, "user id missing");
      await r.call("GET user (read-back)", tn(A, "GET", `/users/${u.id}`), 200, (e) => {
        if (str(e, "data.email") !== u.email) return "email mismatch";
        const roles = (getPath(e.resBody, "data.roles") as { name: string }[]) ?? [];
        return roles.some((x) => x.name === "member") ? null : "role `member` missing from user.roles";
      });
      await r.call("Update user names/phone", tn(A, "PATCH", `/users/${u.id}`, { first_name: "Changed", phone: "+911234567890" }), 200);
      await r.call("GET after update", tn(A, "GET", `/users/${u.id}`), 200, expectFields(["data.first_name", "Changed"], ["data.phone", "+911234567890"]));
      await r.call("List users contains created user", tn(A, "GET", "/users", undefined, { per_page: "100" }), 200, listHas(u.id));
      await r.call("Duplicate email rejected", tn(A, "POST", "/users", { email: u.email, password: PASSWORD, first_name: "D", last_name: "D" }), 409);
      await r.call("Invalid email rejected", tn(A, "POST", "/users", { email: "not-an-email", password: PASSWORD, first_name: "D", last_name: "D" }), 400);
      await r.call("Short password rejected", tn(A, "POST", "/users", { email: `x.${uid}@example.com`, password: "short", first_name: "D", last_name: "D" }), 400);
      await login(r, "Created user can log in", A.slug, u.email, PASSWORD, 200);
      await r.call("Deactivate user", tn(A, "POST", `/users/${u.id}/deactivate`), 200);
      await r.call("GET after deactivate", tn(A, "GET", `/users/${u.id}`), 200, expectFields(["data.status", "inactive"]));
      await login(r, "Deactivated user cannot log in", A.slug, u.email, PASSWORD, [401, 403]);
    },
  },
  {
    key: "s0.role",
    module: 0,
    title: "Roles & permissions (create → read → update → assign permissions → delete)",
    description: "Requires permissions to exist in the (global) permissions table. The backend has no permission seed and no create-permission API, so run scripts/dev-test-bootstrap-admin.sh first.",
    needs: "A",
    run: async (r, { A, uid }) => {
      r.need(A, "session A required");
      const pRead = await permId(r, A, "users", "read");
      const pRoles = await permId(r, A, "roles", "read");
      r.need(pRead && pRoles, "permissions table has no users:read / roles:read (backend never seeds permissions — finding)");
      const c = await r.call("Create role with 1 permission", tn(A, "POST", "/roles", { name: `dt-role-${uid}`, description: "dev test", permission_ids: [pRead] }), 201, (e) =>
        (getPath(e.resBody, "data.permissions") as unknown[] | undefined)?.length === 1 ? null : "expected 1 permission in response",
      );
      const id = str(c, "data.id");
      r.need(id, "role id missing");
      await r.call("GET role (read-back)", tn(A, "GET", `/roles/${id}`), 200, expectFields(["data.name", `dt-role-${uid}`], ["data.is_system", false]));
      await r.call("Update role description", tn(A, "PATCH", `/roles/${id}`, { description: "updated" }), 200);
      await r.call("GET after update", tn(A, "GET", `/roles/${id}`), 200, expectFields(["data.description", "updated"]));
      await r.call("Assign additional permission to role", tn(A, "POST", `/roles/${id}/permissions`, { permission_ids: [pRoles] }), 200);
      await r.call("GET shows 2 permissions", tn(A, "GET", `/roles/${id}`), 200, (e) =>
        (getPath(e.resBody, "data.permissions") as unknown[] | undefined)?.length === 2 ? null : "expected 2 permissions",
      );
      await r.call("Duplicate role name rejected", tn(A, "POST", "/roles", { name: `dt-role-${uid}` }), 409);
      await r.call("List roles contains it", tn(A, "GET", "/roles", undefined, { per_page: "100" }), 200, listHas(id));
      const admin = await roleId(r, A, "tenant_admin");
      r.need(admin, "tenant_admin role missing");
      await r.call("System role cannot be deleted", tn(A, "DELETE", `/roles/${admin}`), [400, 403, 409]);
      await r.call("Delete custom role", tn(A, "DELETE", `/roles/${id}`), [200, 204]);
      await r.call("GET after delete → 404", tn(A, "GET", `/roles/${id}`), 404);
    },
  },
  {
    key: "s0.rbac",
    module: 0,
    title: "RBAC enforcement (limited user)",
    description: "Creates role {users:read}, a user with that role, logs in as that user and checks 200 vs 403 per permission, then grants roles:read and re-checks.",
    needs: "A",
    run: async (r, { A, uid }) => {
      r.need(A, "session A required");
      const pRead = await permId(r, A, "users", "read");
      const pRoles = await permId(r, A, "roles", "read");
      r.need(pRead && pRoles, "permissions not seeded (finding)");
      const role = await r.call("Create role {users:read}", tn(A, "POST", "/roles", { name: `dt-rbac-${uid}`, permission_ids: [pRead] }), 201);
      const rid = str(role, "data.id");
      r.need(rid, "role id missing");
      const u = await mkUser(r, A, uid, "rbac", [rid]);
      r.need(u.id, "user id missing");
      const L = await login(r, "Login as limited user", A.slug, u.email, PASSWORD, 200);
      r.need(L.at, "no access token");
      const S: Session = { ...A, accessToken: L.at };
      await r.call("limited user: GET /users (has users:read) → 200", tn(S, "GET", "/users"), 200);
      await r.call("limited user: POST /users (lacks users:create) → 403", tn(S, "POST", "/users", { email: `n.${uid}@example.com`, password: PASSWORD, first_name: "N", last_name: "N" }), 403);
      await r.call("limited user: GET /roles (lacks roles:read) → 403", tn(S, "GET", "/roles"), 403);
      await r.call("limited user: PATCH /users/:id (lacks users:update) → 403", tn(S, "PATCH", `/users/${u.id}`, { first_name: "X" }), 403);
      await r.call("limited user: GET /departments (lacks organization:read) → 403", tn(S, "GET", "/departments"), 403);
      await r.call("limited user: GET /employees (lacks employee:read) → 403", tn(S, "GET", "/employees"), 403);
      await r.call("admin grants roles:read to role", tn(A, "POST", `/roles/${rid}/permissions`, { permission_ids: [pRoles] }), 200);
      await r.call("limited user: GET /roles now allowed → 200", tn(S, "GET", "/roles"), 200);
      await r.call("tenant_admin bypass: GET /roles → 200", tn(A, "GET", "/roles"), 200);
      await r.call("cleanup: deactivate limited user", tn(A, "POST", `/users/${u.id}/deactivate`), 200);
    },
  },
  {
    key: "s0.access",
    module: 0,
    title: "Access management: /auth/me, activate, unassign role/permission, delete guards, audit trail, pagination",
    description: "Exercises the endpoints that make RBAC administrable end-to-end, the safety guards (role in use, self-deactivation), the audit-log read API, and pagination clamping.",
    needs: "A",
    run: async (r, { A, uid }) => {
      await r.call("/auth/me shows tenant_admin", { method: "GET", scope: "global", path: "/auth/me", token: A.accessToken }, 200, (e) =>
        ((getPath(e.resBody, "data.roles") as { name: string }[]) ?? []).some((x) => x.name === "tenant_admin") && getPath(e.resBody, "data.is_tenant_admin") === true ? null : "tenant_admin not reported",
      );
      const pRead = await permId(r, A, "users", "read");
      r.need(pRead, "users:read permission missing (seed)");
      const dupPerm = await r.call("Create role with the same permission id twice (de-duplicated)", tn(A, "POST", "/roles", { name: `dt-acc-${uid}`, permission_ids: [pRead, pRead] }), 201, (e) =>
        (getPath(e.resBody, "data.permissions") as unknown[] | undefined)?.length === 1 ? null : "expected exactly 1 permission",
      );
      const rid = str(dupPerm, "data.id");
      r.need(rid, "role id missing");
      await r.call("Unknown permission → 404 and NO partial role is left behind", tn(A, "POST", "/roles", { name: `dt-ghost-${uid}`, permission_ids: [ZERO] }), [400, 404]);
      await r.call("Ghost role was not created", tn(A, "GET", "/roles", undefined, { per_page: "100" }), 200, (e) =>
        ((getPath(e.resBody, "data") as { name: string }[]) ?? []).some((x) => x.name === `dt-ghost-${uid}`) ? "role created despite the failed request (no rollback)" : null,
      );
      const u = await mkUser(r, A, uid, "acc", [rid]);
      r.need(u.id, "user id missing");
      await r.call("User shows the assigned role", tn(A, "GET", `/users/${u.id}`), 200, (e) =>
        ((getPath(e.resBody, "data.roles") as { id: string }[]) ?? []).some((x) => x.id === rid) ? null : "assigned role missing in user.roles",
      );
      await r.call("Delete a role that is still assigned → 409", tn(A, "DELETE", `/roles/${rid}`), 409);
      await r.call("Unassign role from user", tn(A, "DELETE", `/users/${u.id}/roles/${rid}`), 200);
      await r.call("User no longer has the role", tn(A, "GET", `/users/${u.id}`), 200, (e) =>
        ((getPath(e.resBody, "data.roles") as { id: string }[]) ?? []).some((x) => x.id === rid) ? "role still listed" : null,
      );
      await r.call("Unassign again → 404", tn(A, "DELETE", `/users/${u.id}/roles/${rid}`), 404);
      await r.call("Remove permission from role", tn(A, "DELETE", `/roles/${rid}/permissions/${pRead}`), 200);
      await r.call("Role has no permissions now", tn(A, "GET", `/roles/${rid}`), 200, (e) => (((getPath(e.resBody, "data.permissions") as unknown[]) ?? []).length === 0 ? null : "permission still attached"));
      await r.call("Remove permission again → 404", tn(A, "DELETE", `/roles/${rid}/permissions/${pRead}`), 404);
      await r.call("Delete unassigned role", tn(A, "DELETE", `/roles/${rid}`), [200, 204]);
      await r.call("Self-deactivation is refused → 409", tn(A, "POST", `/users/${A.user?.id}/deactivate`), 409);
      await r.call("Deactivate user", tn(A, "POST", `/users/${u.id}/deactivate`), 200);
      await r.call("Activate user", tn(A, "POST", `/users/${u.id}/activate`), 200, expectFields(["data.status", "active"]));
      await r.call("GET after activation → active", tn(A, "GET", `/users/${u.id}`), 200, expectFields(["data.status", "active"]));
      await r.call("Activate again → 409", tn(A, "POST", `/users/${u.id}/activate`), 409);
      await r.call("Activate non-existent user → 404", tn(A, "POST", `/users/${ZERO}/activate`), 404);
      await r.call("Audit trail is readable and records user.created", tn(A, "GET", "/audit-logs", undefined, { per_page: "100" }), 200, (e) =>
        ((getPath(e.resBody, "data") as { action: string }[]) ?? []).some((x) => x.action === "user.created") ? null : "no user.created entry in the audit trail",
      );
      await r.call("per_page=0 falls back to the default (20)", tn(A, "GET", "/users", undefined, { per_page: "0" }), 200, expectFields(["meta.per_page", 20]));
      await r.call("per_page=1000 is clamped to 100", tn(A, "GET", "/users", undefined, { per_page: "1000" }), 200, expectFields(["meta.per_page", 100]));
      await r.call("page=-5 falls back to 1", tn(A, "GET", "/users", undefined, { page: "-5" }), 200, expectFields(["meta.page", 1]));
      await r.call("per_page=abc is tolerated", tn(A, "GET", "/users", undefined, { per_page: "abc" }), 200);
      await r.call("cleanup: deactivate user", tn(A, "POST", `/users/${u.id}/deactivate`), 200);
    },
  },
  {
    key: "s0.session",
    module: 0,
    title: "Session lifecycle: login → refresh rotation → refresh-reuse detection → logout → token revoked",
    description: "Uses a throwaway user so session A is not disturbed. Uses 2 of the 10-per-15-minute login attempts.",
    needs: "A",
    run: async (r, { A, uid }) => {
      r.need(A, "session A required");
      const member = await roleId(r, A, "member");
      const u = await mkUser(r, A, uid, "sess", member ? [member] : []);
      r.need(u.id, "user id missing");
      const L1 = await login(r, "Login #1", A.slug, u.email, PASSWORD, 200);
      r.need(L1.at && L1.rt, "login returned no tokens");
      const R1 = await r.call("Refresh with refresh token #1", gl("POST", "/auth/refresh", { refresh_token: L1.rt }), 200, (e) =>
        str(e, "data.access_token") && str(e, "data.refresh_token") && str(e, "data.refresh_token") !== L1.rt ? null : "no rotated tokens (or refresh token not rotated)",
      );
      await r.call("Re-use of consumed refresh token #1 → rejected", gl("POST", "/auth/refresh", { refresh_token: L1.rt }), 401);
      await r.call("Refresh with garbage token → 401", gl("POST", "/auth/refresh", { refresh_token: "garbage" }), 401);
      await r.call("Refresh with empty body → 400", gl("POST", "/auth/refresh", {}), 400);
      void R1;
      const L2 = await login(r, "Login #2", A.slug, u.email, PASSWORD, 200);
      r.need(L2.at, "login #2 returned no token");
      const S2: Session = { ...A, accessToken: L2.at };
      // member role has no permissions → 403 proves the token was ACCEPTED (not 401)
      await r.call("Access token accepted (403 = authenticated, but no permission)", tn(S2, "GET", "/users"), 403);
      await r.call("Logout (invalidate session)", gl("POST", "/auth/logout", undefined, L2.at), [200, 204]);
      await r.call("Access token after logout → 401", tn(S2, "GET", "/users"), 401);
      await r.call("Refresh token after logout → 401", gl("POST", "/auth/refresh", { refresh_token: L2.rt }), 401);
      await r.call("Logout without token → 401", gl("POST", "/auth/logout"), 401);
      await r.call("cleanup: deactivate user", tn(A, "POST", `/users/${u.id}/deactivate`), 200);
    },
  },
  {
    key: "s0.isolation",
    module: 0,
    title: "Tenant isolation (A ↔ B) — users, roles, departments",
    description:
      "Needs two logged-in sessions in two DIFFERENT tenants. Verifies A's token is refused on B's host (and vice-versa), A cannot read/modify B's records by id, and B's records do not leak into A's lists.",
    needs: "AB",
    run: async (r, { A, B, uid }) => {
      r.need(A && B, "sessions A and B required");
      r.need(A.slug !== B.slug, "sessions A and B must be different tenants");
      await r.call("Token A on tenant-B host → 403", { ...tn(A, "GET", "/users"), slug: B.slug }, 403);
      await r.call("Token B on tenant-A host → 403", { ...tn(B, "GET", "/users"), slug: A.slug }, 403);
      await r.call("Token A on tenant-B host (departments) → 403", { ...tn(A, "GET", "/departments"), slug: B.slug }, 403);
      await r.call("Token A on tenant-B host (employees) → 403", { ...tn(A, "GET", "/employees"), slug: B.slug }, 403);
      const dB = await r.call("B creates a department", tn(B, "POST", "/departments", { name: `B-dept-${uid}`, code: `BD${uid.toUpperCase()}` }), 201);
      const dId = str(dB, "data.id");
      r.need(dId, "B department id missing");
      await r.call("A GET B's department by id (own host) → 404", tn(A, "GET", `/departments/${dId}`), 404);
      await r.call("A PATCH B's department by id → 404", tn(A, "PATCH", `/departments/${dId}`, { name: "hijacked" }), 404);
      await r.call("A deactivate B's department by id → 404", tn(A, "POST", `/departments/${dId}/deactivate`), 404);
      await r.call("A's department list does not contain B's department", tn(A, "GET", "/departments", undefined, { per_page: "100" }), 200, listLacks(dId));
      await r.call("B still sees its department unchanged", tn(B, "GET", `/departments/${dId}`), 200, expectFields(["data.name", `B-dept-${uid}`]));
      const bUser = B.user?.id ?? "";
      r.need(bUser, "B user id missing");
      await r.call("A GET B's user by id → 404", tn(A, "GET", `/users/${bUser}`), 404);
      await r.call("A's user list does not contain B's user", tn(A, "GET", "/users", undefined, { per_page: "100" }), 200, listLacks(bUser));
      const bRole = B.user?.roles[0]?.id ?? "";
      if (bRole) await r.call("A GET B's role by id → 404", tn(A, "GET", `/roles/${bRole}`), 404);
      await r.call("A assigns B's role to A's user → rejected", tn(A, "POST", `/users/${A.user?.id}/roles`, { role_ids: [bRole || ZERO] }), [400, 404, 422]);
      await r.call("Cleanup: B deactivates its department", tn(B, "POST", `/departments/${dId}/deactivate`), 200);
    },
  },
];
export { dataOf };
