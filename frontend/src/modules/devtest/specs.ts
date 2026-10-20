import type { ChecklistItem, Field, Method, OpSpec } from "./types";

// Every spec below was derived from the real backend: routes.go, controllers and DTO binding tags.
// Nothing here is guessed; where the backend returns an unexpected shape the console reports it.

const pg: Field[] = [
  { name: "page", where: "query", type: "number", placeholder: "1" },
  { name: "per_page", where: "query", type: "number", placeholder: "20" },
];
const idf = (name: string, ref: string, label?: string): Field => ({ name, where: "path", ref, required: true, label: label ?? name });
const t = (k: string, label: string, method: Method, path: string, fields: Field[], expect: number[], extra: Partial<OpSpec> = {}): OpSpec => ({
  key: k,
  label,
  method,
  path,
  scope: "tenant",
  auth: true,
  fields,
  expect,
  ...extra,
});
const g = (k: string, label: string, method: Method, path: string, fields: Field[], expect: number[], extra: Partial<OpSpec> = {}): OpSpec => ({
  key: k,
  label,
  method,
  path,
  scope: "global",
  auth: false,
  fields,
  expect,
  ...extra,
});

// ============================ MODULE 0 ============================
export const M0_AUTH: OpSpec[] = [
  g(
    "m0.auth.forgot",
    "Forgot password",
    "POST",
    "/auth/forgot-password",
    [
      { name: "tenant_slug", where: "body", required: true },
      { name: "email", where: "body", required: true },
    ],
    [200, 202],
    { note: "Rate limited 5/hour. Backend should answer generically (no user enumeration)." },
  ),
  g(
    "m0.auth.reset",
    "Reset password",
    "POST",
    "/auth/reset-password",
    [
      { name: "token", where: "body", required: true, placeholder: "token from reset email" },
      { name: "new_password", where: "body", type: "password", required: true },
      { name: "confirm_password", where: "body", type: "password", required: true },
    ],
    [200],
    { note: "No endpoint returns the reset token (email not wired), so a success path cannot be driven from the UI. Use it to test failure with a bad token." },
  ),
];

export const M0_TENANTS: OpSpec[] = [
  g(
    "m0.tenant.create",
    "Create tenant",
    "POST",
    "/tenants",
    [
      { name: "name", where: "body", required: true },
      { name: "slug", where: "body", required: true, placeholder: "acme (3-64, [a-z0-9-])" },
      { name: "domain", where: "body", placeholder: "acme.example.com" },
      { name: "plan", where: "body", placeholder: "free" },
      {
        name: "admin",
        where: "body",
        type: "json",
        placeholder: '{"email":"admin@acme.com","password":"Passw0rd!123","first_name":"A","last_name":"B"}',
        label: "admin (optional first tenant_admin)",
      },
    ],
    [201],
    { capture: [{ idKey: "tenantId", from: "data.id" }], fetchAfter: "m0.tenant.get", note: "Needs the platform key (sidebar). Optional `admin` provisions the first tenant_admin atomically." },
  ),
  g("m0.tenant.list", "List tenants", "GET", "/tenants", pg, [200]),
  g("m0.tenant.get", "Get tenant", "GET", "/tenants/:id", [idf("id", "tenantId")], [200]),
  g(
    "m0.tenant.update",
    "Update tenant",
    "PATCH",
    "/tenants/:id",
    [
      idf("id", "tenantId"),
      { name: "name", where: "body" },
      { name: "domain", where: "body" },
      { name: "plan", where: "body" },
    ],
    [200],
  ),
  g("m0.tenant.suspend", "Suspend tenant", "POST", "/tenants/:id/suspend", [idf("id", "tenantId")], [200]),
  g("m0.tenant.activate", "Activate tenant", "POST", "/tenants/:id/activate", [idf("id", "tenantId")], [200]),
];

