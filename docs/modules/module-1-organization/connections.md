# Module 1 — Connections (FROZEN at P1)

Depends On: Module 0 (Tenant, User, RBAC) + shared infra. Consumed By: Modules 2, 3, 4, 5.

## Edge table

| # | Source → Destination | Protocol | Contract | Auth | Format | Failure | Owner | Tests |
|---|---|---|---|---|---|---|---|---|
| C1 | Client → Module 1 API (`/api/v1/departments, /teams, /designations, /mappings, /org-chart`) | HTTPS (Kong) → HTTP internal | This spec §4 + swagger (P2) | JWT (Module 0 Auth middleware); `organization:read/update` via RequirePermission | JSON envelope `{data,meta}` / `{error}` | 401/403/404/409 per spec §6; 503 DB down | Module 1 | API contract tests (P3) |
| C2 | Module 1 → Module 0 UserService (user existence/activity) | In-process Go call | Module 0 `UserService.GetByID/ListUsers` (merged) | Service-level (no JWT; tenant_id passed explicitly) | Go structs | User missing/inactive → 404/409, no write | Module 0 (provider), Module 1 (consumer) | Mapping service unit (mocked UserService) |
| C3 | Module 1 → Module 0 RBAC (`RequirePermission` middleware) | In-process Gin middleware | `RequirePermission(db, resource, action, ...)` (merged routes.go pattern) | JWT context roles; tenant_admin bypass preserved | Gin context | Deny → 403 | Module 0 | Middleware tests (P3) |
| C4 | Module 1 → Module 0 AuditService | In-process Go call | `AuditService.Log(...)` (merged) | Service-level | Go structs | Best-effort; audit failure never fails API | Module 0 | Service unit |
| C5 | Module 1 → PostgreSQL (org tables) | TCP/5432 GORM/pgx pool (shared) | Migrations 000013.. (P2) + §8 schema | DB user/pass (env) | SQL | Conn fail → 503 + rollback; deadlock → retry once → 503 | Module 1 | Repo integration (Docker PG) |
| C6 | Module 1 → Redis (chart cache 60s) | TCP/6379 (shared client) | Keys `org:chart:{tenant}:*`, `org:chain:{tenant}:{user}`; TTL 60s | AUTH+TLS prod | JSON | Miss/down → compute direct, log | shared/cache | Service unit (fake) |
| C7 | Identity events → Module 1 consumers | AMQP (`jaas.identity.events`) | Module 0 events.md envelope; keys `identity.tenant.created/suspended`, `identity.user.created/deactivated` | Queue creds (env) | JSON envelope | Redeliver → idempotent via event_id; poison → DLQ | Module 0 (producer), Module 1 (consumer) | Consumer tests (P3) |
| C8 | Module 1 → `jaas.organization.events` (7 produced types) | AMQP topic exchange (durable) | This spec FR-E005; envelope = Module 0 shape; version 1.0 | Queue creds (env) | JSON envelope | Publish after commit; failure → outbox + retry; never blocks API | Module 1 | Producer tests (P3) |
| C9 | Modules 2–5 → Module 1 reads (departments/teams/mappings/chart) | In-process Go calls (same deployable v1) | `DepartmentService/TeamService/MappingService/OrgChartService` read methods (P2) | Service-level + tenant_id | Go structs/DTOs | Missing → nil,nil → consumer 404 | Module 1 (provider) | Contract tests (P3) |

## Trust boundary

Everything outside the Gin handler is untrusted. JWT tid == subdomain tenant (Module 0). Every org query filters `tenant_id`. Cross-tenant IDs → 404 (never confirm existence). User references validated against Module 0 per write. No org endpoint is public.
