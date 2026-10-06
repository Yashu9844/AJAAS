# MODULE 0–2 VERIFICATION UI REPORT

Console: `http://localhost:3000/dev-test` (temporary QA tool, **not** product UI). Code: `frontend/src/app/dev-test/**`, `frontend/src/modules/devtest/**`.
Run date 2026-10-06 against the real backend (Go, GORM AutoMigrate on Postgres 15 + Redis 7 in Docker, RabbitMQ **not** running → NoOp publisher).
No backend code was changed. All results below come from real HTTP calls made by the console.

How to run: `docker compose up -d postgres redis`; backend `DATABASE_PASSWORD=postgres DATABASE_DBNAME=jaas_identity RABBITMQ_HOST= go run cmd/main.go`; frontend `npm run dev`; create a tenant (Module 0 → Tenants), `bash scripts/dev-test-bootstrap-admin.sh <slug>`, log in (Module 0 → Authentication), run scenarios. Scenario login budget: backend allows 10 logins / 15 min per IP.

## 1. Current Module Inventory (traced Route → Controller → Service → Repository → DB)

**Module 0 — Tenant / Identity / RBAC** (22 routes, 12 tables)

| Feature | API | DB |
|---|---|---|
| Login / Refresh / Logout / Forgot / Reset | `POST /auth/login, /refresh, /logout, /forgot-password, /reset-password` | users, sessions, refresh_tokens, password_reset_tokens |
| Tenants | `POST/GET /tenants`, `GET/PATCH /tenants/:id`, `POST /:id/activate|suspend` (global, host-less) | tenants, roles (2 system roles auto-created) |
| Users | `POST/GET /users`, `GET/PATCH /users/:id`, `POST /:id/deactivate`, `POST /:id/roles` (tenant host + JWT + RBAC) | users, user_roles |
| Roles | `POST/GET /roles`, `GET/PATCH/DELETE /roles/:id`, `POST /:id/permissions` | roles, role_permissions |
| Permissions | `GET /permissions` (read only; global table) | permissions |
| Audit | written by middleware on successful mutations — **no read API** | audit_logs |
| Not exposed (code exists): invite user, MFA | – | mfa_configs, tenant_settings |

Tenant-scoped routes resolve the tenant from the **Host subdomain** (`<slug>.localhost:8080`); JWT `tid` must match.

**Module 1 — Organization** (all tenant host + JWT; `organization:read` for GET, `organization:update` for writes)
Departments (`/departments` CRUD + `/:id/deactivate?force=`; hierarchy, depth, cycle guard), Teams (`/teams`), Designations (`/designations`), Mappings (`/mappings`, user↔dept/team/designation/manager), Org chart (`GET /org-chart`, `GET /org-chart/chain?user_id=`). Tables: departments, teams, designations, mappings, org_events_outbox. Cross-module: user deactivation converges mappings (FR-M005).

**Module 2 — Employee** (tenant host + JWT; `employee:read|create|update|update_sensitive|admin`)
`/employees` (create/list/get/patch/status/deactivate), self-service `GET|PATCH /employees/me`, `GET|PUT /employees/:id/statutory`, `POST|GET /employees/:id/documents` (+ `/:doc_id/verify`; metadata only, no file upload), `GET /employees/:id/timeline`. Tables: employee_profiles, employment_details, employee_contacts, employee_statutory, employee_documents, employee_timelines, employee_events_outbox.

## 2. UI Routes Created

`/dev-test` (checklist + how-to) · `/dev-test/module-0` · `/dev-test/module-1` · `/dev-test/module-2`
Each module page: tabs per resource (form cards for every endpoint), "Scenarios (automated)", "Checklist". Persistent Auth State panel (sessions A and B, masked tokens, JWT expiry), request log with full request inspector, API host/port config.

## 3. Features Connected

Every endpoint above is wired as a generic form card (fields taken from the real DTO binding tags; raw-body editor for invalid-input tests; token-slot and host-slug override for cross-tenant tests; ID capture + "Fetch created record" read-back). Each card shows method, URL, status, time, expected vs actual, PASS/FAIL (PASS requires expected status **and** `data` envelope **and** the captured id), request headers/body and response JSON, plus the data-flow chain (only the API response is treated as evidence). Scenarios assert status **and** body at every CREATE→READ→UPDATE→READ→DEACTIVATE→READ step.

Connected: all Module 0 routes (except invite/MFA, which have none), all Module 1 routes, all Module 2 routes.

## 4. Features Not Connected / Not Testable

