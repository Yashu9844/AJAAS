# JAAS — Current State: What's Done

Snapshot of the codebase as it stands today. Design-heavy, implementation-early.

## Project in one line

JAAS (Just Another Admin SaaS / Company Operating System) — multi-tenant SaaS unifying HR, Assets, Projects, Meetings, Payroll, Reporting, AI, IoT. Target: replace Jira, Monday, ClickUp, GreytHR, Zoho One, Odoo, ERPNext.

- Model: one tenant per customer (`acme.jaas.com`), isolated users / roles / data.
- Roles: JAAS Super Admin (platform) vs Tenant Admin (customer).

## Languages & stack

- Backend: **Go 1.25** (go.mod) — Gin, GORM + pgx, golang-jwt v5, bcrypt (cost 12), go-redis, RabbitMQ amqp, Viper, zerolog, validator v10, google/uuid.
  - Note: `backend/Dockerfile` still says `golang:1.22-alpine` — mismatch with go.mod.
- Frontend: **TypeScript + Next.js 16.2.9 App Router + React 19.2.4**, ESLint, plain CSS. React Compiler enabled.
- Infra / specs: Docker Compose, YAML config, DBML + Mermaid for design, Markdown docs. No migrations runner, no gateway, no CI wired yet.

## Architecture & principles (as designed)

- Multi-tenancy: shared infra + shared PostgreSQL, isolation via `tenant_id` on every row, subdomain routing (`{slug}.jaas.com/api/v1`).
- Clean Architecture + DDD layers: HTTP (Routes → Middleware → Controllers) → Application (Services → DTOs → Validators) → Domain (Models → Events → Rules) → Infrastructure (Repos → DB / Cache / Queue).
- Conventions: UUID PKs, `created_at / updated_at / deleted_at` soft delete (except immutable `audit_logs`), `{data, meta}` success envelope, `{error: {code, message, details}}` errors, `page / per_page` pagination (20 default, 100 max).
- Auth: JWT access 15 min + rotating refresh 7 days. Redis for sessions / rate-limit / blacklist. RabbitMQ for domain events. Middleware order: `CORS → RateLimit → TenantResolver → Auth → RBAC → Audit`.
- Module 0 (Tenant + Identity + RBAC) is the root dependency — zero internal deps, everything else (Modules 1–12) builds on it.

## What's done

- Product discovery: problem, requirements, 10 personas (CEO → Super Admin), journeys, IA — marked complete in `context01.md`.
- Module 0 spec (complete on paper): 16 files in `docs/modules/module-0-identity/` — README, system-architecture, api-contracts, database-schema.dbml, sequence-diagrams, events, security, business-rules, testing-strategy, roadmap, checklist, implementation-plan, etc.
- Backend shared foundation (`internal/shared/`, with tests):
  - `config/config.go` — Viper YAML + env loader (server, database, redis, rabbitmq, jwt).
  - `database/database.go` — GORM Postgres + pool (25 open / 10 idle / 5m lifetime).
  - `database/base.go` — `BaseModel` (UUID + timestamps + soft delete + BeforeCreate hook), `TenantBaseModel` (+ `tenant_id`).
  - `errors/errors.go` — `AppError` + sentinels (400/401/403/404/409/429/500).
  - `logger/logger.go` — zerolog, console in dev / JSON in prod.
  - `constants/constants.go` — statuses, token TTLs, pagination, `tenant_admin` / `member` roles.
  - Tests: `config_test.go`, `errors_test.go`, `base_test.go`.
- Config & dev scaffolding: `configs/development.yaml` + `production.yaml`, `Makefile` (`dev-frontend`, `dev-backend`, `docker-up/down`), `docker-compose.yml` (frontend:3000, backend:8080), backend + frontend Dockerfiles, `.env.example`.
- Frontend bootstrap only: `src/app/layout.tsx` + `page.tsx` ("Bootstrap foundation ready"). Directory skeleton (`app/components/hooks/lib/services/stores/types/utils`, `modules/identity,organization,employee`) is all `.gitkeep`.

## What's partially done / broken

- `internal/identity/models/tenant.go` exists but references `User, Role, Session, AuditLog, TenantSettings` which don't exist yet — won't compile. Only 1 of ~12 models.
- `Makefile dev-backend` points at `cmd/main.go` — file doesn't exist (`cmd/` is just `.gitkeep`). Same for `api/`, `migrations/`, `internal/organization`, `internal/employee` (all empty).
- `docker-compose.yml` has frontend + backend only — Postgres, Redis, RabbitMQ, Kong, SMTP from the spec are missing.
- `production.yaml` has empty `database.password` and `jwt.secret` (must inject via env). `.env.example` only has `PORT` + `DATABASE_URL`.
- No TODOs in code — but that's because there's almost no feature code yet; the pending work lives in `implementation-checklist.md` / `implementation-roadmap.md`.

## What's pending (the real work)

- Immediate next per `context01.md`: Module 0 ER Diagram review → then Module 1 Organization Architecture (Departments, Teams, Designations, Reporting hierarchy, Org chart).
- Backend roadmap Phases 2–10 (~30h est.): 11 models, 12 migrations, DTOs/validation, events/queue, repositories, 8 services (Auth/Tenant/User/Role/Permission/Session/Token/Audit), controllers/routes, middleware stack, bootstrap + Swagger.
- Frontend: auth pages, org/employee pages, state, API clients — all unwritten.
- Modules 1–12: Organization, Employee, Attendance/Leave, Projects, Meetings, Approvals, Notifications, Assets, Payroll, Analytics, AI, IoT — design + code pending.

## Open questions for discussion

- Confirm `tenant.go` won't build — finish models + migrations next, or ER diagram first?
- Fix `golang:1.22` Dockerfile vs go 1.25, add missing `cmd/main.go`, infra services in compose?
- Frontend: which module (identity/auth) gets built first against the future API?