export const M0_USERS: OpSpec[] = [
  t(
    "m0.user.create",
    "Create user",
    "POST",
    "/users",
    [
      { name: "email", where: "body", required: true },
      { name: "password", where: "body", type: "password", required: true, def: "Passw0rd!123" },
      { name: "first_name", where: "body", required: true },
      { name: "last_name", where: "body", required: true },
      { name: "phone", where: "body" },
      { name: "role_ids", where: "body", type: "list", placeholder: "comma separated role UUIDs" },
    ],
    [201],
    { capture: [{ idKey: "userId", from: "data.id" }], fetchAfter: "m0.user.get" },
  ),
  t("m0.user.list", "List users", "GET", "/users", pg, [200]),
  t("m0.user.get", "Get user", "GET", "/users/:id", [idf("id", "userId")], [200]),
  t(
    "m0.user.update",
    "Update user",
    "PATCH",
    "/users/:id",
    [
      idf("id", "userId"),
      { name: "first_name", where: "body" },
      { name: "last_name", where: "body" },
      { name: "phone", where: "body" },
      { name: "avatar_url", where: "body", placeholder: "https://…" },
    ],
    [200],
  ),
  t("m0.user.deactivate", "Deactivate user", "POST", "/users/:id/deactivate", [idf("id", "userId")], [200]),
  t(
    "m0.user.assignRoles",
    "Assign roles to user",
    "POST",
    "/users/:id/roles",
    [idf("id", "userId"), { name: "role_ids", where: "body", type: "list", ref: "roleId", required: true }],
    [200],
  ),
];

export const M0_ROLES: OpSpec[] = [
  t(
    "m0.role.create",
    "Create role",
    "POST",
    "/roles",
    [
      { name: "name", where: "body", required: true },
      { name: "description", where: "body" },
      { name: "permission_ids", where: "body", type: "list", placeholder: "comma separated permission UUIDs (tenant scope)" },
      { name: "grants", where: "body", type: "json", placeholder: '[{"permission_id":"<uuid>","scope":"direct_reports"}]', label: "grants (scoped)" },
      { name: "parent_role_id", where: "body", placeholder: "optional: inherit everything this role grants" },
    ],
    [201],
    { capture: [{ idKey: "roleId", from: "data.id" }], fetchAfter: "m0.role.get", note: "Scopes: own, direct_reports, all_reports, department, tenant. You cannot grant more than you hold." },
  ),
  t("m0.role.list", "List roles", "GET", "/roles", pg, [200]),
  t("m0.role.get", "Get role", "GET", "/roles/:id", [idf("id", "roleId")], [200]),
  t(
    "m0.role.update",
    "Update role",
    "PATCH",
    "/roles/:id",
    [idf("id", "roleId"), { name: "name", where: "body" }, { name: "description", where: "body" }, { name: "parent_role_id", where: "body", placeholder: "role UUID, or empty string to remove the parent" }],
    [200],
  ),
  t("m0.role.clone", "Clone role", "POST", "/roles/:id/clone", [idf("id", "roleId"), { name: "name", where: "body", required: true }], [201], {
    capture: [{ idKey: "roleId", from: "data.id" }],
    note: "Copies grants, scopes and parent into a new custom role (predefined roles stay untouched).",
  }),
  t("m0.role.delete", "Delete role", "DELETE", "/roles/:id", [idf("id", "roleId")], [200, 204]),
  t(
    "m0.role.assignPerms",
    "Assign permissions to role",
    "POST",
    "/roles/:id/permissions",
    [
      idf("id", "roleId"),
      { name: "permission_ids", where: "body", type: "list", ref: "permissionId" },
      { name: "grants", where: "body", type: "json", placeholder: '[{"permission_id":"<uuid>","scope":"own"}]', label: "grants (scoped; re-granting changes the scope)" },
    ],
    [200],
  ),
  t("m0.perm.list", "List permissions", "GET", "/permissions", pg, [200], {
    capture: [{ idKey: "permissionId", from: "data.0.id" }],
    note: "Captures the first permission id as `permissionId`.",
  }),
];

