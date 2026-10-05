# Module 1 — Architecture (FROZEN at P1)

## 1. Layers (mirrors Module 0 Clean + DDD)

```text
Kong → Gin → Module 0 middleware (CORS → RateLimit → TenantResolver → Auth)
→ Org RBAC (organization:read / organization:update via Module 0 RequirePermission)
→ Org Controllers (department, team, designation, mapping, org_chart)
→ Org Services (DepartmentService, TeamService, DesignationService, MappingService, OrgChartService)
→ Org Repositories (department, team, designation, mapping — all tenant_id-scoped, *gorm.DB tx injection)
→ PostgreSQL (truth) / Redis (org-chart 60s cache + rate via Module 0) / RabbitMQ (jaas.organization.events)
```

Module 0 owns authN/authZ primitives, sessions, audit transport. Module 1 owns org tables, hierarchy invariants, mapping rules, chart reads, org events. No duplicated identity logic: user existence/activity checks call Module 0 UserService/reads; permission checks reuse `RequirePermission`; audit entries go through Module 0 AuditService.

## 2. Components

- `models/`: department.go (TenantBaseModel + name/code/description/parent_id/status), team.go (+department_id/lead_user_id), designation.go (+title/level), mapping.go (user/department/team/designation/is_primary/manager/status, append-only), all UUID PKs, soft delete on dept/team/designation, partial unique index for primary mapping.
- `repositories/`: interfaces.go + 4 impls. Methods: Create/FindByID/FindAll(paginated+filters)/Update/Delete(soft)/FindChildren/FindByParent/FindMappingsByUser/FindReports. Not-found → nil,nil. Parameterized only.
- `services/`: DepartmentService (CRUD + move-subtree + deactivate-guard), TeamService (CRUD + move + lead validation), DesignationService (CRUD), MappingService (create/swap-primary/update/deactivate + manager-cycle check + team-department consistency + UserDeactivated convergence), OrgChartService (forest build + chain-to-root + headcounts + 60s cache). All take DTOs + context.Context, never *gin.Context; tx for multi-table ops; events after commit.
- `controllers/` + `routes/`: REST groups `/departments`, `/teams`, `/designations`, `/mappings`, `/org-chart` under `/api/v1`, Module 0 envelope responses, validation via shared utils + custom org validators (code format, cycle pre-check).
- `events/events.go`: 7 produced types (payloads mirror Module 0 envelope); consumer handlers for 4 identity events (tenant.created/suspended, user.created noop, user.deactivated converge).
- `validators/`: org code format, reserved codes, depth guard.

## 3. Data flow (write: move team)

```text
PATCH /teams/:id {department_id} → validate DTO → RBAC organization:update → tx begin
→ load team + target dept (same tenant?) → revalidate mappings (team-dept consistency)
→ update + audit + commit → publish organization.team.moved → invalidate org-chart cache
```

## 4. Control flow (hierarchy guard)

Every parent/manager mutation runs `assertAcyclic` inside the same tx before write: walk ancestors (departments) or manager chain (mappings) with visited-set, cap 10/chain cap 50 respectively; violation → rollback + 409/422. Reads never lock.

## 5. External/internal dependencies

External: Module 0 repo/service contracts (tenant resolve, user read, RBAC, audit, events envelope, middleware). Internal: services depend on repo interfaces only; chart service reads repos, never services' tx handles. No cycles: controllers → services → repos; events → queue interface.

## 6. Failure paths

DB down → 503 + rollback (same as Module 0 EC-DB). Redis down → chart cache skipped (compute direct), rate via Module 0 fallback. Queue down → best-effort + fallback table + retry worker (same pattern). Tenant suspended → 403 before any write. Cycle/depth violation → 409/422 with chain, no partial write.

## 7. Caching

Org-chart full forest cached 60s keyed `org:chart:{tenant_id}:{max_depth}:{inactive_flag}`; per-user chain cached 60s `org:chain:{tenant}:{user}`. Invalidated on any FR-D/T/DG/H/M write in the tenant. Permission checks reuse Module 0 5m cache. No cache for writes.

## 8. Persistence

Tables: `departments`, `teams`, `designations`, `mappings` (+ `org_events_outbox` fallback). FKs: teams.department_id → departments; mappings.* → respective tables + users (logical FK to Module 0, enforced in service since cross-module hard FK is avoided — decision, see decisions.md). Indexes: tenant_id on all; unique(tenant_id, lower(name)) per scope; unique codes; parent/manager indexes for traversal.

## 9. Concurrency

Same-name/code races → unique constraints win (409 loser). Concurrent subtree moves → row locks in tx order (parent→child); deadlock → retry once → 503. Mapping primary swaps serialized per user (tx + partial unique). Event consumers idempotent via event_id.

## 10. WHY

- Separate Module 1 (not columns on users): org has its own lifecycle, hierarchy invariants, and 4 consumers — embedding would couple identity to structure.
- Logical (not hard) FK to users: Module 0 owns users; hard cross-module FKs freeze independent deploys. Service-level checks + UserDeactivated convergence keep integrity without coupling.
- Computed chart + short cache (not materialized): v1 scale fits compute; invalidation is trivial; no dual-write drift. Materialize only if NFR-P003 breaches at scale.
- Append-only mappings: history answers "who was where when" for audit/payroll/approvals without temporal tables in v1.
