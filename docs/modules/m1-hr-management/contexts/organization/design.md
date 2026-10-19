# HR / Organization — Design

> Part of **M1 HR Management** (bounded context `organization`). Formerly the stand-alone "module-1-organization" bundle; module numbers in older text were renamed to the canonical M0–M12 / HR-context names.

## Architecture

### 1. Layers (mirrors M0 Identity Clean + DDD)

```text
Kong → Gin → M0 Identity middleware (CORS → RateLimit → TenantResolver → Auth)
→ Org RBAC (organization:read / organization:update via M0 Identity RequirePermission)
→ Org Controllers (department, team, designation, mapping, org_chart)
→ Org Services (DepartmentService, TeamService, DesignationService, MappingService, OrgChartService)
→ Org Repositories (department, team, designation, mapping — all tenant_id-scoped, *gorm.DB tx injection)
→ PostgreSQL (truth) / Redis (org-chart 60s cache + rate via M0 Identity) / RabbitMQ (jaas.organization.events)
```

M0 Identity owns authN/authZ primitives, sessions, audit transport. HR/Organization owns org tables, hierarchy invariants, mapping rules, chart reads, org events. No duplicated identity logic: user existence/activity checks call M0 Identity UserService/reads; permission checks reuse `RequirePermission`; audit entries go through M0 Identity AuditService.

### 2. Components

- `models/`: department.go (TenantBaseModel + name/code/description/parent_id/status), team.go (+department_id/lead_user_id), designation.go (+title/level), mapping.go (user/department/team/designation/is_primary/manager/status, append-only), all UUID PKs, soft delete on dept/team/designation, partial unique index for primary mapping.
- `repositories/`: interfaces.go + 4 impls. Methods: Create/FindByID/FindAll(paginated+filters)/Update/Delete(soft)/FindChildren/FindByParent/FindMappingsByUser/FindReports. Not-found → nil,nil. Parameterized only.
- `services/`: DepartmentService (CRUD + move-subtree + deactivate-guard), TeamService (CRUD + move + lead validation), DesignationService (CRUD), MappingService (create/swap-primary/update/deactivate + manager-cycle check + team-department consistency + UserDeactivated convergence), OrgChartService (forest build + chain-to-root + headcounts + 60s cache). All take DTOs + context.Context, never *gin.Context; tx for multi-table ops; events after commit.
- `controllers/` + `routes/`: REST groups `/departments`, `/teams`, `/designations`, `/mappings`, `/org-chart` under `/api/v1`, M0 Identity envelope responses, validation via shared utils + custom org validators (code format, cycle pre-check).
- `events/events.go`: 7 produced types (payloads mirror M0 Identity envelope); consumer handlers for 4 identity events (tenant.created/suspended, user.created noop, user.deactivated converge).
- `validators/`: org code format, reserved codes, depth guard.

### 3. Data flow (write: move team)

```text
PATCH /teams/:id {department_id} → validate DTO → RBAC organization:update → tx begin
→ load team + target dept (same tenant?) → revalidate mappings (team-dept consistency)
→ update + audit + commit → publish organization.team.moved → invalidate org-chart cache
```

### 4. Control flow (hierarchy guard)

Every parent/manager mutation runs `assertAcyclic` inside the same tx before write: walk ancestors (departments) or manager chain (mappings) with visited-set, cap 10/chain cap 50 respectively; violation → rollback + 409/422. Reads never lock.

### 5. External/internal dependencies

External: M0 Identity repo/service contracts (tenant resolve, user read, RBAC, audit, events envelope, middleware). Internal: services depend on repo interfaces only; chart service reads repos, never services' tx handles. No cycles: controllers → services → repos; events → queue interface.

### 6. Failure paths

DB down → 503 + rollback (same as M0 Identity EC-DB). Redis down → chart cache skipped (compute direct), rate via M0 Identity fallback. Queue down → best-effort + fallback table + retry worker (same pattern). Tenant suspended → 403 before any write. Cycle/depth violation → 409/422 with chain, no partial write.

### 7. Caching

Org-chart full forest cached 60s keyed `org:chart:{tenant_id}:{max_depth}:{inactive_flag}`; per-user chain cached 60s `org:chain:{tenant}:{user}`. Invalidated on any FR-D/T/DG/H/M write in the tenant. Permission checks reuse M0 Identity 5m cache. No cache for writes.

### 8. Persistence

Tables: `departments`, `teams`, `designations`, `mappings` (+ `org_events_outbox` fallback). FKs: teams.department_id → departments; mappings.* → respective tables + users (logical FK to M0 Identity, enforced in service since cross-module hard FK is avoided — decision, see decisions.md). Indexes: tenant_id on all; unique(tenant_id, lower(name)) per scope; unique codes; parent/manager indexes for traversal.

### 9. Concurrency

Same-name/code races → unique constraints win (409 loser). Concurrent subtree moves → row locks in tx order (parent→child); deadlock → retry once → 503. Mapping primary swaps serialized per user (tx + partial unique). Event consumers idempotent via event_id.

### 10. WHY

- Separate HR/Organization (not columns on users): org has its own lifecycle, hierarchy invariants, and 4 consumers — embedding would couple identity to structure.
- Logical (not hard) FK to users: M0 Identity owns users; hard cross-module FKs freeze independent deploys. Service-level checks + UserDeactivated convergence keep integrity without coupling.
- Computed chart + short cache (not materialized): v1 scale fits compute; invalidation is trivial; no dual-write drift. Materialize only if NFR-P003 breaches at scale.
- Append-only mappings: history answers "who was where when" for audit/payroll/approvals without temporal tables in v1.