export const M0_ACCESS: OpSpec[] = [
  t("m0.access.me", "Explain my access", "GET", "/auth/me/access", [], [200], { note: "Every permission with its scope and where it comes from (role, inherited, automatic role, personal grant) and blocks." }),
  t("m0.access.explain", "Explain a user's access", "GET", "/users/:id/access", [idf("id", "userId")], [200], { note: "Needs access:read." }),
  t("m0.access.grants", "List user grants", "GET", "/users/:id/grants", [idf("id", "userId")], [200]),
  t(
    "m0.access.grant",
    "Grant a permission to a user",
    "POST",
    "/users/:id/grants",
    [
      idf("id", "userId"),
      { name: "permission_id", where: "body", ref: "permissionId", required: true },
      { name: "scope", where: "body", type: "select", options: ["tenant", "department", "all_reports", "direct_reports", "own"] },
      { name: "expires_at", where: "body", type: "datetime", placeholder: "optional, must be in the future" },
      { name: "reason", where: "body" },
    ],
    [201],
    { note: "Needs access:manage. Temporary cover / delegation: set expires_at." },
  ),
  t("m0.access.revoke", "Revoke a user grant", "DELETE", "/users/:id/grants/:permission_id", [idf("id", "userId"), idf("permission_id", "permissionId")], [200]),
  t("m0.access.blocks", "List user blocks", "GET", "/users/:id/blocks", [idf("id", "userId")], [200]),
  t(
    "m0.access.block",
    "Block a permission for a user",
    "POST",
    "/users/:id/blocks",
    [idf("id", "userId"), { name: "permission_id", where: "body", ref: "permissionId", required: true }, { name: "reason", where: "body" }],
    [201],
    { note: "A block always wins over roles and grants. tenant_admin holders cannot be blocked (409)." },
  ),
  t("m0.access.unblock", "Remove a block", "DELETE", "/users/:id/blocks/:permission_id", [idf("id", "userId"), idf("permission_id", "permissionId")], [200]),
  t("m0.access.sync", "Sync automatic roles now", "POST", "/access/automatic-roles/sync", [], [200], {
    note: "Employee = has an active employee record; Manager = has active direct reports (HR mappings). Also runs at login and every 5 minutes.",
  }),
];

export const M0_OPS: OpSpec[] = [...M0_AUTH, ...M0_TENANTS, ...M0_USERS, ...M0_ROLES, ...M0_ACCESS];

// ============================ MODULE 1 ============================
export const M1_DEPT: OpSpec[] = [
  t(
    "m1.dept.create",
    "Create department",
    "POST",
    "/hr/departments",
    [
      { name: "name", where: "body", required: true },
      { name: "code", where: "body", placeholder: "ENG (2-32)" },
      { name: "description", where: "body" },
      { name: "parent_department_id", where: "body", placeholder: "optional parent dept UUID" },
    ],
    [201],
    { capture: [{ idKey: "deptId", from: "data.id" }], fetchAfter: "m1.dept.get" },
  ),
  t("m1.dept.list", "List departments", "GET", "/hr/departments", pg, [200]),
  t("m1.dept.get", "Get department", "GET", "/hr/departments/:id", [idf("id", "deptId")], [200]),
  t(
    "m1.dept.update",
    "Update department",
    "PATCH",
    "/hr/departments/:id",
    [
      idf("id", "deptId"),
      { name: "name", where: "body" },
      { name: "description", where: "body" },
      { name: "parent_department_id", where: "body" },
    ],
    [200],
  ),
  t(
    "m1.dept.deactivate",
    "Deactivate department",
    "POST",
    "/hr/departments/:id/deactivate",
    [idf("id", "deptId"), { name: "force", where: "query", type: "select", options: ["", "true", "false"] }, { name: "reason", where: "body" }],
    [200],
  ),
];

