# Module 1 — Specification (FROZEN at P1 — contract for P2 implementation)

Status: FROZEN 2026-10-01 (plan.md P1). P2 must implement exactly this. Changes need decisions.md + gate log entry.

## 1. Purpose

Own the company structure inside a tenant: departments, teams, designations, reporting hierarchy, employee-to-org mappings, and the org-chart read model. Module 1 is structural data only — employment semantics (joining dates, compensation, attendance) belong to Module 2+.

## 2. Functional requirements

### Departments (FR-D)

| ID | Requirement |
|----|-------------|
| FR-D001 | Create a department in the authenticated tenant: name (required, 1–100), code (optional, unique per tenant, slug-like `^[a-z0-9-]{2,32}$`), description, parent department (optional, same tenant). |
| FR-D002 | Department names unique per tenant (case-insensitive). Codes unique per tenant when set. |
| FR-D003 | Get department by ID (tenant-scoped, 404 otherwise). |
| FR-D004 | List departments paginated (`page`/`per_page`, defaults 1/20, max 100). Support `parent_id` filter + `root_only` flag. |
| FR-D005 | Update name/description/code/parent (code immutable once set — ignored, not error; mirrors TN slug rule). |
| FR-D006 | Parent must exist in the same tenant; parent chain must stay acyclic (see FR-H). |
| FR-D007 | Deactivate department (soft status `active`/`inactive`, default active). Deactivation blocked (409) while active teams or active mappings reference it unless `force=true` cascades deactivation to child departments only (never deletes). |
| FR-D008 | Soft delete departments (`deleted_at`); slugs/codes/emails of soft-deleted rows stay reserved. |

### Teams (FR-T)

| ID | Requirement |
|----|-------------|
| FR-T001 | Create a team in the authenticated tenant: name (1–100), code (optional, unique per tenant), department (required, same tenant), lead user (optional, must be an active user in the tenant per Module 0 UserService). |
| FR-T002 | Team names unique per department (case-insensitive); codes unique per tenant when set. |
| FR-T003 | Get team by ID (tenant-scoped). |
| FR-T004 | List teams paginated with `department_id` filter. |
| FR-T005 | Update name/description/code/lead (code immutable once set). Moving a team across departments allowed; audit old/new department. |
| FR-T006 | Deactivate team (status flag). Blocked while active mappings reference it unless reassigned first (409 with conflicting mapping IDs). |
| FR-T007 | Soft delete teams; names/codes of soft-deleted rows stay reserved. |

### Designations (FR-DG)

| ID | Requirement |
|----|-------------|
| FR-DG001 | Create a designation (job title/level): title (1–100), code (optional, unique per tenant), level (optional int ≥1, higher = senior), description. |
| FR-DG002 | Titles unique per tenant (case-insensitive); codes unique per tenant when set. |
| FR-DG003 | Get/List (paginated, `level` ordering), Update (code immutable), Deactivate, soft delete — same lifecycle as departments. |
| FR-DG004 | Deactivation blocked while active mappings reference the designation (409) unless mappings are migrated first. |

### Reporting hierarchy (FR-H)

| ID | Requirement |
|----|-------------|
| FR-H001 | Departments form a forest: `parent_department_id` nullable; exactly one path to a root; cycles rejected (409 `hierarchy.cycle`). |
| FR-H002 | Team→department edge is many-to-one and mandatory. |
| FR-H003 | Setting a parent validates: parent exists, same tenant, not self, not a descendant (cycle check in tx). |
| FR-H004 | Depth cap: 10 levels. Exceeding returns 422 with the offending chain. |
| FR-H005 | Moving a subtree revalidates the whole subtree (depth + acyclicity) atomically; failure rolls back. |

### Employee mappings (FR-M)

| ID | Requirement |
|----|-------------|
| FR-M001 | Map a user to org: `user_id` (must resolve via Module 0 UserService: exists, same tenant, active), `department_id` (optional), `team_id` (optional), `designation_id` (optional), `is_primary` bool, `manager_user_id` (optional, active same-tenant user, must not equal self). |
| FR-M002 | One primary mapping per user per tenant (`unique(tenant_id, user_id, is_primary=true)` effectively; enforced in service + partial unique index). Secondary mappings allowed. |
| FR-M003 | Manager chain must stay acyclic (user A cannot transitively manage themselves); validated in tx, 409 on cycle. |
| FR-M004 | Team mapping requires the team to belong to the mapping's department when both set (or department derived from team). |
| FR-M005 | On Module 0 `UserDeactivated` event: mark mappings `inactive`, clear `manager_user_id` references to that user (reassign or null with audit). |
| FR-M006 | On Module 0 `UserCreated`: no auto-mapping (explicit mapping only — decision, see decisions.md). |
| FR-M007 | Unmap (deactivate mapping) with reason + audit; history retained (no hard delete). |

### Org chart read model (FR-O)

| ID | Requirement |
|----|-------------|
| FR-O001 | `GET /org-chart` returns the tenant forest: departments with nested children, teams per department, headcount per node (active mappings), lead user per team. |
| FR-O002 | `GET /org-chart?user_id=` returns the user's chain to root (user → team → department → ancestors) + direct reports (users with `manager_user_id` = user). |
| FR-O003 | Read model is computed from transactional tables (no separate store in v1); response cached 60s in Redis, invalidated on any FR-D/T/DG/H/M write. |
| FR-O004 | Depth/pruning params: `max_depth` (default full, max 10), `include_inactive=false` default. |

### Events consumed/produced

