# JAAS Test Console

A browser console for verifying every shipped backend feature by hand, against the live API, without waiting for the
product UI. Standard and rules: `docs/ARCHITECTURE.md` §14. It is a verification tool, not product UI, and must be
excluded from production builds.

**Status (2026-10-20):** pages exist for M0 and the HR `organization` and `employee` contexts
(`frontend/src/app/dev-test/module-{0,1,2}`, code in `frontend/src/modules/devtest/`, about 3,000 lines). Restructure
Phase D renames the pages to canonical names and adds every other built HR context; from then on each feature ships
its page in the same phase as its backend (delivery phase P8, `docs/ENGINEERING.md` §3).

## 1. Run it

```bash
# 1. infrastructure
docker compose up -d postgres redis rabbitmq        # or: docker start ajaas-postgres-1 ajaas-redis-1 ajaas-rabbitmq-1
docker exec ajaas-postgres-1 psql -U postgres -c "CREATE DATABASE jaas_console"   # fresh database recommended

# 2. backend (applies SQL migrations and seeds the permission catalogue on boot)
cd backend
APP_ENV=development DATABASE_PASSWORD=postgres DATABASE_DBNAME=jaas_console go run ./cmd
curl localhost:8080/health                           # {"status":"UP"}

# 3. console
cd frontend && npm install && npm run dev            # http://localhost:3000/dev-test
```

A database created by an old AutoMigrate build has no `schema_migrations` table and will not migrate cleanly; use a
fresh one.

## 2. Tenants, users and permissions

- The tenant comes from the **Host subdomain only**, so calls go to `http://<slug>.localhost:8080/api/v1/...`
  (`*.localhost` resolves to 127.0.0.1 in Chrome and Firefox). JWT as `Authorization: Bearer <token>`.
- Create a tenant and its first admin (the console's tenant panel does this):

  ```bash
  curl -X POST localhost:8080/api/v1/tenants -H "X-Platform-Key: dev-platform-admin-key-change-me" \
    -H 'Content-Type: application/json' \
    -d '{"name":"Demo","slug":"demo","admin":{"email":"admin@demo.com","password":"Passw0rd!123","first_name":"Admin","last_name":"User"}}'
  ```

  The dev platform key is in `backend/configs/development.yaml` (override with `PLATFORM_ADMIN_KEY`). The admin holds
  `tenant_admin`, which bypasses permission checks.
- For exact-grant RBAC checks create a role holding exactly one permission (`POST /api/v1/roles`
  `{name, permission_ids}`; ids from `GET /api/v1/permissions`) and a user with that role. A plain member with no
  roles is the negative case.
- HR contexts need an employee profile for the acting user (`POST /api/v1/employees`; otherwise
  `EMPLOYEE_NOT_FOUND`).
- Only failed logins count against the login rate limit; if a run trips it, wait out the window or flush the dev Redis.
- CORS: in development with `CORS_ALLOWED_ORIGINS` unset every origin is allowed; set it to
  `http://localhost:3000` to test the real allow-list. Do not use `credentials: 'include'`.

## 3. What a page must have

| Part | Content |
|---|---|
| Session panel | tenant slug, login, refresh, logout, current user, roles and resolved permissions; two sessions (A and B) for cross-tenant and RBAC checks |
| Operations | every endpoint of the module/context grouped by resource, each with a form (path params, query, JSON body), Send, and the raw request, status, timing and response body |
| Scenarios | each golden test and the context's manual checklist (in its `testing.md`) as a scripted flow that asserts status **and** body at every step, creates its own throwaway data, and shows pass/fail per step |
| Inspectors | audit log for the tenant; outbox/events for the context once the shared outbox is in place (Phase C) |
| Checklist | the context's acceptance items with stored pass/fail results |

Response conventions to display verbatim: success `{"data": ...}` (lists add `meta {page, per_page, total_items,
total_pages}`); errors `{"error": {"code", "message", "details"}}`; foreign-tenant id → 404; foreign JWT → 403.

## 4. Code layout and how to add a page

```text
frontend/src/app/dev-test/            route pages (one per module/context)
frontend/src/modules/devtest/
  api.ts        fetch wrapper (tenant host, token, platform key, raw exchange capture)
  auth.tsx      session panel        store.ts   console state (sessions, results)
  runner.ts     scenario runner      types.ts   CallSpec, Exchange, Session, results
  specs.ts      endpoint specs (operation forms) per resource
  scenarios*.ts scripted scenarios per module/context
  components.tsx, shell.tsx, modules.tsx, devtest.css
```

To add a context: add its endpoint specs to `specs.ts` (or a new `specs-<context>.ts`), its scenarios to
`scenarios-<context>.ts`, a section in `modules.tsx`, and a route page under `app/dev-test/<context>/`. Sensors:
`npx tsc --noEmit`, `npm run lint`, `npm run build`. Then run every scenario against the live backend and record the
result in the context's `testing.md`.