export const M1_TEAM: OpSpec[] = [
  t(
    "m1.team.create",
    "Create team",
    "POST",
    "/hr/teams",
    [
      { name: "name", where: "body", required: true },
      { name: "department_id", where: "body", ref: "deptId", required: true },
      { name: "code", where: "body" },
      { name: "description", where: "body" },
      { name: "lead_user_id", where: "body" },
    ],
    [201],
    { capture: [{ idKey: "teamId", from: "data.id" }], fetchAfter: "m1.team.get" },
  ),
  t("m1.team.list", "List teams", "GET", "/hr/teams", [...pg, { name: "department_id", where: "query" }], [200]),
  t("m1.team.get", "Get team", "GET", "/hr/teams/:id", [idf("id", "teamId")], [200]),
  t(
    "m1.team.update",
    "Update team",
    "PATCH",
    "/hr/teams/:id",
    [
      idf("id", "teamId"),
      { name: "name", where: "body" },
      { name: "description", where: "body" },
      { name: "department_id", where: "body" },
      { name: "lead_user_id", where: "body" },
    ],
    [200],
  ),
  t("m1.team.deactivate", "Deactivate team", "POST", "/hr/teams/:id/deactivate", [idf("id", "teamId"), { name: "reason", where: "body" }], [200]),
];

export const M1_DESIG: OpSpec[] = [
  t(
    "m1.desig.create",
    "Create designation",
    "POST",
    "/hr/designations",
    [
      { name: "title", where: "body", required: true },
      { name: "code", where: "body" },
      { name: "level", where: "body", type: "number" },
      { name: "description", where: "body" },
    ],
    [201],
    { capture: [{ idKey: "desigId", from: "data.id" }], fetchAfter: "m1.desig.get" },
  ),
  t("m1.desig.list", "List designations", "GET", "/hr/designations", pg, [200]),
  t("m1.desig.get", "Get designation", "GET", "/hr/designations/:id", [idf("id", "desigId")], [200]),
  t(
    "m1.desig.update",
    "Update designation",
    "PATCH",
    "/hr/designations/:id",
    [idf("id", "desigId"), { name: "title", where: "body" }, { name: "level", where: "body", type: "number" }, { name: "description", where: "body" }],
    [200],
  ),
  t("m1.desig.deactivate", "Deactivate designation", "POST", "/hr/designations/:id/deactivate", [idf("id", "desigId"), { name: "reason", where: "body" }], [200]),
];

export const M1_LOC: OpSpec[] = [
  t(
    "m1.loc.create",
    "Create location",
    "POST",
    "/hr/locations",
    [
      { name: "code", where: "body", required: true, placeholder: "BLR-HQ" },
      { name: "name", where: "body", required: true },
      { name: "address_line1", where: "body" },
      { name: "address_line2", where: "body" },
      { name: "city", where: "body", required: true },
      { name: "state_code", where: "body", required: true, placeholder: "KA" },
      { name: "country_code", where: "body", placeholder: "IN" },
      { name: "postal_code", where: "body" },
      { name: "time_zone", where: "body", placeholder: "Asia/Kolkata" },
    ],
    [201],
    { capture: [{ idKey: "locationId", from: "data.id" }], fetchAfter: "m1.loc.get" },
  ),
  t(
    "m1.loc.list",
    "List locations",
    "GET",
    "/hr/locations",
    [...pg, { name: "status", where: "query", type: "select", options: ["", "active", "inactive"] }, { name: "state_code", where: "query" }, { name: "search", where: "query" }],
    [200],
  ),
  t("m1.loc.get", "Get location", "GET", "/hr/locations/:id", [idf("id", "locationId")], [200]),
  t(
    "m1.loc.update",
    "Update location (code is immutable)",
    "PATCH",
    "/hr/locations/:id",
    [
      idf("id", "locationId"),
      { name: "name", where: "body" },
      { name: "address_line1", where: "body" },
      { name: "city", where: "body" },
      { name: "state_code", where: "body" },
      { name: "postal_code", where: "body" },
      { name: "time_zone", where: "body" },
    ],
    [200],
  ),
  t("m1.loc.deactivate", "Deactivate location (409 while mappings use it)", "POST", "/hr/locations/:id/deactivate", [idf("id", "locationId")], [200]),
  t("m1.loc.activate", "Activate location", "POST", "/hr/locations/:id/activate", [idf("id", "locationId")], [200]),
];

