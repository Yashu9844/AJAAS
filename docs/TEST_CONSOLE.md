# JAAS Test Console

A browser console for verifying every shipped backend feature by hand, against the live API, without waiting for the
product UI. Standard and rules: `docs/ARCHITECTURE.md` §14. It is a verification tool, not product UI, and must be
excluded from production builds.

**Status (2026-10-20, restructure Phase D):** pages exist at canonical routes for M0 and every built HR context:

| Route | Page | Operations | Scenarios |
|---|---|---|---|
| `/dev-test/identity` | M0 Identity & Access | hand-written (`specs.ts`) | `scenarios0.ts`, `scenariosAccess.ts` |
| `/dev-test/hr/organization` | HR Organization | hand-written | `scenarios1.ts` |
| `/dev-test/hr/employee` | HR Employee | hand-written | `scenarios2.ts` |
| `/dev-test/hr/attendance` | HR Attendance & Time | generated (19) | `scenariosHr.ts` `S_ATTENDANCE` |
| `/dev-test/hr/leave` | HR Leave & Absence | generated (20) | `S_LEAVE` |
| `/dev-test/hr/payroll` | HR Payroll | generated (20) | `S_PAYROLL` |
| `/dev-test/hr/recruitment` | HR Recruitment | generated (20) | `S_RECRUITMENT` |
| `/dev-test/hr/performance` | HR Performance & OKRs | generated (33) | `S_PERFORMANCE` (4) |

From now on each feature ships its page in the same phase as its backend (delivery phase P8, `docs/ENGINEERING.md` §3).
Last full run (2026-10-20, `jaas_console` DB, headless): **36/36 scenarios passed** (A-only scenarios; the two-tenant
`AB` scenarios run in the browser).

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
- HR contexts need an employee profile for the acting user (`POST /api/v1/hr/employees`; otherwise
  `EMPLOYEE_NOT_FOUND`). The HR scenarios create their own users and profiles.
- Only failed logins count against the login rate limit (10 per 15 min per IP). The M0 negative scenarios fail logins on
  purpose, so back-to-back full runs trip it: wait out the window or clear the dev keys
  (`docker exec ajaas-redis-1 sh -c "redis-cli --scan --pattern 'ratelimit:*' | xargs -r redis-cli del"`).
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
frontend/src/app/dev-test/            route pages: identity/, hr/<context>/
frontend/src/modules/devtest/
  api.ts        fetch wrapper (tenant host, token, platform key, raw exchange capture)
  auth.tsx      session panel        store.ts   console state (sessions, results)
  runner.ts     scenario runner      types.ts   CallSpec, Exchange, Session, results
  specs.ts      hand-written operation specs (M0, organization, employee)
  generated/hrSpecs.ts  operation specs generated from backend/api/swagger.yaml — never edit by hand
  scenarios*.ts scripted scenarios (scenariosHr.ts holds the HR context goldens)
  hrPages.tsx   generic HR context page (endpoints / scenarios / checklist tabs)
  components.tsx, shell.tsx (nav + overview checklists), modules.tsx (M0/organization/employee pages), devtest.css
```

To add an HR context:

1. Document its endpoints in `backend/api/swagger.yaml` under `/hr/<context>/...`.
2. Add the context to `CONTEXTS` in `scripts/gen_console_specs.py` and run `python scripts/gen_console_specs.py .`.
3. Add its scenarios to `scenariosHr.ts` (or `scenariosHr<Context>.ts`), ported from the context's golden integration
   tests: each creates its own users (`member`, `limited` helpers) and asserts status and body at every step.
4. Export a page from `hrPages.tsx`, add `app/dev-test/hr/<context>/page.tsx`, a NAV entry and an overview checklist in
   `shell.tsx`, and the scenario list in `scripts/console-headless.mjs`.

Sensors: `npx tsc --noEmit`, `npx eslint src/modules/devtest src/app/dev-test`, `npx next build`. Then run every
scenario against the live backend and record the result in the context's `testing.md`.

## 5. Headless run

The same scenario code runs without a browser (useful in CI or when no browser is available):

```bash
node scripts/console-headless.mjs <tenant-slug> <admin-email> <admin-password> [key-filter]   # e.g. filter "hr."
```

It transpiles the console's scenario modules with the project's TypeScript, routes `<slug>.localhost` to 127.0.0.1
with the right `Host` header, signs session A in as the given tenant admin and prints PASS/FAIL per scenario with the
failing steps. Exit code 1 when any scenario fails.