| Item | Why |
|---|---|
| Audit log verification | No API. Only observable in the `audit_logs` table (checklist shows ⚪ NOT TESTABLE). |
| Reset-password success path | No endpoint returns the token (email not wired). Only failure path testable. |
| Invite user / MFA | Service-layer only, no HTTP route. |
| Expired access token | Needs a 15-min wait; "invalid token" covered, "expired" not. |
| FR-EV001 (user deactivated → employee inactive via event) | Needs RabbitMQ; reported as SKIPPED. |
| Document binary upload | Backend stores metadata/URL only. |
| First tenant admin / permissions | No API: `scripts/dev-test-bootstrap-admin.sh` seeds via SQL (also seeds the 16 permission rows the routes check). |

## 5. Backend Issues Found (not patched)

1. **Logout is non-functional** — `POST /auth/logout` always 401 `"Unauthorized context mapping"`: controller requires `tenant_id` in context but route has no tenant resolver and `Authenticate` never sets it. Session is never revoked; refresh token still works afterwards.
2. **Tenant admin API is unauthenticated** — anyone can `POST/GET/PATCH /tenants`, suspend/activate any tenant (spec NFR-SEC005 requires auth).
3. **PII masking bypass** — `GET /employees/:id/statutory?unmasked=true` returns raw tax ID / national ID / bank account to any caller with only `employee:read`.
4. **Statutory upsert broken on the runnable schema** — `PUT /employees/:id/statutory` → 500: repo uses `ON CONFLICT (tenant_id, employee_profile_id)` but GORM AutoMigrate creates the unique index on `employee_profile_id` only (SQL migration 000021 has the composite index; main.go uses AutoMigrate, so migrations and runtime schema diverge). GET then 404.
5. **Duplicate code → 500** for departments, teams, designations (unique violation not mapped to 409).
6. `POST /roles` with an unknown permission id → **500 and the role row is still created** (no rollback).
7. `UserResponse.roles` is always `null` (create/get/list) although `user_roles` rows are persisted; `RoleResponse.permissions` never populated → no API reveals a user's/role's permissions.
8. Department deactivate counts **inactive** teams → a department whose teams are all inactive cannot be deactivated without `force`.
9. `GET /org-chart` cached 60 s per (tenant, max_depth, include_inactive) with **no invalidation** → stale reads after writes (verified: created department missing from a warm key).
10. `PATCH /employees/me` with `emergency_contacts` as a plain string → 500 (DTO says string, column is JSONB; only valid JSON works).
11. Employee status: no transition matrix (FR-ED002; `resigned → active` accepted) and exit finalisation does **not** deactivate the Module 0 user (FR-ED005).
12. `POST /mappings/:id/deactivate` without a JSON body → 400 `EOF` (other deactivate endpoints accept no body).
13. No first-user/permission seed, no create-permission, activate-user, unassign-role/permission APIs.

## 6. API Contract Mismatches

| Frontend / spec expected | Backend actually returns |
|---|---|
| Error envelope `{error:{code,message,details}}` | Bare string `{"error":"Key: 'X.Y' Error:…"}` for many validation failures (login, users, tenants, refresh, `Invalid user ID`) and logout 401 |
| `user.roles` populated | `null` |
| `role.permissions` populated after create/assign | omitted |
| Department `path` includes self | ancestors only |
| `max_depth=1` etc. consistent with latest data | stale cache |
| Employee `emergency_contacts` string | must be valid JSON text |
| List meta `total_items/total_pages` (tenants/users/org) | employees returns `{page, per_page, total}` (different shape) |