| ID | Requirement |
|----|-------------|
| FR-E001 | Consume `identity.tenant.created` → no-op record check (tenant exists via Module 0 read; org starts empty). |
| FR-E002 | Consume `identity.tenant.suspended` → freeze all writes (403 `tenant.suspended` on mutating endpoints until active). |
| FR-E003 | Consume `identity.user.created` → no auto-mapping (FR-M006). |
| FR-E004 | Consume `identity.user.deactivated` → FR-M005. |
| FR-E005 | Produce `organization.department.created/deactivated`, `organization.team.created/moved/deactivated`, `organization.mapping.created/updated/deactivated`, `organization.hierarchy.moved` on exchange `jaas.organization.events` (topic, durable), routing `organization.{resource}.{action}`, Module 0 envelope (event_id/type/version/occurred_at/producer/tenant_id/correlation_id/payload). Publish after commit; queue-down never blocks API (fallback table + retry, same pattern as Module 0). |

## 3. Non-functional

| ID | Requirement | Target |
|----|-------------|--------|
| NFR-P001 | Single-resource GET | p95 <200ms |
| NFR-P002 | List (100 items) | p95 <500ms |
| NFR-P003 | Org-chart full tenant (≤5k nodes) | p95 <800ms (cached <100ms) |
| NFR-P004 | DB query | p95 <50ms |
| NFR-S001 | Tenants/tenant scale inherits Module 0 (10k tenants); per tenant ≤10k departments, ≤50k teams, ≤500k mappings | — |
| NFR-SEC001 | All endpoints require JWT; tenant_id from JWT == subdomain tenant; every query tenant-scoped | — |
| NFR-SEC002 | Hierarchy/mapping writes require `organization:update`; reads require `organization:read` (new permissions, seeded) | — |
| NFR-D001 | Multi-table writes in tx (move subtree, mapping+manger updates) | — |
| NFR-D002 | UUID PKs; soft deletes for departments/teams/designations; mappings append-only (status flag, no hard delete) | — |

## 4. Inputs/Outputs (DTO shapes — P2 must match exactly)

- `CreateDepartmentRequest{name, code?, description?, parent_department_id?}` → `DepartmentResponse{id, tenant_id, name, code?, description?, parent_department_id?, status, depth, path[], created_at, updated_at}`.
- `UpdateDepartmentRequest{name?, description?, code? (ignored), parent_department_id?}`.
- `CreateTeamRequest{name, code?, description?, department_id, lead_user_id?}` → `TeamResponse{..., member_count}`.
- `CreateDesignationRequest{title, code?, level?, description?}` → `DesignationResponse{...}`.
- `CreateMappingRequest{user_id, department_id?, team_id?, designation_id?, is_primary, manager_user_id?}` → `MappingResponse{..., status}`.
- Lists: `{data[], meta{page, per_page, total_items, total_pages}}`. Errors: Module 0 envelope `{error:{code,message,details}}`.
- Validation tags: `binding:"required,min=1,max=100"` etc.; codes `omitempty,min=2,max=32`; UUIDs `uuid`; level `omitempty,min=1`.

## 5. Validation rules

- VL-001 names/titles non-empty after trim, ≤100. VL-002 codes match `^[a-z0-9-]{2,32}$`, reserved list shared with tenant slugs where colliding (`api, admin, ...`) rejected. VL-003 UUID fields valid v4. VL-004 pagination page ≥1 (default 1), per_page 1–100 (default 20). VL-005 unknown JSON fields ignored. VL-006 malformed JSON → 400.

## 6. Error behavior

| Status | Code | When |
|---|---|---|
| 400 | VALIDATION_ERROR | bad body/fields, depth exceeded (422 variant `HIERARCHY_TOO_DEEP` for FR-H004), bad UUID |
| 401 | UNAUTHORIZED | missing/invalid JWT |
| 403 | FORBIDDEN | no `organization:*` permission; tenant suspended (code TENANT_SUSPENDED) |
| 404 | NOT_FOUND | id not in tenant (incl. cross-tenant → 404, never 403 leak) |
| 409 | CONFLICT | duplicate name/code, cycle (`hierarchy.cycle`), deactivate-while-referenced |

## 7. Edge cases

- EC-01 create dept with parent in another tenant → 404 parent. EC-02 set parent = self → 400. EC-03 set parent = descendant → 409 cycle. EC-04 move subtree exceeding depth 10 → 422 + chain. EC-05 map inactive/nonexistent user → 404 (same message; no enumeration of user existence beyond Module 0 semantics). EC-06 map user from another tenant → 404. EC-07 two primaries for one user → 409 (second wins only via explicit `make_primary` swap in tx). EC-08 manager = self → 400; manager cycle A→B→A → 409. EC-09 team move across departments with mappings → allowed, mappings follow team, audit records old/new. EC-10 tenant suspended mid-write → tx rolls back, 403. EC-11 concurrent same-name creates → unique constraint wins, loser 409. EC-12 org-chart on empty tenant → `{data: [], meta}` 200. EC-13 `UserDeactivated` race with mapping write → mapping write fails closed (user inactive → 404/409), event converges state.

## 8. Out of scope (explicit)

Employment lifecycle (joining/exit dates, probation, compensation, documents) → Module 2. Attendance/leave balances → Module 3. Project assignment semantics → Module 4. Approval chains over org changes (deactivate-force approvals) → Module 6 (v1: `force` flag with audit, no workflow). Bulk CSV import → v1.1. Multi-root cross-tenant reporting → never. Email sending → Module 7 via events.