export const M1_MAP: OpSpec[] = [
  t(
    "m1.map.create",
    "Create mapping (user ↔ dept/team/designation)",
    "POST",
    "/hr/mappings",
    [
      { name: "user_id", where: "body", ref: "userId", required: true },
      { name: "department_id", where: "body", ref: "deptId" },
      { name: "team_id", where: "body", ref: "teamId" },
      { name: "designation_id", where: "body", ref: "desigId" },
      { name: "location_id", where: "body", ref: "locationId" },
      { name: "is_primary", where: "body", type: "select", options: ["", "true", "false"] },
      { name: "manager_user_id", where: "body" },
    ],
    [201],
    { capture: [{ idKey: "mappingId", from: "data.id" }], fetchAfter: "m1.map.get" },
  ),
  t("m1.map.list", "List mappings of a user", "GET", "/hr/mappings", [...pg, { name: "user_id", where: "query", ref: "userId", required: true }], [200]),
  t("m1.map.get", "Get mapping", "GET", "/hr/mappings/:id", [idf("id", "mappingId")], [200]),
  t(
    "m1.map.update",
    "Update mapping",
    "PATCH",
    "/hr/mappings/:id",
    [
      idf("id", "mappingId"),
      { name: "department_id", where: "body" },
      { name: "team_id", where: "body" },
      { name: "designation_id", where: "body" },
      { name: "location_id", where: "body" },
      { name: "is_primary", where: "body", type: "select", options: ["", "true", "false"] },
      { name: "manager_user_id", where: "body" },
    ],
    [200],
  ),
  t("m1.map.deactivate", "Deactivate mapping", "POST", "/hr/mappings/:id/deactivate", [idf("id", "mappingId"), { name: "reason", where: "body" }], [200]),
];

export const M1_CHART: OpSpec[] = [
  t(
    "m1.chart.tree",
    "Org chart (department tree)",
    "GET",
    "/hr/org-chart",
    [
      { name: "max_depth", where: "query", type: "number", placeholder: "10" },
      { name: "include_inactive", where: "query", type: "select", options: ["", "true", "false"] },
    ],
    [200],
  ),
  t("m1.chart.chain", "Reporting chain of a user", "GET", "/hr/org-chart/chain", [{ name: "user_id", where: "query", ref: "userId", required: true }], [200]),
];

export const M1_OPS: OpSpec[] = [...M1_DEPT, ...M1_TEAM, ...M1_DESIG, ...M1_LOC, ...M1_MAP, ...M1_CHART];