## 7. Authentication Issues
Logout broken (#1); refresh rotation and **refresh-reuse detection work** (`TOKEN_REUSED`, all sessions invalidated); wrong password/unknown tenant both → 401 (no enumeration); deactivated user → 403 `ACCOUNT_NOT_ACTIVE`; login rate limit works but counts failed attempts too and is keyed by IP only (`ratelimit:/api/v1/auth/login:::1`).

## 8. RBAC Issues
Enforcement itself is correct in all three modules (read-only vs write, per-permission, grant-then-allow, `tenant_admin` bypass, sensitive/admin tiers). Defects: PII bypass via `?unmasked=true` (#3), unauthenticated tenant routes (#2), and permissions not observable via API (#7).

## 9. Data Flow Issues
Read-back verified for tenants, users, roles, departments, teams, designations, mappings, employees, documents. Cross-module convergence works for user-deactivation → mappings (FR-M005); fails for employee exit → user (#11). Partial write on role creation (#6). Stale org chart (#9).

## 10. UI (console) Issues / limits
Static "expires in Ns" label does not tick; scenarios create throwaway data (names prefixed `DT`/`dt-`) and some cleanup steps are best-effort; scenarios consume login budget (≈10 logins per full Module 0 run → wait 15 min or flush Redis between full runs); results persist in browser localStorage (tokens in sessionStorage); pane tested at narrow width only.

## 11. Tests Performed (exact results, last runs)

Static: `npx tsc --noEmit` clean, `npx eslint src/modules/devtest src/app/dev-test` clean, `npx next build` OK (4 new static routes).

| Scenario | Steps pass / fail / skip | Failing steps (all real backend behaviour) |
|---|---|---|
| M0 Auth & error negatives | 15 / 3 / 0 | unknown permission id → 500; GET /tenants and POST /tenants without token → 200/201 |
| M0 Tenant lifecycle | 12 / 0 / 0 | – |
| M0 User lifecycle | 12 / 1 / 0 | `roles` missing in GET |
| M0 Roles & permissions | 12 / 2 / 0 | permissions not returned (create / after assign) |
| M0 RBAC enforcement | 15 / 0 / 0 | – |
| M0 Session lifecycle | 11 / 3 / 0 | logout 401; token still valid after logout; refresh works after logout |
| M0 Tenant isolation (A↔B) | 15 / 0 / 0 | – (cross-host 403, cross-id 404, no list leaks) |
| M1 Departments | 16 / 2 / 0 | duplicate code 500; deactivate with inactive team 409 |
| M1 Teams | 15 / 2 / 0 | duplicate code 500; deactivate dept w/ inactive team 409 |
| M1 Designations | 10 / 1 / 0 | duplicate code 500 |
| M1 Mappings (+FR-M005) | 28 / 1 / 0 | deactivate without body → 400 EOF |
| M1 Org chart + chain | 17 / 1 / 0 | stale cache (read-your-writes) |
| M1 RBAC | 15 / 0 / 0 | – |
| M1 Validation/errors | 16 / 0 / 0 | – |
| M2 Employee CRUD/validation | 23 / 0 / 0 | – |
| M2 Status/timeline | 16 / 2 / 1 | exit doesn't deactivate user; illegal transition accepted; FR-EV001 skipped (no RabbitMQ) |
| M2 Statutory | **as found: 6 / 5 / 0** (PUT 500, GET 404) — **after adding the composite unique index (see note): 11 / 0 / 0** | |
| M2 Documents | 12 / 0 / 0 | – |
| M2 Self-service | 16 / 1 / 0 (as found 13 / 2) | emergency_contacts plain string → 500 |
| M2 RBAC | 33 / 1 / 0 (as found 30 / 4) | PII leak via `?unmasked=true` |
| M2 Validation/errors | 12 / 0 / 0 | – |

Manual form-card checks: Login (200 + tokens), Refresh (200), Logout (401 FAIL, as #1), Forgot password (200), List permissions (200, id captured), Create tenant (201) → Fetch created record (200, same id).

**Environment note (disclosure):** to see past defect #4, I replaced the dev DB index `idx_emp_statutory_tenant_profile` with the composite `(tenant_id, employee_profile_id)` index that SQL migration 000021 specifies. That is a schema change to the local Docker Postgres (`jaas_identity`); to restore as-found behaviour: `DROP INDEX idx_emp_statutory_tenant_profile; CREATE UNIQUE INDEX idx_emp_statutory_tenant_profile ON employee_statutory(employee_profile_id);`. Local test data (tenants `probeco`, `probeco2`, `uismoke…`, many `DT`/`dt-` rows) remains in that DB.

## 12. Recommended Fixes

**CRITICAL**
- Authenticate and authorise `/api/v1/tenants*` (super-admin only).
- Remove/gate `?unmasked=true` — require `employee:read_sensitive` server-side.
- Fix logout (set `tenant_id` from claims in `Authenticate`, or mount tenant resolver) so sessions are really revoked.

**HIGH**
- Make runtime schema = migrations (use the SQL migrations, or fix the composite unique index) so statutory upsert works; add a test that boots from the real schema.
- Map unique-constraint violations to 409 (dept/team/designation codes); validate permission ids and wrap role creation in a transaction (no 500, no partial writes).
- Return `roles` on users and `permissions` on roles.
- Implement FR-ED002 transition matrix and FR-ED005 exit → user deactivation.

**MEDIUM**
- Invalidate/shorten the org-chart cache on writes.
- Count only active teams in department deactivation.
- Normalise validation errors to the standard envelope; unify list `meta` shape.
- Accept bodiless `POST /mappings/:id/deactivate`; make `emergency_contacts` a JSON field or wrap plain text.
- Seed permissions; add bootstrap path for the first tenant admin; add audit-log read API.

**LOW**
- Rate limit by IP+email and don't count successful logins; expose audit/permission introspection (`GET /me/permissions`); document the host-subdomain requirement for non-browser clients.
