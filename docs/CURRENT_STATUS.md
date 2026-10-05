# JAAS — Current State: What's Done

Snapshot of the codebase. Clean architecture foundation + Module 0 Identity + Module 1 Organization + Module 2 Employee fully implemented in backend.

## Project in one line

JAAS (Just Another Admin SaaS / Company Operating System) — multi-tenant SaaS unifying HR, Assets, Projects, Meetings, Payroll, Reporting, AI, IoT. Target: replace Jira, Monday, ClickUp, GreytHR, Zoho One, Odoo, ERPNext.

- Model: one tenant per customer (`acme.jaas.com`), isolated users / roles / data.
- Roles: JAAS Super Admin (platform) vs Tenant Admin (customer) vs Members.

## Languages & stack

- Backend: **Go 1.25+ / 1.27** — Gin, GORM + pgx, golang-jwt v5, bcrypt (cost 12), go-redis, RabbitMQ amqp, Viper, zerolog, validator v10, google/uuid.
- Frontend: **TypeScript + Next.js 16.2.9 App Router + React 19.2.4**, ESLint, plain CSS. React Compiler enabled.
- Infra / specs: Docker Compose (frontend, backend, postgres, redis, rabbitmq), YAML config, DBML + Mermaid design docs, OpenAPI 3.0.3 Swagger spec.

## Architecture & principles

- Multi-tenancy: shared infra + shared PostgreSQL, isolation via mandatory `tenant_id` on every row, subdomain routing (`{slug}.jaas.com/api/v1`).
- Architecture Masterplan: Detailed end-to-end specifications for all 13 modules (Modules 0–12) and 2-person team parallelization blueprint are documented in [SYSTEM_ARCHITECTURE_MASTERPLAN.md](file:///D:/.pycache/personal/JASS/AJAAS/docs/architecture/SYSTEM_ARCHITECTURE_MASTERPLAN.md).
- Clean Architecture + DDD layers: HTTP (Routes → Middleware → Controllers) → Application (Services → DTOs → Validators) → Domain (Models → Events → Rules) → Infrastructure (Repos → DB / Cache / Queue).
- Conventions: UUID PKs, `created_at / updated_at / deleted_at` soft delete (except immutable `audit_logs` and join tables), `{data, meta}` success envelope, `{error: {code, message, details}}` errors, `page / per_page` pagination (20 default, 100 max).
- Auth: JWT access 15 min + rotating refresh 7 days with cryptographic reuse detection. Redis for sessions / rate-limit / token blacklisting. RabbitMQ for domain events (with NoOp/Outbox fallback).
- Middleware order: `CORS → RequestLogger → RateLimit → TenantResolver → Auth → RBAC → Audit`.

## What's done

### Backend Foundation & Infrastructure
- Shared packages: `config`, `database` (pool & BaseModel), `errors` (sentinel AppErrors), `logger` (zerolog), `constants`, `cache` (Redis client), `queue` (RabbitMQ + NoOp fallback), `utils` (validator).
- Bootstrap: `cmd/main.go` DI container boots identity, organization, and employee modules, automigrates GORM models, connects caches and event queues, registers routes under `/api/v1`, handles graceful shutdown.
- API Documentation: `backend/api/swagger.yaml` OpenAPI 3.0.3 spec covers all 24 Identity endpoints, 20 Organization endpoints, and 13 Employee endpoints.

### Module 0 — Identity & RBAC (Complete)
- 12 GORM models & 12 migration pairs (000001–000012).
- 12 repositories with tenant-scoping.
- 8 services: Tenant, Auth (token rotation + reuse breach revocation), User, Role, Permission, Session, Token, Audit.
- 5 controllers & 24 REST endpoints.
- Full middleware stack: Subdomain Tenant Resolver, JWT Auth, RBAC permission checker, Audit logger, CORS, sliding-window Rate Limiter.
- 12 domain events with publisher.
- 100% unit tests passing with high coverage across services and controllers.

### Module 1 — Organization Structure (Complete)
- 4 domain models (Department with acyclic tree hierarchy, Team, Designation, Mapping) + Outbox model.
- 5 migration pairs (000013–000017).
- 4 tenant-scoped repositories.
- 5 services: Department (depth ≤ 10, cycle guards), Team, Designation, Mapping (single primary check, manager cycle prevention), OrgChart (forest generator, reporting chains), Event Consumer.
- User deactivation convergence: deactivating a user automatically converges org mappings in the same transaction.
- 5 controllers & 20 REST endpoints with RBAC (`organization:read`, `organization:update`) and audit logging.
- 7 domain events published to outbox/RabbitMQ.
- Comprehensive unit test suites covering services, edge cases, error paths, branch coverage, and hierarchy rules.

### Module 2 — Employee Management & Records (Complete)
- 7 domain models (`EmployeeProfile`, `EmploymentDetail`, `EmployeeContact`, `EmployeeStatutory`, `EmployeeDocument`, `EmployeeTimeline`, `EmployeeEventsOutbox`).
- 7 migration pairs (000018–000024).
- 4 tenant-scoped repositories (Profile, Statutory, Document, Timeline).
- 5 application services:
  - `EmployeeService`: Atomic onboarding, employee code uniqueness, self-service contact update, status transitions (`active`, `probation`, `notice`, `terminated`, `resigned`).
  - `EmployeeStatutoryService`: Field-level PII masking for banking & tax IDs with strict RBAC (`employee:read_sensitive`, `employee:update_sensitive`).
  - `EmployeeDocumentService`: Document metadata attachment & admin verification workflow.
  - `EmployeeTimelineService`: Chronological career milestone audit log.
  - `EventConsumer`: Status sync on `identity.user.deactivated`.
- 4 controllers & 13 REST endpoints registered under `/api/v1/employees` with RBAC and AuditLog middleware.
- 5 domain events published to outbox/RabbitMQ.
- 100% unit test suites passing across models, validators, services, and controllers.

## What's pending

- Integration testing against live PostgreSQL/Redis/RabbitMQ containers via Docker (`tests/api/`).
- Frontend UI implementation (Identity, Organization, Employee slices).
- Module 3: Attendance & Leave Management design and implementation.