## Connections

Depends On: M0 Identity (Tenant, User, RBAC) + shared infra. Consumed By: HR/Employee, HR/Attendance & Leave, M2 Projects & Tasks, M3 Meetings.

### Edge table

| # | Source → Destination | Protocol | Contract | Auth | Format | Failure | Owner | Tests |
|---|---|---|---|---|---|---|---|---|
| C1 | Client → HR/Organization API (`/api/v1/departments, /teams, /designations, /mappings, /org-chart`) | HTTPS (Kong) → HTTP internal | This spec §4 + swagger (P2) | JWT (M0 Identity Auth middleware); `organization:read/update` via RequirePermission | JSON envelope `{data,meta}` / `{error}` | 401/403/404/409 per spec §6; 503 DB down | HR/Organization | API contract tests (P3) |
| C2 | HR/Organization → M0 Identity UserService (user existence/activity) | In-process Go call | M0 Identity `UserService.GetByID/ListUsers` (merged) | Service-level (no JWT; tenant_id passed explicitly) | Go structs | User missing/inactive → 404/409, no write | M0 Identity (provider), HR/Organization (consumer) | Mapping service unit (mocked UserService) |
| C3 | HR/Organization → M0 Identity RBAC (`RequirePermission` middleware) | In-process Gin middleware | `RequirePermission(db, resource, action, ...)` (merged routes.go pattern) | JWT context roles; tenant_admin bypass preserved | Gin context | Deny → 403 | M0 Identity | Middleware tests (P3) |
| C4 | HR/Organization → M0 Identity AuditService | In-process Go call | `AuditService.Log(...)` (merged) | Service-level | Go structs | Best-effort; audit failure never fails API | M0 Identity | Service unit |
| C5 | HR/Organization → PostgreSQL (org tables) | TCP/5432 GORM/pgx pool (shared) | Migrations 000013.. (P2) + §8 schema | DB user/pass (env) | SQL | Conn fail → 503 + rollback; deadlock → retry once → 503 | HR/Organization | Repo integration (Docker PG) |
| C6 | HR/Organization → Redis (chart cache 60s) | TCP/6379 (shared client) | Keys `org:chart:{tenant}:*`, `org:chain:{tenant}:{user}`; TTL 60s | AUTH+TLS prod | JSON | Miss/down → compute direct, log | shared/cache | Service unit (fake) |
| C7 | Identity events → HR/Organization consumers | AMQP (`jaas.identity.events`) | M0 Identity events.md envelope; keys `identity.tenant.created/suspended`, `identity.user.created/deactivated` | Queue creds (env) | JSON envelope | Redeliver → idempotent via event_id; poison → DLQ | M0 Identity (producer), HR/Organization (consumer) | Consumer tests (P3) |
| C8 | HR/Organization → `jaas.organization.events` (7 produced types) | AMQP topic exchange (durable) | This spec FR-E005; envelope = M0 Identity shape; version 1.0 | Queue creds (env) | JSON envelope | Publish after commit; failure → outbox + retry; never blocks API | HR/Organization | Producer tests (P3) |
| C9 | HR/Employee, HR/Attendance & Leave, M2 Projects & Tasks, M3 Meetings → HR/Organization reads (departments/teams/mappings/chart) | In-process Go calls (same deployable v1) | `DepartmentService/TeamService/MappingService/OrgChartService` read methods (P2) | Service-level + tenant_id | Go structs/DTOs | Missing → nil,nil → consumer 404 | HR/Organization (provider) | Contract tests (P3) |

### Trust boundary

Everything outside the Gin handler is untrusted. JWT tid == subdomain tenant (M0 Identity). Every org query filters `tenant_id`. Cross-tenant IDs → 404 (never confirm existence). User references validated against M0 Identity per write. No org endpoint is public.

## Security

Law: docs/SECURITY.md (read before any security-sensitive change).

### Module-specific

- Data class: internal structure (Medium). No salary, no credentials, no tokens in org tables. Lead/manager references are user IDs only — never embed emails, hashes, or tokens in org responses or events.
- Permissions (new, seeded with HR/Organization deploy): `organization:read` (departments/teams/designations/mappings/chart reads), `organization:update` (all writes incl. moves, mapping changes, deactivations). Tenant-scoped enforcement via M0 Identity `RequirePermission`; `tenant_admin` bypass preserved.
- Trust boundary: same as M0 Identity — JWT tid == subdomain tenant; every query filters `tenant_id`; cross-tenant IDs → 404. Manager/lead `user_id` inputs validated against M0 Identity (exists + same tenant + active) — never trusted from body alone.
- Rate limits: inherit M0 Identity table; org writes fall under default 100/min per user. Chart reads cached, not rate-exempt.
- Audit: every FR-D/T/DG/H/M mutation writes an immutable M0 Identity audit row (actor, resource, resource_id, diff old/new, IP, user agent). Reads are not audited except org-chart exports (flagged, audited).
- PII: mapping responses include user IDs + org position only. No bulk export endpoint in v1 (prevents silent exfiltration; export arrives with M6 Reports & Analytics + consent + audit).
- Attack surfaces: hierarchy cycle payloads (guarded FR-H003/FR-M003), depth bombs (FR-H004 cap), IDOR via ID guessing (tenant scoping + 404), event spoofing (consumers verify tenant_id + event_id idempotency, ignore unknown producers).
- Failure: tenant suspended → 403 on all mutating endpoints (FR-E002); DB down → 503 + rollback; cache down → compute direct.

Pre-production checks (§12 of docs/SECURITY.md) apply in full at P3 exit.