// ============================ MODULE 2 ============================
const EMP_TYPES = ["", "full_time", "part_time", "contract", "intern"];
const EMP_STATUS = ["", "active", "probation", "notice", "terminated", "resigned", "on_leave", "inactive"];
export const M2_EMP: OpSpec[] = [
  t(
    "m2.emp.create",
    "Create employee",
    "POST",
    "/hr/employees",
    [
      { name: "user_id", where: "body", ref: "userId", required: true },
      { name: "employee_code", where: "body", required: true, placeholder: "EMP-001 (A-Z0-9-)" },
      { name: "first_name", where: "body", required: true },
      { name: "last_name", where: "body", required: true },
      { name: "employment_type", where: "body", type: "select", options: EMP_TYPES, required: true, def: "full_time" },
      { name: "joining_date", where: "body", type: "datetime", required: true, placeholder: "2026-01-15" },
      { name: "display_name", where: "body" },
      { name: "gender", where: "body" },
      { name: "date_of_birth", where: "body", type: "datetime", placeholder: "1990-05-20" },
      { name: "marital_status", where: "body" },
      { name: "blood_group", where: "body" },
      { name: "avatar_url", where: "body" },
      { name: "probation_end_date", where: "body", type: "datetime" },
      { name: "notice_period_days", where: "body", type: "number" },
      { name: "personal_email", where: "body" },
      { name: "work_phone", where: "body" },
      { name: "personal_phone", where: "body" },
      { name: "current_address", where: "body" },
      { name: "permanent_address", where: "body" },
    ],
    [201],
    { capture: [{ idKey: "employeeId", from: "data.id" }], fetchAfter: "m2.emp.get" },
  ),
  t(
    "m2.emp.list",
    "List employees",
    "GET",
    "/hr/employees",
    [...pg, { name: "status", where: "query", type: "select", options: EMP_STATUS }, { name: "search", where: "query" }, { name: "department_id", where: "query" }],
    [200],
  ),
  t("m2.emp.get", "Get employee", "GET", "/hr/employees/:id", [idf("id", "employeeId")], [200]),
  t(
    "m2.emp.update",
    "Update employee (profile fields)",
    "PATCH",
    "/hr/employees/:id",
    [
      idf("id", "employeeId"),
      { name: "first_name", where: "body" },
      { name: "last_name", where: "body" },
      { name: "display_name", where: "body" },
      { name: "gender", where: "body" },
      { name: "date_of_birth", where: "body", type: "datetime" },
      { name: "marital_status", where: "body" },
      { name: "blood_group", where: "body" },
      { name: "avatar_url", where: "body" },
    ],
    [200],
  ),
  t(
    "m2.emp.status",
    "Transition status",
    "POST",
    "/hr/employees/:id/status",
    [
      idf("id", "employeeId"),
      { name: "status", where: "body", type: "select", options: EMP_STATUS, required: true },
      { name: "confirmation_date", where: "body", type: "datetime" },
      { name: "resignation_date", where: "body", type: "datetime" },
      { name: "exit_date", where: "body", type: "datetime" },
      { name: "exit_reason", where: "body" },
      { name: "notes", where: "body" },
    ],
    [200],
  ),
  t("m2.emp.deactivate", "Deactivate employee", "POST", "/hr/employees/:id/deactivate", [idf("id", "employeeId")], [200]),
  t("m2.emp.me", "Get my employee profile (self-service)", "GET", "/hr/employees/me", [], [200], {
    note: "Uses the token of the logged-in user; that user needs an employee profile.",
  }),
  t(
    "m2.emp.updateMe",
    "Update my contact (self-service)",
    "PATCH",
    "/hr/employees/me",
    [
      { name: "personal_phone", where: "body" },
      { name: "current_address", where: "body" },
      { name: "permanent_address", where: "body" },
      { name: "emergency_contacts", where: "body" },
    ],
    [200],
  ),
];

export const M2_STAT: OpSpec[] = [
  t("m2.stat.get", "Get statutory & bank details", "GET", "/hr/employees/:id/statutory", [idf("id", "employeeId"), { name: "unmasked", where: "query", type: "select", options: ["", "true"] }], [200]),
  t(
    "m2.stat.put",
    "Upsert statutory & bank details",
    "PUT",
    "/hr/employees/:id/statutory",
    [
      idf("id", "employeeId"),
      { name: "tax_id", where: "body" },
      { name: "national_id", where: "body" },
      { name: "bank_name", where: "body" },
      { name: "bank_account_number", where: "body" },
      { name: "bank_routing_swift", where: "body" },
    ],
    [200],
  ),
];

