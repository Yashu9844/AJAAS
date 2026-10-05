# JAAS Architecture

## System overview

Multi-tenant SaaS. One tenant per customer (`{slug}.jaas.com`). Shared infrastructure + shared PostgreSQL, isolation via `tenant_id` on every row, subdomain routing. JAAS Super Admin owns the platform; Tenant Admin owns their tenant.

```text
Client → Kong (TLS + routing) → Go/Gin API → Middleware [CORS → RateLimit → TenantResolver → Auth → RBAC → Audit]
→ Controllers → Services → Repositories → PostgreSQL (truth) / Redis (sessions/rate/blacklist/cache) / RabbitMQ (events) / SMTP via events (Module 7)
```

Frontend: Next.js App Router consumes the REST contracts only — never backend internals. Auth: JWT 15m + rotating opaque refresh 7d; reset tokens 1h single-use; sessions 24h; permission cache 5m.

## Module graph

```text
              Module 0 Identity (root, zero internal deps)
                         |
              Module 1 Organization (Tenant, User, RBAC)
                         |
              Module 2 Employee (Org + Identity)
               /  |  \  \  \  \
              /   |   \  \  \  \
        Mod 3  Mod 4 Mod 5 Mod 6 Mod 8  Mod 9, Mod 12
        (Att) (Proj)(Meet)(Appr)(Assets)(Payroll)(IoT)
           \   |   /  /  /  /
            \  |  /  /  /  /
        Module 7 Notifications (events → email)
                         |
          Module 10 Analytics (all) → Module 11 AI (all business)
```

Module homes: backend `backend/internal/{identity,organization,employee,<future>}/` + `backend/internal/shared/`; frontend `frontend/src/modules/{identity,organization,employee,<future>}/`; specs + operational bundles `docs/modules/module-N-name/` (17 files each per MASTER_PROMPT §3.2).

## Request lifecycle (Module 0 example)

```text
Request → CORS → RateLimit (Redis sliding window) → TenantResolver (Host → slug → tenant_id; 404/403)
→ Auth (JWT verify: signature, exp, iss, JTI blacklist, user+tenant active, tid == subdomain)
→ RBAC (DB permission check + 5m Redis cache; 403 if missing)
→ Controller (parse → validate → service → map → status) → Service (tx, rules, events-after-commit, audit)
→ Repository (tenant_id-scoped GORM) → PostgreSQL
```

## Conventions

Clean Architecture + DDD layers; UUID PKs; `created_at/updated_at/deleted_at` soft delete (tenants/users/roles only; audit_logs immutable, no update/delete); envelopes `{data,meta}` / `{error:{code,message,details}}`; `page/per_page` (1/20/100); constructor DI, interfaces, no globals, no init() wiring. Full standard: `docs/SECURITY.md` (law) + `docs/modules/module-0-identity/coding-guidelines.md`.