export const M2_DOC: OpSpec[] = [
  t(
    "m2.doc.upload",
    "Register document (metadata + URL)",
    "POST",
    "/hr/employees/:id/documents",
    [
      idf("id", "employeeId"),
      { name: "document_type", where: "body", required: true, placeholder: "passport" },
      { name: "file_name", where: "body", required: true, placeholder: "passport.pdf" },
      { name: "file_url", where: "body", required: true, placeholder: "https://example.com/passport.pdf" },
      { name: "file_size", where: "body", type: "number", required: true, placeholder: "1024" },
      { name: "mime_type", where: "body", required: true, placeholder: "application/pdf" },
    ],
    [201],
    { capture: [{ idKey: "docId", from: "data.id" }], note: "The backend stores metadata only (no file upload endpoint)." },
  ),
  t("m2.doc.list", "List documents", "GET", "/hr/employees/:id/documents", [idf("id", "employeeId")], [200]),
  t("m2.doc.verify", "Verify document", "POST", "/hr/employees/:id/documents/:doc_id/verify", [idf("id", "employeeId"), idf("doc_id", "docId")], [200]),
  t("m2.timeline.list", "Timeline", "GET", "/hr/employees/:id/timeline", [idf("id", "employeeId")], [200]),
];

export const M2_OPS: OpSpec[] = [...M2_EMP, ...M2_STAT, ...M2_DOC];

export const ALL_OPS: OpSpec[] = [...M0_OPS, ...M1_OPS, ...M2_OPS];
export const OP_BY_KEY: Record<string, OpSpec> = Object.fromEntries(ALL_OPS.map((o) => [o.key, o]));

// ============================ CHECKLISTS ============================
export const CHECKLIST: Record<0 | 1 | 2, ChecklistItem[]> = {
  0: [
    { label: "Login", keys: ["m0.login"] },
    { label: "Logout", keys: ["m0.logout", "s0.session"] },
    { label: "Refresh (+ rotation / reuse detection in Session scenario)", keys: ["m0.refresh"] },
    { label: "Forgot / reset password", keys: ["m0.auth.forgot"] },
    { label: "Tenant lifecycle (create/get/update/suspend/activate)", keys: ["s0.tenant"] },
    { label: "User lifecycle", keys: ["s0.user"] },
    { label: "Roles (+unassign/delete guards)", keys: ["s0.role", "s0.access"] },
    { label: "Permissions", keys: ["m0.perm.list"] },
    { label: "RBAC enforcement", keys: ["s0.rbac"] },
    { label: "Tenant isolation", keys: ["s0.isolation"] },
    { label: "Audit log", keys: ["s0.access"] },
    { label: "Error handling / auth negatives", keys: ["s0.errors"] },
    { label: "Predefined roles, scopes, inheritance", keys: ["s0.scopes"] },
    { label: "Personal grants and blocks", keys: ["s0.grants"] },
    { label: "Automatic Employee / Manager roles", keys: ["s0.auto"] },
  ],
  1: [
    { label: "Departments (CRUD + hierarchy)", keys: ["s1.dept"] },
    { label: "Teams", keys: ["s1.team"] },
    { label: "Designations", keys: ["s1.desig"] },
    { label: "User mappings", keys: ["s1.map"] },
    { label: "Org chart & reporting chain", keys: ["s1.chart"] },
    { label: "RBAC (organization:read/update)", keys: ["s1.rbac"] },
    { label: "Tenant isolation (org data)", keys: ["s0.isolation"] },
    { label: "Validation / error handling", keys: ["s1.errors"] },
  ],
  2: [
    { label: "Employee create / read / update", keys: ["s2.emp"] },
    { label: "Status transitions + timeline", keys: ["s2.status"] },
    { label: "Statutory & bank details (masking)", keys: ["s2.stat"] },
    { label: "Documents (upload / verify)", keys: ["s2.doc"] },
    { label: "Self-service (/employees/me)", keys: ["s2.me"] },
    { label: "RBAC (employee:* permissions)", keys: ["s2.rbac"] },
    { label: "Validation / error handling", keys: ["s2.errors"] },
  ],
};
