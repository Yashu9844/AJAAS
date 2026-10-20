# JAAS — System Reference (M0 Identity & Access + M1 HR Management)

> Snapshot: 2026-10-18, branch `module_7_performance` (3 commits ahead of `main` @ `fe620fa`).
> **Part 1** (this part) is the hand-written explanation: architecture, how modules depend on each other, the logic of
> every module, and end-to-end flows.
> **Part 2** (the appendix) is generated from the source and is exhaustive:
> - every route with its full middleware/permission chain;
> - every table with every column and constraint;
> - every Go file with every type, function, method and interface method (signature + doc comment);
> - every test function;
> - every module's ID-tagged requirements/rules and its full decision log.
>
> Regenerate the appendix with `python scripts/gen_reference.py <repo> <out>` instead of hand-editing it.

> **Read this first (2026-10-20):** Part 2 below is regenerated from the current code (HR under
> `backend/internal/hr/<context>`, routes under `/api/v1/hr`, migrations per schema in `backend/migrations/identity`
> and `backend/migrations/hr`). Part 1 is the narrative written before restructure Phases B–C: its module logic is
> still accurate, but for paths, schemas, the access model, idempotency, outbox, invitations and EdDSA tokens see
> `docs/ARCHITECTURE.md` §12–17 and the module plans (`docs/modules/m0-identity-access/plan.md`,
> `docs/modules/m1-hr-management/plan.md`).

---

## 1. Status at a glance

| Module | Scope | Ops | Tables (migrations) | Events exchange | State |
|---|---|---|---|---|---|
| 0 Identity & RBAC | tenants, users, roles, permissions, sessions, JWT, audit | 29 | 13 (000001–000012, 000025) | identity events | on `main`; hardened by partner |
| 1 Organization | departments, teams, designations, user mappings + manager, org chart | 22 | 5 (000013–000017) | `org_events_outbox` | on `main` |
| 2 Employee | profiles, employment, contacts, statutory (masked), documents, timeline | 14 | 7 (000018–000024) | `employee_events_outbox` | on `main` |
| 3 Attendance | shifts, assignments, punches, daily records, regularization, summary | 19 | 6 (000026–000031) | `jaas.attendance.events` | on `main`, backend closed |
| 4 Leave | types, holidays, ledger-backed balances, requests, approvals | 20 | 6 (000032–000037) | `jaas.leave.events` | on `main`, backend closed |
| 5 Payroll | structures, CTC assignments, runs (maker-checker), statutory, payslips, CSV | 20 | 7 (000038–000044) | `jaas.payroll.events` | on `main`, backend closed |
| 6 Recruitment | jobs, candidates + stage history, interviews, offers, hire saga | 20 | 6 (000045–000050) | `jaas.recruitment.events` | on `main`, backend closed |
| 7 Performance & OKRs | cycles, goals + key results + check-ins, reviews, 9-box, calibration, continuous feedback | 33 | 9 (hr 000038–000046) | `jaas.performance.events` | backend closed 2026-10-20 (branch `restructure_architecture`) |

**Totals on `main`:**
- 144 routes + `/health`;
- 50 migrations (56 on the HR/Performance branch);
- integration suite 63/63; all unit packages green.

**Totals on `restructure_architecture` (2026-10-20):** 192 module routes (Part 2) + health/JWKS; migrations re-baselined
into `migrations/identity` (18) and `migrations/hr` (46); full unit and integration suites green, including
`TestSwaggerMatchesRoutes` (contract ↔ router) and `TestImportBoundaries`; test console 36/36 scenarios. Migration numbers
in the table above are the pre-split ones.

**Who built what:**
- M0 Identity, HR/Organization, HR/Employee existed before this work and were hardened by the partner (`fix/module-0-2-hardening`, now in `main`).
- HR/Attendance, HR/Leave, HR/Payroll, HR/Recruitment, HR/Performance, the integration onto the hardened base and the architecture restructure were built in this workstream.

**Module numbering** follows `docs/ARCHITECTURE.md`: M0 Identity & Access plus product modules M1–M12; the HR rows above are bounded contexts of M1 HR Management (`docs/modules/m1-hr-management/`). Meetings, Approvals and Notifications are M3, M7 and M10 (not started).

**Working-tree state on this branch:**
- Two untracked `.rar` files under `docs/modules/` are not from this workstream.

---

## 2. Global architecture

### 2.1 Stack
- **Backend:**
  - Go 1.27, Gin, GORM (pgx driver), PostgreSQL;
  - Redis (go-redis): rate limits, org-chart cache, sessions;
  - RabbitMQ (`amqp`, with a no-op fallback);
  - Viper config, zerolog, validator v10, golang-jwt v5, bcrypt.
- **Infra:** `docker-compose.yml` runs Postgres, Redis and RabbitMQ (`ajaas-postgres-1`, `ajaas-redis-1`, `ajaas-rabbitmq-1`).
- **Frontend:** Next.js 16 / React 19. Only the test console exists (`src/app/dev-test/identity` and `src/app/dev-test/hr/<context>` for organization, employee, attendance, leave, payroll and recruitment; `docs/TEST_CONSOLE.md`); all other module folders are `.gitkeep` placeholders. Product UIs are blocked on the identity shell.

### 2.2 Layering (identical in every module)
```
HTTP route table (routes/routes.go)
  → group middleware: TenantResolver → Authenticate
  → per-route: RequirePermission(resource, action) | PermissionFlag | rate limit | AuditLog
  → controller (parse path/query/body, validate tags, build Actor, call service, write envelope)
  → service (business rules, transactions, locks, cross-module ports, outbox, audit)
  → pure engine (calc / pipeline / score / lifecycle — no DB, 100% tested)
  → repository (GORM, tenant_id on every query; db/tx passed per call)
  → PostgreSQL;  outbox → RabbitMQ (after commit + background relay)
```
Package layout per module `backend/internal/<module>/`:

| Folder | Purpose |
|---|---|
| `models` | DB rows; `TableName`; UUID assignment in `BeforeCreate` |
| `dto` | Request/response JSON; never GORM models |
| `validators` | Rules tags can't express |
| `repositories` | Interfaces + implementations |
| `services` | Business logic + `ports.go` (what the module needs from others) |
| `controllers` | HTTP edge |
| `routes` | Endpoint table + permission per route |
| `events` | Envelope + payloads |
| `module.go` | DI container; `NewModule`, `RegisterModels`, `SeedPermissions`, `RegisterRoutes`, `Relay`, cross-module accessors |

### 2.3 Boot sequence
`cmd/main.go`:
1. load config;
2. connect Postgres, Redis and RabbitMQ (falls back to `NoOpPublisher`);
3. create `relayCtx`;
4. call `app.Build(Options{…, Background: relayCtx})`;
5. serve;
6. on SIGTERM: graceful shutdown, then `stopRelays()`.

`internal/app/app.go` `Build`:
1. `ValidateConfig`: in production the JWT secret must be ≥ 32 chars; TTLs must be > 0.
2. `identity.NewModule(...)`, `SetPlatformAdminKey`. An empty key means `/tenants` is disabled (fail closed).
3. Schema:
   - `database.RunMigrations(ctx, db, migrations.FS)` applies the embedded `backend/migrations/*.up.sql` in order and records them in `schema_migrations`. SQL is the schema authority.
   - Only when `database.auto_migrate=true` (legacy/dev) does GORM AutoMigrate run instead.
4. `identityModule.SeedPermissions()`: the base permission catalogue.
5. `organization.NewModule(...)`; `SetUserDeactivationConverger(org)`.
6. `employee.NewModule(...)`; `AddUserDeactivationConverger(employee)`.
   - Deactivating a user closes their org mappings and converges their employee profile **in the same transaction**.
7. `buildPeopleModules(o, coreModules{identity, org, employee})` in `internal/app/time_modules.go` wires the people modules:
   - attendance ← (employee, audit, publisher);
   - leave ← (employee, audit, `attendance.LeaveSync()`, publisher);
   - payroll ← (employee, `leave.UnpaidLeave()`, audit, publisher);
   - recruitment ← (employee, `identity.UserService()` for invites, `org.DepartmentService()` / `org.DesignationService()`, audit, publisher).
8. `peopleModules.prepare`:
   - AutoMigrate (legacy only);
   - each module's `SeedPermissions`;
   - `go module.Relay().Run(Background)` for each module.
9. Router:
   - `gin.Recovery`;
   - `SetTrustedProxies(nil)` by default, so `X-Forwarded-For` can't spoof the rate limiter;
   - `SecurityHeaders`, `BodyLimit`, `RequestLogger`, `CORS(allowList)`.
10. `/api/v1` routes for every module; `GET /health`.

The in-process integration tests call the same `Build`, so the tested router is the production router.

### 2.4 Multi-tenancy and security model
- **Tenant comes from the Host subdomain only.**
  - `identity/middleware/tenant.go` `TenantResolver` strips the port, takes the first label (`acme.localhost`, `acme.jaas.com`), loads the tenant by slug, rejects suspended tenants, and sets `tenant_id` in the context.
- **`Authenticate`** (`middleware/auth.go`):
  - validates the JWT;
  - validates the session (not revoked);
  - requires JWT `tid` == resolved tenant, so a foreign token on another host gets **403**;
  - sets `user_id`, `roles`, `session_id`.
- **RBAC** (`middleware/rbac.go` `evaluatePermission`):
  - `tenant_admin` role bypasses checks;
  - otherwise user → roles → role_permissions → permission `resource:action`.
  - `RequirePermission` blocks (403).
  - `PermissionFlag` never blocks; it stores a bool in the context. Used for statutory unmasking (HR/Employee) and owner-or-manager checks (HR/Performance).
- **Platform API:** `PlatformAdmin` guards `/tenants` with the `X-Platform-Key` header.
- **Isolation:** every table has `tenant_id`; every repository filters on it. A foreign id returns **404** (not 403), so existence never leaks.
- **Anti-escalation** (`identity/services/grants.go`): a non-admin cannot grant permissions, or assign roles carrying permissions, they don't hold themselves, and can never grant `tenant_admin`.
- **Hardening:**
  - `SecurityHeaders`;
  - `BodyLimit`;
  - CORS allow-list (only `*` in development);
  - `FailureRateLimiter` on login: only failed attempts count (10 / 15 min, keyed on route + client IP), so successful logins never lock users out;
  - `RateLimiter` on forgot-password (5 / hour);
  - punch rate limit in HR/Attendance.
- **Login timing:** an unknown email still runs a bcrypt compare against a dummy hash, so response time doesn't reveal whether an account exists.

### 2.5 Conventions
- Success `{"data": …}`; lists `{"data": […], "meta": {page, per_page, total_items, total_pages}}`.
- Errors `{"error": {"code", "message", "details"?}}`. `shared/errors.Normalize` maps unique violation → 409 CONFLICT, FK violation → 400, not found → 404. Anything unknown becomes an **opaque 500**.
- Pagination: `page ≥ 1`, `per_page 1–100`, default 20; garbage is clamped and never causes a 500.
- UUID primary keys everywhere; timestamps UTC; calendar dates `YYYY-MM-DD`.

### 2.6 Shared packages (`backend/internal/shared`)

| Package | Contents |
|---|---|
| `config` | `Config{Server, Database, Redis, RabbitMQ, JWT, Platform}`, `LoadConfig` (YAML in `configs/` + env overrides such as `DATABASE_PASSWORD`, `DATABASE_DBNAME`, `JWT_SECRET`, `PLATFORM_ADMIN_KEY`, `CORS_ALLOWED_ORIGINS`) |
| `database` | `NewDatabase` (pool), `BaseModel` / `TenantBaseModel`, `RunMigrations`, `BeginTx` / `Finish` |
| `errors` | `AppError{Code, Message, StatusCode}`, sentinels, `ValidationErrorDetail`, `Normalize` |
| `cache` | `RedisClient`: Get/Set/Delete/Exists/DeleteByPrefix/IncrWithExpire/Decr/TTL/Close |
| `queue` | `EventPublisher` interface; `RabbitMQPublisher` (auto-reconnect); `NoOpPublisher` |
| `middleware` | `CORS`, `SecurityHeaders`, `BodyLimit`, `RateLimiter`, `FailureRateLimiter`, `RequestLogger` |
| `utils` | `NormalizePagination`, `ParsePagination`, `ValidateStruct`, `BindErrorDetails` |
| `logger` | zerolog wrapper `NewLogger(env)` |

### 2.7 Engineering patterns used in HR/Attendance, HR/Leave, HR/Payroll, HR/Recruitment, HR/Performance (and why)

| Pattern | Where | Why |
|---|---|---|
| **Ports & adapters** | `services/ports.go` in each module: `EmployeeDirectory`, `AuditLogger`, `TxRunner`, `Clock`, `LeaveSync`, `LeaveLOP`, `UserInviter`, `OrgDirectory`… | Modules depend on tiny interfaces, not each other's internals. `internal/app` injects the real services; unit tests inject fakes. |
| **Actor from context only** | `controllers/respond.go` `actorFrom` | Identity (tenant, user, correlation id, IP, UA) comes only from middleware-set context; a body can never claim to be someone else. |
| **Transactional outbox + relay** | `services/helpers.go` `writeOutbox`; `services/outbox_relay.go` | Events are written in the same DB transaction as the change (no lost or phantom events). They are published right after commit and marked published. `OutboxRelay.Run` retries every 30 s, batch 100, max 10 attempts, recording `attempts` / `last_error`. |
| **Audit after commit** | `afterCommit` | Best-effort `AuditService.Log`; metadata carries ids/statuses only (no PII, salaries, ratings). |
| **Exact decimals** | `payroll/calc.Money` (paise), `leave/calc.Days` and `performance/score.Fixed` (hundredths) | No floating point in stored or compared values; DB `NUMERIC`; exact `Value`/`Scan`/JSON codecs; half-away-from-zero rounding. |
| **Concurrency** | `LockEmployee` (transaction advisory lock per employee) in attendance/leave/payroll; `FOR UPDATE` on payroll runs, recruitment jobs/candidates; per-(cycle, owner) lock in HR/Performance | Balances, punches, weight caps and headcount can't race. |
| **Pure engines** | `attendance/calc`, `leave/calc`, `payroll/calc`, `recruitment/pipeline`, `performance/score` + `lifecycle` | All math and state machines are DB-free and fully tested with golden tables. |
| **Append-only history** | punches, leave ledger, payroll assignments, recruitment stage events, performance check-ins | Corrections supersede or reverse; nothing is rewritten. |
| **SQL authority + parity** | `backend/migrations` + per-module model tests | Each module's migrations pass up/down/up, and GORM AutoMigrate of the models produces the same columns and indexes as the SQL (checked with a temp DB diff). |
| **Testing style** | `services/*_test.go` | In-memory fakes with snapshot rollback imitate transactions; failure injection makes every repository call fail in turn; goldens named `TestGolden_G<n>_…`; exact-grant RBAC tests in integration. |

---

## 3. Module dependency map

```
            ┌───────────────── M0 Identity (tenants, auth, RBAC, audit, InviteUser) ─────────────────┐
            │ used by every module (middleware + AuditService); HR/Recruitment also calls UserService.InviteUser │
            └───────────────────────────────────────────────────────────────────────────────────────────────┘
HR/Organization ── DepartmentService/DesignationService ──▶ HR/Recruitment (job references)
                      └─ MappingService (manager_user_id) ──────▶ HR/Performance (manager access, manager reviews)
HR/Employee ── EmployeeService.GetByID/GetByUserID/List ──▶ HR/Attendance, HR/Leave, HR/Payroll, HR/Recruitment, HR/Performance
                  └─ CreateEmployee ─────────────────────────────▶ HR/Recruitment (hire)
HR/Attendance ── LeaveSync.MarkLeave/ClearLeave ◀────────── HR/Leave (approved leave → on_leave days)
HR/Leave ── UnpaidLeave.UnpaidDays ───────────────────────▶ HR/Payroll (LOP days)
User deactivation (M0 Identity) ──▶ converges HR/Organization mappings + HR/Employee profile in the same transaction
```
Cross-module edits made for these links (each recorded in the owning module's decisions):
- `attendance.Module.LeaveSync()` (D3-17)
- `leave.Module.UnpaidLeave()` (D4-13)
- `organization.Module.DepartmentService()` / `DesignationService()` (D6-03)

### 3.1 Permission catalogue (seeded at boot)

| Resource | Actions | Seeded by |
|---|---|---|
| users | create, read, update, delete | M0 Identity |
| roles | create, read, update, delete | M0 Identity |
| permissions | read | M0 Identity |
| audit | read | M0 Identity |
| organization | read, update | M0 Identity catalogue |
| employee | read, create, update, read_sensitive, update_sensitive, admin | M0 Identity catalogue |
| attendance | read, manage, approve | HR/Attendance |
| leave | read, manage, approve | HR/Leave |
| payroll | read, manage, approve | HR/Payroll |
| recruitment | read, manage, hire | HR/Recruitment |
| performance | read, manage | HR/Performance (when wired) |

"Self" endpoints (`/me`, punch, apply leave, my payslips, my interviews, my goals…) need authentication only and resolve the employee from the JWT.

### 3.2 Events

| Exchange | Routing keys | Payload policy |
|---|---|---|
| identity (M0 Identity publisher) | tenant created/activated/suspended, user created/invited/deactivated, role/permission assigned/revoked, session revoked, password reset | ids |
| `jaas.attendance.events` | `attendance.punch.in`, `attendance.punch.out`, `attendance.regularization.requested/approved/rejected` | ids, dates, statuses |
| `jaas.leave.events` | `leave.applied`, `leave.approved`, `leave.rejected`, `leave.cancelled`, `leave.accrued` | ids, dates, days |
| `jaas.payroll.events` | `payroll.run_initiated`, `payroll.calculated`, `payroll.approved`, `payroll.finalized` | ids + period; no salaries |
| `jaas.recruitment.events` | `recruitment.job_published`, `candidate_applied`, `interview_scheduled`, `candidate_hired` | ids + stages; no candidate PII/CTC/feedback |
| `jaas.performance.events` | `performance.cycle_started`, `goal_updated`, `review_submitted`, `cycle_completed` | ids, statuses, progress; no ratings/scores/text |

All of HR/Attendance, HR/Leave, HR/Payroll, HR/Recruitment, HR/Performance use the same envelope:
`{event_id, event_type, routing_key, version "1.0", occurred_at, producer, tenant_id, correlation_id, payload}`.

---

## 4. M0 Identity — Identity & RBAC

**Tables:**
- `tenants`, `tenant_settings`;
- `users`, `roles`, `permissions`, `user_roles`, `role_permissions`;
- `sessions`, `refresh_tokens` (linked to the session by 000025);
- `password_reset_tokens`, `mfa_configs`, `audit_logs`.

**Services:**
- **`AuthService.Login`:**
  1. tenant by slug → suspended check;
  2. user by email; if missing, run a dummy bcrypt compare (timing);
  3. user status check;
  4. bcrypt compare (failures audited);
  5. create session;
  6. issue an opaque refresh token (stored hashed) + JWT access token.
- **`AuthService.RefreshToken`:**
  - hash and look up the token; expired → reject;
  - revoked by logout → plain invalid;
  - **a revoked rotated token presented again is treated as theft (RT-005): revoke all of the user's sessions and tokens**;
  - check user and tenant status, rotate (revoke old, issue new pair + session).
- **`Logout`:** revokes the session and its linked refresh token. `ForgotPassword` / `ResetPassword` use a hashed one-time token.
- **`TokenService`:** `GenerateAccessToken` (claims: user, tenant, session, roles), `ValidateAccessToken`, `GenerateOpaqueToken`, `HashOpaqueToken`.
- **`SessionService`:** create / validate / revoke / revoke-all.
- **`TenantService`:** create (optionally `WithAdminProvisioning` → first `tenant_admin` user), get by id/slug, list, update, activate, suspend.
- **`UserService`:**
  - create, invite (status `invited`, no password), get, list, update, activate;
  - deactivate (runs the convergers);
  - `WithRolePermissions`.
- **`RoleService`:** CRUD, assign/remove permissions, assign/remove roles to users, `GetMyAccess` (`/auth/me`). All grant paths go through `grants.go` anti-escalation.
- **`PermissionService.ListPermissions`**; **`AuditService.Log` / `List`**.

**Validators:** `ValidateSlug`, `IsReservedSlug`.

**Middleware:** `TenantResolver`, `Authenticate`, `RequirePermission`, `PermissionFlag`, `AuditLog`, `PlatformAdmin`.

**API (29):**

| Group | Routes | Guard |
|---|---|---|
| `/auth` | login, refresh, forgot-password, reset-password | public |
| `/auth` | logout, me | authenticated |
| `/tenants` | create, list, get, update, activate, suspend | platform key |
| `/users` | create, list, get, update, deactivate, activate, add roles, remove role | `users:*` |
| `/roles` | create, list, get, update, delete, add permissions, remove permission | `roles:*` |
| `/permissions` | list | `permissions:read` |
| `/audit-logs` | list | `audit:read` |

---

## 5. HR/Organization — Organization

**Tables:**
- `departments` (tree via `parent_id`);
- `teams` (belongs to a department, optional lead);
- `designations`;
- `mappings`: user → department / team / designation, `is_primary`, `manager_user_id`, status; append-only;
- `org_events_outbox`.

**Services:**
- **`DepartmentService`:** create/get/list/update/deactivate.
  - `assertDeptAcyclic` + `deptDepth` reject cycles and depth > `MaxHierarchyDepth` (`HIERARCHY_TOO_DEEP`);
  - names unique per tenant (case-insensitive).
- **`TeamService`:** CRUD; `checkLead`.
- **`DesignationService`:** CRUD.
- **`MappingService`:**
  - `CreateMapping` / `UpdateMapping` with `checkUser` / `checkRefs`;
  - `assertManagerAcyclic` (no A→B→A management loops);
  - `DeactivateMapping`, `ListUserMappings`, `DeactivateUserMappings` (the user-deactivation converger).
- **`OrgChartService`:** `GetChart` (nested departments with teams and headcount, cached in Redis and invalidated on writes), `GetUserChain` (reporting chain).
- **`EventConsumer.HandleIdentityEvent`.**

**API (22):** CRUD + deactivate on `/departments`, `/teams`, `/designations` and `/mappings` (5 each), plus `GET /org-chart` and `GET /org-chart/chain`.

---

## 6. HR/Employee — Employee

**Tables:**
- `employee_profiles` (1:1 with a user per tenant; `employee_code` unique per tenant; status);
- `employment_details`, `employee_contacts`;
- `employee_statutory` (tax id, national id, bank account);
- `employee_documents`, `employee_timelines`, `employee_events_outbox`.

**Status machine** (`validators.ValidateTransition`):

| From | Allowed to |
|---|---|
| probation | active, notice, resigned, terminated, on_leave, inactive |
| active | notice, resigned, terminated, on_leave, inactive |
| on_leave | active, notice, resigned, terminated, inactive |
| notice | active, resigned, terminated, inactive |
| inactive | active, terminated |
| resigned, terminated | final |

Employment types: full_time, part_time, contract, intern.

**Services:**
- **`EmployeeService`:**
  - `CreateEmployee` (user exists, code unique, one profile per user; status probation if the probation end date is in the future);
  - `GetByID`, `GetByUserID`, `List` (filters);
  - `Update`, `UpdateSelfContact`, `TransitionStatus` (+ timeline), `Deactivate`;
  - `ConvergeUserDeactivation`.
- **`EmployeeStatutoryService`:** `Upsert`; `GetByProfileID(sensitive)` masks every value unless `employee:read_sensitive` is held (via `PermissionFlag`). Unmasked reads are audited.
- **`EmployeeDocumentService`** (upload/list/verify), **`EmployeeTimelineService`**.

**API (14):**
- `/employees/me` GET, PATCH;
- create, list, get, update, status, deactivate;
- statutory GET/PUT;
- documents POST/GET/verify;
- timeline.

---

## 7. HR/Attendance — Attendance, Shifts & Time

**Tables:** `shifts`, `shift_assignments`, `attendance_records` (one per employee per day), `attendance_punches` (append-only; `is_superseded`), `attendance_regularizations`, `attendance_events_outbox`.

**Engine `attendance/calc`:**
- `ParseHHMM` / `FormatHHMM`;
- `IsNightShift` (end ≤ start);
- `ShiftStart`, `SpanMinutes`, `ExpectedMinutes` (span − break; 480 without a shift);
- `AttendanceDate`: a night-shift early "in" belongs to the previous date;
- `Thresholds` (full/half day);
- `ComputeTotals(punches, shift)`: work = Σ closed sessions, break = gaps, late = first in − (start + grace), overtime = work − expected, status present / half_day / absent; `OpenSessionStart`.

**Key rules:**
- **Punch input** (AT-001, AT-002): server clock only; caller needs a working employee profile.
- **Session rules** (AT-003, AT-004, AT-006): alternate in/out; a punch within 60 s is a duplicate; sessions > 20 h are abandoned.
- **Shifts and assignments** (AT-011, AT-012): assignments never overlap; a shift can't be deactivated while assigned.
- **Regularization requests** (AT-014 to AT-017): within 30 days; one pending per date; no self-approval; approval supersedes punches and recomputes the day.
- **Concurrency** (AT-022): per-employee advisory lock on every write.

**Services:**
- `PunchService.Punch`;
- `QueryService` (`Today`, `Mine`, `List`, `Detail`, `Summary`);
- `ShiftService` (CRUD/deactivate);
- `AssignmentService.Assign` (close or reject overlaps);
- `RegularizationService` (Create / Cancel / Approve / Reject / List / ListMine; `regularization_review.go` applies the approval);
- `LeaveSync` (`MarkLeave` / `ClearLeave`, for HR/Leave);
- `OutboxRelay`.

**API (19):**
- `/attendance`: punch, me/today, me, records, records/:id, summary, regularizations (create, me, list, cancel, approve, reject);
- `/shifts`: create, list, get, update, deactivate, assign;
- a shift-assignment list.

Permissions `attendance:read|manage|approve`.

---

## 8. HR/Leave — Leave & Absence

**Tables:** `leave_types`, `leave_holidays`, `leave_balances`, `leave_requests`, `leave_ledger` (append-only), `leave_events_outbox`.

**Engine `leave/calc`:**
- `Days` (hundredths of a day);
- `CountDays`: working days minus weekends/holidays, sandwich rule, half days;
- `Accrued`: annual prorated by joining date, or monthly allowance/12 with December absorbing the remainder;
- `CarryForward`: min(max(previous available, 0), cap).

**Ledger design:**
- Every change is a ledger row: accrual, carry_forward, reserve, release, consume, reversal, adjustment.
- `leave_balances` is a projection: `available = opening + accrued + adjusted − used − reserved`, kept equal to ledger sums (LV-013).
- Balances are created lazily on first touch (no scheduler).

**Request lifecycle:**
- **Apply:** reserve days.
- **Approve:**
  - consume the reservation;
  - mark full-day working dates `on_leave` in HR/Attendance (`LeaveSync`);
  - unpaid types are recorded as used (LOP).
- **Reject / cancel:** release or reverse.
  - Cancelling approved leave is allowed only before it starts; it reverses the ledger and clears the attendance marks.

**Rules:**
- **Validity** (LV-002 to LV-005): type active and gender-applicable; dates within the current year; minimum notice, or a 30-day backdating window.
- **Counting** (LV-006, LV-007): half days only on a single date; zero working days rejected; `max_consecutive_days` enforced.
- **Conflicts** (LV-008, LV-009): no overlap with your own pending/approved requests; paid leave needs available balance.
- **Approval** (LV-010): approver ≠ requester.
- **Concurrency** (LV-014): per-employee advisory lock.

**Services:**
- `TypeService`, `HolidayService`;
- `BalanceService` (`Mine`, `ForEmployee`, `Adjust`, `Ledger`);
- `RequestService` (`Preview`, `Apply`, `Approve`, `Reject`, `Cancel`, `Get`, `List`, `ListMine`); `ledger.go` (`ensure`, `move`, `carryForward`); `request_review.go` (`settle` / `unwind`);
- `UnpaidLeave.UnpaidDays` (for HR/Payroll);
- `OutboxRelay`.

**API (20):**
- types: CRUD + deactivate;
- holidays: create / list / delete;
- balances: me / list / adjust; ledger;
- requests: preview / apply / me / cancel / list / get / approve / reject.

Permissions `leave:read|manage|approve`.

---

## 9. HR/Payroll — Payroll & Statutory Compliance (India)

**Tables:** `payroll_structures`, `payroll_components`, `payroll_assignments` (CTC history), `payroll_runs`, `payroll_payslips`, `payroll_payslip_lines`, `payroll_events_outbox`.

**Engine `payroll/calc`:**
- **`Money`** (paise): `ParseMoney`, `Percent`, `Prorate`, `RoundRupee`, `CeilRupee`, `Min`.
- **`ValidateStructure`:** exactly one BASIC, at most one balance component, no overflow.
- **`Breakdown`** (full month):
  - fixed = value;
  - percent_of_ctc = monthly CTC × p%;
  - percent_of_basic = BASIC × p%;
  - balance = monthly CTC − other earnings.
- **Statutory deductions:**
  - **PF** = 12% × min(earned Basic, ₹15,000), rounded to the rupee.
  - **ESI** = 0.75% × earned gross when full-month gross ≤ ₹21,000, rounded up.
  - **PT** = ₹200 when earned gross ≥ ₹25,000.
  - **TDS**, new regime FY 2025-26: annual taxable = 12 × monthly taxable − ₹75,000.
    - Slabs 0/5/10/15/20/25/30% at 4/8/12/16/20/24 L.
    - Rebate 87A: zero tax ≤ ₹12 L; no marginal relief.
    - Plus 4% cess; monthly = annual / 12.
- **`Compute`:** earned amounts = full × payable / days. Deductions are capped at gross (`NET_CAPPED`), so net pay ≥ 0.

**Run lifecycle:**
- draft → calculated (recalculate replaces payslips) → approved → finalized.
- **Maker-checker:** the approver must differ from the last calculator (`SELF_APPROVAL_FORBIDDEN`).
- The run row is locked `FOR UPDATE`; one run per period; no future periods.

**Calculation per employee:**
1. window = [max(month start, joining), min(month end, exit)];
2. LOP from HR/Leave `UnpaidDays`;
3. payable = window days − LOP;
4. effective assignment → structure spec → `Compute` → payslip + lines.

**Services:**
- `StructureService` (create/update/deactivate; components immutable once assigned);
- `AssignmentService` (`Assign` closes the previous assignment; `List`, `Mine`, `Preview`);
- `RunService` (`Create`, `Calculate`, `Approve`, `Finalize`, `Get`, `List`);
- `PayslipService` (`ByRun`, `Get`, `Mine`, `MineOne`, `PayoutCSV`; `validators.CSVSafe` blocks spreadsheet formula injection);
- `OutboxRelay`.

**API (20):** structures (5), assignments (4 incl. `/me`, `/preview`), runs (8 incl. calculate / approve / finalize / payslips / payout.csv), payslips (3 incl. `/me`, `/me/:id`). Permissions `payroll:read|manage|approve`.

**Visibility:** employees see only their own payslips of finalized runs; others → 404.

---

## 10. HR/Recruitment — Recruitment (ATS)

**Tables:**
- `recruitment_jobs`;
- `recruitment_candidates` (unique `(job_id, lower(email))`);
- `recruitment_stage_events` (append-only);
- `recruitment_interviews`;
- `recruitment_offers` (partial unique index: one `offered` per candidate);
- `recruitment_events_outbox`.

**Engine `recruitment/pipeline`:**
- Candidate stages: applied → screening → interview; exits rejected / withdrawn; `offer` only by creating an offer; `hired` only by hire.
- Job statuses: draft → open ⇄ on_hold → closed; `filled` automatic at headcount.

**Services:**
- **`JobService`:**
  - `Create` validates the department/designation exist in HR/Organization (`OrgDirectory` adapter over HR/Organization services);
  - `Update` only while editable; headcount ≥ hired;
  - `SetStatus`: the first open publishes `job_published`.
- **`CandidateService`:**
  - `Create`: job must be open; email normalized and unique per job; first stage event; `candidate_applied`.
  - `Get` (with history), `List`, `Update` (not when hired).
  - `MoveStage`: graph-checked; appends a stage event; leaving `offer` withdraws the open offer (D6-07).
- **`InterviewService`:**
  - `Schedule`: candidate in `interview`, future time, interviewer has a working status; `interview_scheduled`.
  - `ListByCandidate`, `ListMine`.
  - `Feedback`: only the assigned interviewer (others 404), once; `Cancel`.
- **`OfferService`:**
  - `Create`: CTC 0 < x ≤ ₹1,000,000,000; joining ≥ today; expiry ≤ joining; no open or accepted offer (D6-08); candidate → `offer`.
  - `Decide`: accepted/declined; expired offers can't be accepted.
  - `Withdraw`, `List`.
- **`HireService.Hire`** — the 3-step idempotent saga (D6-04):
  1. Transaction: lock candidate and job; require stage `offer`, an accepted offer, and job open/on_hold. If `hired_user_id` is empty, call M0 Identity `InviteUser` (status invited, no roles) and store its id.
  2. HR/Employee: reuse the user's profile if present, else `CreateEmployee` (joining date = offer joining date; employment type from the request or the job).
  3. Transaction: candidate → `hired` (+ event, `hired_employee_id`); `hired_count + 1` (`filled` at headcount); outbox `candidate_hired`.
  - A retry after any failure reuses the same user and profile, so there's never a second invite.
- **`OutboxRelay`.**

**API (20):**
- jobs (5);
- candidates (6 incl. stage, hire);
- interviews (5; `/me` and feedback are self);
- offers (4).

Permissions `recruitment:read|manage|hire`. Hire is a separate permission because it creates users.

---

## 11. HR/Performance — Performance & OKRs (in progress)

**Design (frozen in `docs/modules/module-7-performance`):** 22 ops, rules PM-001..PM-017, goldens G1–G15, decisions D7-01..D7-06.
- **Cycles:** draft → active → review → completed. Fixed criteria (default: Quality, Delivery, Collaboration, Ownership).
- **Goals:**
  - weight 0.01–100, Σ per employee per cycle ≤ 100 (advisory lock);
  - parent goal in the same cycle, depth ≤ 3;
  - key results (progress = clamp((current − start) / (target − start) × 100)); goal progress = mean of key results;
  - append-only check-ins.
- **Access:** owner, the owner's manager (HR/Organization primary mapping), or `performance:read|manage`; others → 404.
- **Reviews:**
  - **Launch:** "launch reviews" creates self + manager reviews (idempotent).
  - **Who submits:** the manager adds peers; only the assigned reviewer submits.
  - **Submission rules:** exact criteria, scores 1–5; overall = mean; potential is required on manager reviews only.
  - **Expiry:** completing the cycle expires pending reviews.
  - **Visibility:** subjects see submitted reviews only after completion, with peers anonymous.
- **9-box:** manager overall score band (<2.50 low, <3.50 medium, else high) × potential, plus the weighted goal score.

**Built:**

| Package / item | State | Contents |
|---|---|---|
| `score` | 100% | `Fixed`, `Parse`, `KRProgress`, `Mean`, `WeightedScore`, `Overall`, `Band` |
| `lifecycle` | 100% | `CanTransitionCycle`, `CanTransitionGoal`, `CycleEditable`, `GoalsOpen`, `CheckInsOpen`, `ReviewsOpen` |
| `models` | 100% | 6 tables; JSON types `Criteria`, `Ratings` |
| Migrations 000051–000056 | cycle + parity clean (72 columns, 16 indexes) | DB checks: self review ⇔ reviewer = subject; submitted ⇒ score + timestamp; weight/progress ranges |
| `validators` | 100%, committed | `Criteria`, `DateRange`, `Weight`, `Progress`, `KeyResult`, `Ratings`, `Canonical`, `Potential` |

**Remaining:** DTOs, repositories, services, controllers/routes, wiring, Swagger, integration tests, DoD docs.

---

## 12. End-to-end flows

**Tenant onboarding:**
1. `POST /api/v1/tenants` with `X-Platform-Key` → tenant + first `tenant_admin` user.
2. `POST /api/v1/auth/login {tenant_slug, email, password}` on `<slug>.localhost` → access + refresh tokens.

**Hire → employee → time → pay:**
1. Recruiter creates and opens a job.
2. Adds a candidate; screening → interview; schedules an interview.
3. The interviewer submits feedback.
4. Offer → accepted → **hire**: user invited (M0 Identity) + employee profile (HR/Employee).
5. HR assigns a shift (HR/Attendance) and a CTC structure (HR/Payroll).
6. The employee punches in/out daily → records with totals.
7. The employee applies for leave → manager approves → ledger consumed + attendance days `on_leave`.
8. Month end, payroll run:
   - calculate (LOP from unpaid leave);
   - approve by a different user;
   - finalize;
   - employee sees the payslip; HR downloads the payout CSV.

**Every write path:**
1. Middleware: tenant + auth + permission.
2. Controller: parse + Actor.
3. Service, inside one transaction: lock → rules → write rows → write the outbox row.
4. Commit.
5. Audit log + publish + mark published.
6. If the broker is down, the relay retries later.

---

## 13. How to run and verify
```bash
docker start ajaas-postgres-1 ajaas-redis-1 ajaas-rabbitmq-1
cd backend
APP_ENV=development DATABASE_PASSWORD=postgres DATABASE_DBNAME=<fresh db> go run ./cmd     # applies 000001–000050(+)
go build ./... && go vet ./... && go test -count=1 ./...                                  # unit
JAAS_REQUIRE_INTEGRATION=1 go test -count=1 -timeout 20m ./tests/integration/           # in-process, real Postgres + Redis
```
- **Integration harness** (`tests/integration/harness_test.go`):
  - DB `jaas_itest`, Redis DB 15;
  - each test provisions its own tenant via the platform API;
  - helpers `newTenant`, `newTimeTenant`, `limitedUser(perms…)`, `call`, `expect`, `expectErr`, `sqlStr`.
- **Integration files:** identity, org, employee, attendance, leave, payroll, recruitment, security, golden, perf, schema, smoke.
- **Manual verification:** `docs/TEST_CONSOLE.md`; per-context manual checklists in each `docs/modules/m1-hr-management/contexts/<context>/testing.md`.

## 14. Known gaps and follow-ups
- **Frontend:** HR/Attendance, HR/Leave, HR/Payroll, HR/Recruitment, HR/Performance have no frontend yet.
- **Recruitment last seat:** concurrent last-seat hires leave an orphan employee profile from the losing request. Documented, untested.
- **Payroll v1 limits:** no employer PF/ESI contributions; one PT slab; new regime only; mid-month CTC change not split.
- **Partner code:** `internal/app.Build` is 77 lines. `TestPerformanceBudgets` can flake under host load.
- **Notifications (M10):** not built. Events are produced but nothing consumes them into emails yet.

---

# Part 2 — Appendix: generated inventory (from source, read-only)

Every non-test Go file under `backend/internal`, with its exported and unexported top-level types, functions, methods and interface methods (signature + doc comment), every HTTP route with its middleware/permission chain, every SQL table with its columns and constraints, and every test function. Regenerate rather than hand-edit.


## A. HTTP routes (all modules)


### M0 Identity & Access — 44 routes

| Method | Path | Middleware / permission → handler |
|---|---|---|
| POST | `/api/v1/auth/login` | sharedMiddleware.FailureRateLimiter(redisClient, 10, 15*time.Minute), authCtrl.Login |
| POST | `/api/v1/auth/refresh` | authCtrl.Refresh |
| POST | `/api/v1/auth/forgot-password` | sharedMiddleware.RateLimiter(redisClient, 5, 1*time.Hour), authCtrl.ForgotPassword |
| POST | `/api/v1/auth/reset-password` | authCtrl.ResetPassword |
| POST | `/api/v1/auth/accept-invitation` | sharedMiddleware.FailureRateLimiter(redisClient, 10, 15*time.Minute), inviteCtrl.Accept |
| POST | `/api/v1/tenants` | [group: identityMiddleware.PlatformAdmin(platformAdminKey)] tenantCtrl.Create |
| GET | `/api/v1/tenants` | [group: identityMiddleware.PlatformAdmin(platformAdminKey)] tenantCtrl.List |
| GET | `/api/v1/tenants/:id` | [group: identityMiddleware.PlatformAdmin(platformAdminKey)] tenantCtrl.GetByID |
| PATCH | `/api/v1/tenants/:id` | [group: identityMiddleware.PlatformAdmin(platformAdminKey)] tenantCtrl.Update |
| POST | `/api/v1/tenants/:id/activate` | [group: identityMiddleware.PlatformAdmin(platformAdminKey)] tenantCtrl.Activate |
| POST | `/api/v1/tenants/:id/suspend` | [group: identityMiddleware.PlatformAdmin(platformAdminKey)] tenantCtrl.Suspend |
| POST | `/api/v1/auth/logout` | [group: authMiddleware] authCtrl.Logout |
| GET | `/api/v1/auth/me` | [group: authMiddleware] roleCtrl.Me |
| GET | `/api/v1/auth/me/access` | [group: authMiddleware] accessCtrl.ExplainMe |
| POST | `/api/v1/users` | [group: tenantResolver, authMiddleware] identityMiddleware.RequirePermission(authz, "users", "create"), identityMiddleware.AuditLog(db, auditSvc, "user.created", "user"), userCtrl.Create |
| GET | `/api/v1/users` | [group: tenantResolver, authMiddleware] identityMiddleware.RequirePermission(authz, "users", "read"), userCtrl.List |
| GET | `/api/v1/users/:id` | [group: tenantResolver, authMiddleware] identityMiddleware.RequirePermission(authz, "users", "read"), userCtrl.GetByID |
| PATCH | `/api/v1/users/:id` | [group: tenantResolver, authMiddleware] identityMiddleware.RequirePermission(authz, "users", "update"), identityMiddleware.AuditLog(db, auditSvc, "user.updated", "user"), userCtrl.Update |
| POST | `/api/v1/users/:id/deactivate` | [group: tenantResolver, authMiddleware] identityMiddleware.RequirePermission(authz, "users", "delete"), identityMiddleware.AuditLog(db, auditSvc, "user.deactivated", "user"), userCtrl.Deactivate |
| POST | `/api/v1/users/:id/activate` | [group: tenantResolver, authMiddleware] identityMiddleware.RequirePermission(authz, "users", "update"), identityMiddleware.AuditLog(db, auditSvc, "user.activated", "user"), userCtrl.Activate |
| DELETE | `/api/v1/users/:id/roles/:role_id` | [group: tenantResolver, authMiddleware] identityMiddleware.RequirePermission(authz, "users", "update"), identityMiddleware.AuditLog(db, auditSvc, "role.removed", "user"), roleCtrl.RemoveRoleFromUser |
| GET | `/api/v1/users/:id/access` | [group: tenantResolver, authMiddleware] identityMiddleware.RequirePermission(authz, "access", "read"), accessCtrl.Explain |
| GET | `/api/v1/users/:id/grants` | [group: tenantResolver, authMiddleware] identityMiddleware.RequirePermission(authz, "access", "read"), accessCtrl.ListGrants |
| POST | `/api/v1/users/:id/grants` | [group: tenantResolver, authMiddleware] identityMiddleware.RequirePermission(authz, "access", "manage"), accessCtrl.Grant |
| DELETE | `/api/v1/users/:id/grants/:permission_id` | [group: tenantResolver, authMiddleware] identityMiddleware.RequirePermission(authz, "access", "manage"), accessCtrl.RevokeGrant |
| GET | `/api/v1/users/:id/blocks` | [group: tenantResolver, authMiddleware] identityMiddleware.RequirePermission(authz, "access", "read"), accessCtrl.ListBlocks |
| POST | `/api/v1/users/:id/blocks` | [group: tenantResolver, authMiddleware] identityMiddleware.RequirePermission(authz, "access", "manage"), accessCtrl.Block |
| DELETE | `/api/v1/users/:id/blocks/:permission_id` | [group: tenantResolver, authMiddleware] identityMiddleware.RequirePermission(authz, "access", "manage"), accessCtrl.Unblock |
| POST | `/api/v1/users/:id/roles` | [group: tenantResolver, authMiddleware] identityMiddleware.RequirePermission(authz, "users", "update"), identityMiddleware.AuditLog(db, auditSvc, "role.assigned", "user"), roleCtrl.AssignRolesToUser |
| POST | `/api/v1/roles` | [group: tenantResolver, authMiddleware] identityMiddleware.RequirePermission(authz, "roles", "create"), identityMiddleware.AuditLog(db, auditSvc, "role.created", "role"), roleCtrl.Create |
| GET | `/api/v1/roles` | [group: tenantResolver, authMiddleware] identityMiddleware.RequirePermission(authz, "roles", "read"), roleCtrl.List |
| GET | `/api/v1/roles/:id` | [group: tenantResolver, authMiddleware] identityMiddleware.RequirePermission(authz, "roles", "read"), roleCtrl.GetByID |
| PATCH | `/api/v1/roles/:id` | [group: tenantResolver, authMiddleware] identityMiddleware.RequirePermission(authz, "roles", "update"), identityMiddleware.AuditLog(db, auditSvc, "role.updated", "role"), roleCtrl.Update |
| DELETE | `/api/v1/roles/:id` | [group: tenantResolver, authMiddleware] identityMiddleware.RequirePermission(authz, "roles", "delete"), identityMiddleware.AuditLog(db, auditSvc, "role.deleted", "role"), roleCtrl.Delete |
| POST | `/api/v1/roles/:id/clone` | [group: tenantResolver, authMiddleware] identityMiddleware.RequirePermission(authz, "roles", "create"), identityMiddleware.AuditLog(db, auditSvc, "role.created", "role"), roleCtrl.Clone |
| DELETE | `/api/v1/roles/:id/permissions/:permission_id` | [group: tenantResolver, authMiddleware] identityMiddleware.RequirePermission(authz, "roles", "update"), identityMiddleware.AuditLog(db, auditSvc, "permission.removed", "role"), roleCtrl.RemovePermission |
| POST | `/api/v1/roles/:id/permissions` | [group: tenantResolver, authMiddleware] identityMiddleware.RequirePermission(authz, "roles", "update"), identityMiddleware.AuditLog(db, auditSvc, "permission.assigned", "role"), roleCtrl.AssignPermissions |
| POST | `/api/v1/invitations` | [group: tenantResolver, authMiddleware] identityMiddleware.RequirePermission(authz, "users", "create"), inviteCtrl.Create |
| GET | `/api/v1/invitations` | [group: tenantResolver, authMiddleware] identityMiddleware.RequirePermission(authz, "users", "read"), inviteCtrl.List |
| POST | `/api/v1/invitations/:id/resend` | [group: tenantResolver, authMiddleware] identityMiddleware.RequirePermission(authz, "users", "create"), inviteCtrl.Resend |
| POST | `/api/v1/invitations/:id/revoke` | [group: tenantResolver, authMiddleware] identityMiddleware.RequirePermission(authz, "users", "update"), inviteCtrl.Revoke |
| POST | `/api/v1/access/automatic-roles/sync` | [group: tenantResolver, authMiddleware] identityMiddleware.RequirePermission(authz, "access", "manage"), identityMiddleware.AuditLog(db, auditSvc, "access.automatic_roles_synced", "tenant"), accessCtrl.SyncAutomaticRoles |
| GET | `/api/v1/audit-logs` | [group: tenantResolver, authMiddleware] identityMiddleware.RequirePermission(authz, "audit", "read"), auditCtrl.List |
| GET | `/api/v1/permissions` | [group: authMiddleware] identityMiddleware.RequirePermission(authz, "permissions", "read"), permCtrl.List |

### M1 HR / Organization — 22 routes

| Method | Path | Middleware / permission → handler |
|---|---|---|
| POST | `/api/v1/hr/departments` | [group: tenantResolver, authMiddleware, invalidate] write, auditMW("department.created", "department"), deptCtrl.Create |
| GET | `/api/v1/hr/departments` | [group: tenantResolver, authMiddleware, invalidate] read, deptCtrl.List |
| GET | `/api/v1/hr/departments/:id` | [group: tenantResolver, authMiddleware, invalidate] read, deptCtrl.GetByID |
| PATCH | `/api/v1/hr/departments/:id` | [group: tenantResolver, authMiddleware, invalidate] write, auditMW("department.updated", "department"), deptCtrl.Update |
| POST | `/api/v1/hr/departments/:id/deactivate` | [group: tenantResolver, authMiddleware, invalidate] write, auditMW("department.deactivated", "department"), deptCtrl.Deactivate |
| POST | `/api/v1/hr/teams` | [group: tenantResolver, authMiddleware, invalidate] write, auditMW("team.created", "team"), teamCtrl.Create |
| GET | `/api/v1/hr/teams` | [group: tenantResolver, authMiddleware, invalidate] read, teamCtrl.List |
| GET | `/api/v1/hr/teams/:id` | [group: tenantResolver, authMiddleware, invalidate] read, teamCtrl.GetByID |
| PATCH | `/api/v1/hr/teams/:id` | [group: tenantResolver, authMiddleware, invalidate] write, auditMW("team.updated", "team"), teamCtrl.Update |
| POST | `/api/v1/hr/teams/:id/deactivate` | [group: tenantResolver, authMiddleware, invalidate] write, auditMW("team.deactivated", "team"), teamCtrl.Deactivate |
| POST | `/api/v1/hr/designations` | [group: tenantResolver, authMiddleware] write, auditMW("designation.created", "designation"), desigCtrl.Create |
| GET | `/api/v1/hr/designations` | [group: tenantResolver, authMiddleware] read, desigCtrl.List |
| GET | `/api/v1/hr/designations/:id` | [group: tenantResolver, authMiddleware] read, desigCtrl.GetByID |
| PATCH | `/api/v1/hr/designations/:id` | [group: tenantResolver, authMiddleware] write, auditMW("designation.updated", "designation"), desigCtrl.Update |
| POST | `/api/v1/hr/designations/:id/deactivate` | [group: tenantResolver, authMiddleware] write, auditMW("designation.deactivated", "designation"), desigCtrl.Deactivate |
| POST | `/api/v1/hr/mappings` | [group: tenantResolver, authMiddleware, invalidate] write, auditMW("mapping.created", "mapping"), mappingCtrl.Create |
| GET | `/api/v1/hr/mappings` | [group: tenantResolver, authMiddleware, invalidate] read, mappingCtrl.ListByUser |
| GET | `/api/v1/hr/mappings/:id` | [group: tenantResolver, authMiddleware, invalidate] read, mappingCtrl.GetByID |
| PATCH | `/api/v1/hr/mappings/:id` | [group: tenantResolver, authMiddleware, invalidate] write, auditMW("mapping.updated", "mapping"), mappingCtrl.Update |
| POST | `/api/v1/hr/mappings/:id/deactivate` | [group: tenantResolver, authMiddleware, invalidate] write, auditMW("mapping.deactivated", "mapping"), mappingCtrl.Deactivate |
| GET | `/api/v1/hr/org-chart` | [group: tenantResolver, authMiddleware, read] chartCtrl.Chart |
| GET | `/api/v1/hr/org-chart/chain` | [group: tenantResolver, authMiddleware, read] chartCtrl.UserChain |

### M1 HR / Employee — 14 routes

| Method | Path | Middleware / permission → handler |
|---|---|---|
| GET | `/api/v1/hr/employees/me` | [group: tenantResolver, authMiddleware] empCtrl.GetMe |
| PATCH | `/api/v1/hr/employees/me` | [group: tenantResolver, authMiddleware] auditMW("employee.self_updated", "employee"), empCtrl.UpdateMe |
| POST | `/api/v1/hr/employees` | [group: tenantResolver, authMiddleware] create, auditMW("employee.created", "employee"), empCtrl.Create |
| GET | `/api/v1/hr/employees` | [group: tenantResolver, authMiddleware] read, empCtrl.List |
| GET | `/api/v1/hr/employees/:id` | [group: tenantResolver, authMiddleware] read, empCtrl.GetByID |
| PATCH | `/api/v1/hr/employees/:id` | [group: tenantResolver, authMiddleware] update, auditMW("employee.updated", "employee"), empCtrl.Update |
| POST | `/api/v1/hr/employees/:id/status` | [group: tenantResolver, authMiddleware] update, auditMW("employee.status_changed", "employee"), empCtrl.TransitionStatus |
| POST | `/api/v1/hr/employees/:id/deactivate` | [group: tenantResolver, authMiddleware] update, auditMW("employee.deactivated", "employee"), empCtrl.Deactivate |
| GET | `/api/v1/hr/employees/:id/statutory` | [group: tenantResolver, authMiddleware] read, sensitiveFlag, auditUnmasked, statCtrl.Get |
| PUT | `/api/v1/hr/employees/:id/statutory` | [group: tenantResolver, authMiddleware] updateSensitive, auditMW("employee.statutory_updated", "employee"), statCtrl.Upsert |
| POST | `/api/v1/hr/employees/:id/documents` | [group: tenantResolver, authMiddleware] update, auditMW("employee.document_uploaded", "employee"), docCtrl.Upload |
| GET | `/api/v1/hr/employees/:id/documents` | [group: tenantResolver, authMiddleware] read, docCtrl.List |
| POST | `/api/v1/hr/employees/:id/documents/:doc_id/verify` | [group: tenantResolver, authMiddleware] admin, auditMW("employee.document_verified", "employee"), docCtrl.Verify |
| GET | `/api/v1/hr/employees/:id/timeline` | [group: tenantResolver, authMiddleware] read, tlCtrl.List |

### M1 HR / Attendance — 19 routes

| Method | Path | Middleware / permission → handler |
|---|---|---|
| POST | `/api/v1/hr/attendance/punch` | [group: mw.TenantResolver, mw.Authenticate] mw.PunchLimit, ctl.Attendance.Punch |
| GET | `/api/v1/hr/attendance/me/today` | [group: mw.TenantResolver, mw.Authenticate] ctl.Attendance.Today |
| GET | `/api/v1/hr/attendance/me` | [group: mw.TenantResolver, mw.Authenticate] ctl.Attendance.Mine |
| GET | `/api/v1/hr/attendance/records` | [group: mw.TenantResolver, mw.Authenticate] read, ctl.Attendance.List |
| GET | `/api/v1/hr/attendance/records/:id` | [group: mw.TenantResolver, mw.Authenticate] read, ctl.Attendance.Detail |
| GET | `/api/v1/hr/attendance/summary` | [group: mw.TenantResolver, mw.Authenticate] read, ctl.Attendance.Summary |
| POST | `/api/v1/hr/attendance/regularizations` | [group: mw.TenantResolver, mw.Authenticate] ctl.Regularization.Create |
| GET | `/api/v1/hr/attendance/regularizations/me` | [group: mw.TenantResolver, mw.Authenticate] ctl.Regularization.ListMine |
| GET | `/api/v1/hr/attendance/regularizations` | [group: mw.TenantResolver, mw.Authenticate] read, ctl.Regularization.List |
| POST | `/api/v1/hr/attendance/regularizations/:id/cancel` | [group: mw.TenantResolver, mw.Authenticate] ctl.Regularization.Cancel |
| POST | `/api/v1/hr/attendance/regularizations/:id/approve` | [group: mw.TenantResolver, mw.Authenticate] approve, ctl.Regularization.Approve |
| POST | `/api/v1/hr/attendance/regularizations/:id/reject` | [group: mw.TenantResolver, mw.Authenticate] approve, ctl.Regularization.Reject |
| POST | `/api/v1/hr/shifts` | [group: mw.TenantResolver, mw.Authenticate] manage, ctl.Shift.Create |
| GET | `/api/v1/hr/shifts` | [group: mw.TenantResolver, mw.Authenticate] read, ctl.Shift.List |
| GET | `/api/v1/hr/shifts/:id` | [group: mw.TenantResolver, mw.Authenticate] read, ctl.Shift.Get |
| PATCH | `/api/v1/hr/shifts/:id` | [group: mw.TenantResolver, mw.Authenticate] manage, ctl.Shift.Update |
| POST | `/api/v1/hr/shifts/:id/deactivate` | [group: mw.TenantResolver, mw.Authenticate] manage, ctl.Shift.Deactivate |
| POST | `/api/v1/hr/shifts/:id/assignments` | [group: mw.TenantResolver, mw.Authenticate] manage, ctl.Shift.Assign |
| GET | `/api/v1/hr/shift-assignments` | mw.TenantResolver, mw.Authenticate, read, ctl.Shift.ListAssignments |

### M1 HR / Leave — 20 routes

| Method | Path | Middleware / permission → handler |
|---|---|---|
| POST | `/api/v1/hr/leave/types` | [group: mw.TenantResolver, mw.Authenticate] manage, ctl.Policy.CreateType |
| GET | `/api/v1/hr/leave/types` | [group: mw.TenantResolver, mw.Authenticate] ctl.Policy.ListTypes |
| GET | `/api/v1/hr/leave/types/:id` | [group: mw.TenantResolver, mw.Authenticate] ctl.Policy.GetType |
| PATCH | `/api/v1/hr/leave/types/:id` | [group: mw.TenantResolver, mw.Authenticate] manage, ctl.Policy.UpdateType |
| POST | `/api/v1/hr/leave/types/:id/deactivate` | [group: mw.TenantResolver, mw.Authenticate] manage, ctl.Policy.DeactivateType |
| POST | `/api/v1/hr/leave/holidays` | [group: mw.TenantResolver, mw.Authenticate] manage, ctl.Policy.CreateHoliday |
| GET | `/api/v1/hr/leave/holidays` | [group: mw.TenantResolver, mw.Authenticate] ctl.Policy.ListHolidays |
| DELETE | `/api/v1/hr/leave/holidays/:id` | [group: mw.TenantResolver, mw.Authenticate] manage, ctl.Policy.DeleteHoliday |
| GET | `/api/v1/hr/leave/balances/me` | [group: mw.TenantResolver, mw.Authenticate] ctl.Balance.Mine |
| GET | `/api/v1/hr/leave/balances` | [group: mw.TenantResolver, mw.Authenticate] read, ctl.Balance.ForEmployee |
| POST | `/api/v1/hr/leave/balances/adjust` | [group: mw.TenantResolver, mw.Authenticate] manage, ctl.Balance.Adjust |
| GET | `/api/v1/hr/leave/ledger` | [group: mw.TenantResolver, mw.Authenticate] read, ctl.Balance.Ledger |
| POST | `/api/v1/hr/leave/requests/preview` | [group: mw.TenantResolver, mw.Authenticate] ctl.Request.Preview |
| POST | `/api/v1/hr/leave/requests` | [group: mw.TenantResolver, mw.Authenticate] ctl.Request.Apply |
| GET | `/api/v1/hr/leave/requests/me` | [group: mw.TenantResolver, mw.Authenticate] ctl.Request.ListMine |
| POST | `/api/v1/hr/leave/requests/:id/cancel` | [group: mw.TenantResolver, mw.Authenticate] ctl.Request.Cancel |
| GET | `/api/v1/hr/leave/requests` | [group: mw.TenantResolver, mw.Authenticate] read, ctl.Request.List |
| GET | `/api/v1/hr/leave/requests/:id` | [group: mw.TenantResolver, mw.Authenticate] read, ctl.Request.Get |
| POST | `/api/v1/hr/leave/requests/:id/approve` | [group: mw.TenantResolver, mw.Authenticate] approve, ctl.Request.Approve |
| POST | `/api/v1/hr/leave/requests/:id/reject` | [group: mw.TenantResolver, mw.Authenticate] approve, ctl.Request.Reject |

### M1 HR / Payroll — 20 routes

| Method | Path | Middleware / permission → handler |
|---|---|---|
| POST | `/api/v1/hr/payroll/structures` | [group: mw.TenantResolver, mw.Authenticate] manage, ctl.Setup.CreateStructure |
| GET | `/api/v1/hr/payroll/structures` | [group: mw.TenantResolver, mw.Authenticate] read, ctl.Setup.ListStructures |
| GET | `/api/v1/hr/payroll/structures/:id` | [group: mw.TenantResolver, mw.Authenticate] read, ctl.Setup.GetStructure |
| PATCH | `/api/v1/hr/payroll/structures/:id` | [group: mw.TenantResolver, mw.Authenticate] manage, ctl.Setup.UpdateStructure |
| POST | `/api/v1/hr/payroll/structures/:id/deactivate` | [group: mw.TenantResolver, mw.Authenticate] manage, ctl.Setup.DeactivateStructure |
| POST | `/api/v1/hr/payroll/assignments` | [group: mw.TenantResolver, mw.Authenticate] manage, ctl.Setup.Assign |
| GET | `/api/v1/hr/payroll/assignments` | [group: mw.TenantResolver, mw.Authenticate] read, ctl.Setup.ListAssignments |
| GET | `/api/v1/hr/payroll/assignments/me` | [group: mw.TenantResolver, mw.Authenticate] ctl.Setup.MyAssignment |
| POST | `/api/v1/hr/payroll/assignments/preview` | [group: mw.TenantResolver, mw.Authenticate] manage, ctl.Setup.Preview |
| POST | `/api/v1/hr/payroll/runs` | [group: mw.TenantResolver, mw.Authenticate] manage, ctl.Run.CreateRun |
| GET | `/api/v1/hr/payroll/runs` | [group: mw.TenantResolver, mw.Authenticate] read, ctl.Run.ListRuns |
| GET | `/api/v1/hr/payroll/runs/:id` | [group: mw.TenantResolver, mw.Authenticate] read, ctl.Run.GetRun |
| POST | `/api/v1/hr/payroll/runs/:id/calculate` | [group: mw.TenantResolver, mw.Authenticate] manage, ctl.Run.Calculate |
| POST | `/api/v1/hr/payroll/runs/:id/approve` | [group: mw.TenantResolver, mw.Authenticate] approve, ctl.Run.Approve |
| POST | `/api/v1/hr/payroll/runs/:id/finalize` | [group: mw.TenantResolver, mw.Authenticate] approve, ctl.Run.Finalize |
| GET | `/api/v1/hr/payroll/runs/:id/payslips` | [group: mw.TenantResolver, mw.Authenticate] read, ctl.Run.RunPayslips |
| GET | `/api/v1/hr/payroll/runs/:id/payout.csv` | [group: mw.TenantResolver, mw.Authenticate] read, ctl.Run.PayoutCSV |
| GET | `/api/v1/hr/payroll/payslips/me` | [group: mw.TenantResolver, mw.Authenticate] ctl.Run.MyPayslips |
| GET | `/api/v1/hr/payroll/payslips/me/:id` | [group: mw.TenantResolver, mw.Authenticate] ctl.Run.MyPayslip |
| GET | `/api/v1/hr/payroll/payslips/:id` | [group: mw.TenantResolver, mw.Authenticate] read, ctl.Run.GetPayslip |

### M1 HR / Recruitment — 20 routes

| Method | Path | Middleware / permission → handler |
|---|---|---|
| POST | `/api/v1/hr/recruitment/jobs` | [group: mw.TenantResolver, mw.Authenticate] manage, ctl.CreateJob |
| GET | `/api/v1/hr/recruitment/jobs` | [group: mw.TenantResolver, mw.Authenticate] read, ctl.ListJobs |
| GET | `/api/v1/hr/recruitment/jobs/:id` | [group: mw.TenantResolver, mw.Authenticate] read, ctl.GetJob |
| PATCH | `/api/v1/hr/recruitment/jobs/:id` | [group: mw.TenantResolver, mw.Authenticate] manage, ctl.UpdateJob |
| POST | `/api/v1/hr/recruitment/jobs/:id/status` | [group: mw.TenantResolver, mw.Authenticate] manage, ctl.SetJobStatus |
| POST | `/api/v1/hr/recruitment/candidates` | [group: mw.TenantResolver, mw.Authenticate] manage, ctl.CreateCandidate |
| GET | `/api/v1/hr/recruitment/candidates` | [group: mw.TenantResolver, mw.Authenticate] read, ctl.ListCandidates |
| GET | `/api/v1/hr/recruitment/candidates/:id` | [group: mw.TenantResolver, mw.Authenticate] read, ctl.GetCandidate |
| PATCH | `/api/v1/hr/recruitment/candidates/:id` | [group: mw.TenantResolver, mw.Authenticate] manage, ctl.UpdateCandidate |
| POST | `/api/v1/hr/recruitment/candidates/:id/stage` | [group: mw.TenantResolver, mw.Authenticate] manage, ctl.MoveStage |
| POST | `/api/v1/hr/recruitment/candidates/:id/hire` | [group: mw.TenantResolver, mw.Authenticate] hire, ctl.Hire |
| POST | `/api/v1/hr/recruitment/interviews` | [group: mw.TenantResolver, mw.Authenticate] manage, ctl.ScheduleInterview |
| GET | `/api/v1/hr/recruitment/interviews` | [group: mw.TenantResolver, mw.Authenticate] read, ctl.ListInterviews |
| GET | `/api/v1/hr/recruitment/interviews/me` | [group: mw.TenantResolver, mw.Authenticate] ctl.MyInterviews |
| POST | `/api/v1/hr/recruitment/interviews/:id/feedback` | [group: mw.TenantResolver, mw.Authenticate] ctl.Feedback |
| POST | `/api/v1/hr/recruitment/interviews/:id/cancel` | [group: mw.TenantResolver, mw.Authenticate] manage, ctl.CancelInterview |
| POST | `/api/v1/hr/recruitment/offers` | [group: mw.TenantResolver, mw.Authenticate] manage, ctl.CreateOffer |
| GET | `/api/v1/hr/recruitment/offers` | [group: mw.TenantResolver, mw.Authenticate] read, ctl.ListOffers |
| POST | `/api/v1/hr/recruitment/offers/:id/decision` | [group: mw.TenantResolver, mw.Authenticate] manage, ctl.DecideOffer |
| POST | `/api/v1/hr/recruitment/offers/:id/withdraw` | [group: mw.TenantResolver, mw.Authenticate] manage, ctl.WithdrawOffer |

### M1 HR / Performance & OKRs — 33 routes

| Method | Path | Middleware / permission → handler |
|---|---|---|
| POST | `/api/v1/hr/performance/cycles` | [group: mw.TenantResolver, mw.Authenticate, mw.Flag(ActionRead, controllers.FlagRead), mw.Flag(ActionManage, controllers.FlagManage)] manage, ctl.CreateCycle |
| GET | `/api/v1/hr/performance/cycles` | [group: mw.TenantResolver, mw.Authenticate, mw.Flag(ActionRead, controllers.FlagRead), mw.Flag(ActionManage, controllers.FlagManage)] ctl.ListCycles |
| GET | `/api/v1/hr/performance/cycles/:id` | [group: mw.TenantResolver, mw.Authenticate, mw.Flag(ActionRead, controllers.FlagRead), mw.Flag(ActionManage, controllers.FlagManage)] ctl.GetCycle |
| PATCH | `/api/v1/hr/performance/cycles/:id` | [group: mw.TenantResolver, mw.Authenticate, mw.Flag(ActionRead, controllers.FlagRead), mw.Flag(ActionManage, controllers.FlagManage)] manage, ctl.UpdateCycle |
| POST | `/api/v1/hr/performance/cycles/:id/activate` | [group: mw.TenantResolver, mw.Authenticate, mw.Flag(ActionRead, controllers.FlagRead), mw.Flag(ActionManage, controllers.FlagManage)] manage, ctl.ActivateCycle |
| POST | `/api/v1/hr/performance/cycles/:id/launch-reviews` | [group: mw.TenantResolver, mw.Authenticate, mw.Flag(ActionRead, controllers.FlagRead), mw.Flag(ActionManage, controllers.FlagManage)] manage, ctl.LaunchReviews |
| POST | `/api/v1/hr/performance/cycles/:id/complete` | [group: mw.TenantResolver, mw.Authenticate, mw.Flag(ActionRead, controllers.FlagRead), mw.Flag(ActionManage, controllers.FlagManage)] manage, ctl.CompleteCycle |
| GET | `/api/v1/hr/performance/cycles/:id/nine-box` | [group: mw.TenantResolver, mw.Authenticate, mw.Flag(ActionRead, controllers.FlagRead), mw.Flag(ActionManage, controllers.FlagManage)] read, ctl.NineBox |
| POST | `/api/v1/hr/performance/cycles/:id/calibrations` | [group: mw.TenantResolver, mw.Authenticate, mw.Flag(ActionRead, controllers.FlagRead), mw.Flag(ActionManage, controllers.FlagManage)] calibrate, ctl.Calibrate |
| GET | `/api/v1/hr/performance/cycles/:id/calibrations` | [group: mw.TenantResolver, mw.Authenticate, mw.Flag(ActionRead, controllers.FlagRead), mw.Flag(ActionManage, controllers.FlagManage)] read, ctl.ListCalibrations |
| POST | `/api/v1/hr/performance/goals` | [group: mw.TenantResolver, mw.Authenticate, mw.Flag(ActionRead, controllers.FlagRead), mw.Flag(ActionManage, controllers.FlagManage)] ctl.CreateGoal |
| GET | `/api/v1/hr/performance/goals` | [group: mw.TenantResolver, mw.Authenticate, mw.Flag(ActionRead, controllers.FlagRead), mw.Flag(ActionManage, controllers.FlagManage)] ctl.ListGoals |
| GET | `/api/v1/hr/performance/goals/me` | [group: mw.TenantResolver, mw.Authenticate, mw.Flag(ActionRead, controllers.FlagRead), mw.Flag(ActionManage, controllers.FlagManage)] ctl.MyGoals |
| GET | `/api/v1/hr/performance/goals/:id` | [group: mw.TenantResolver, mw.Authenticate, mw.Flag(ActionRead, controllers.FlagRead), mw.Flag(ActionManage, controllers.FlagManage)] ctl.GetGoal |
| PATCH | `/api/v1/hr/performance/goals/:id` | [group: mw.TenantResolver, mw.Authenticate, mw.Flag(ActionRead, controllers.FlagRead), mw.Flag(ActionManage, controllers.FlagManage)] ctl.UpdateGoal |
| POST | `/api/v1/hr/performance/goals/:id/status` | [group: mw.TenantResolver, mw.Authenticate, mw.Flag(ActionRead, controllers.FlagRead), mw.Flag(ActionManage, controllers.FlagManage)] ctl.SetGoalStatus |
| POST | `/api/v1/hr/performance/goals/:id/key-results` | [group: mw.TenantResolver, mw.Authenticate, mw.Flag(ActionRead, controllers.FlagRead), mw.Flag(ActionManage, controllers.FlagManage)] ctl.AddKeyResult |
| POST | `/api/v1/hr/performance/goals/:id/check-ins` | [group: mw.TenantResolver, mw.Authenticate, mw.Flag(ActionRead, controllers.FlagRead), mw.Flag(ActionManage, controllers.FlagManage)] ctl.CheckIn |
| POST | `/api/v1/hr/performance/reviews` | [group: mw.TenantResolver, mw.Authenticate, mw.Flag(ActionRead, controllers.FlagRead), mw.Flag(ActionManage, controllers.FlagManage)] manage, ctl.AssignReviewer |
| GET | `/api/v1/hr/performance/reviews` | [group: mw.TenantResolver, mw.Authenticate, mw.Flag(ActionRead, controllers.FlagRead), mw.Flag(ActionManage, controllers.FlagManage)] read, ctl.ListReviews |
| GET | `/api/v1/hr/performance/reviews/assigned` | [group: mw.TenantResolver, mw.Authenticate, mw.Flag(ActionRead, controllers.FlagRead), mw.Flag(ActionManage, controllers.FlagManage)] ctl.AssignedReviews |
| GET | `/api/v1/hr/performance/reviews/me` | [group: mw.TenantResolver, mw.Authenticate, mw.Flag(ActionRead, controllers.FlagRead), mw.Flag(ActionManage, controllers.FlagManage)] ctl.MyReviews |
| GET | `/api/v1/hr/performance/reviews/:id` | [group: mw.TenantResolver, mw.Authenticate, mw.Flag(ActionRead, controllers.FlagRead), mw.Flag(ActionManage, controllers.FlagManage)] ctl.GetReview |
| POST | `/api/v1/hr/performance/reviews/:id/submit` | [group: mw.TenantResolver, mw.Authenticate, mw.Flag(ActionRead, controllers.FlagRead), mw.Flag(ActionManage, controllers.FlagManage)] ctl.SubmitReview |
| POST | `/api/v1/hr/performance/feedback` | [group: mw.TenantResolver, mw.Authenticate, mw.Flag(ActionRead, controllers.FlagRead), mw.Flag(ActionManage, controllers.FlagManage)] ctl.GiveFeedback |
| GET | `/api/v1/hr/performance/feedback` | [group: mw.TenantResolver, mw.Authenticate, mw.Flag(ActionRead, controllers.FlagRead), mw.Flag(ActionManage, controllers.FlagManage)] ctl.FeedbackFor |
| GET | `/api/v1/hr/performance/feedback/received` | [group: mw.TenantResolver, mw.Authenticate, mw.Flag(ActionRead, controllers.FlagRead), mw.Flag(ActionManage, controllers.FlagManage)] ctl.FeedbackReceived |
| GET | `/api/v1/hr/performance/feedback/given` | [group: mw.TenantResolver, mw.Authenticate, mw.Flag(ActionRead, controllers.FlagRead), mw.Flag(ActionManage, controllers.FlagManage)] ctl.FeedbackGiven |
| GET | `/api/v1/hr/performance/feedback/public` | [group: mw.TenantResolver, mw.Authenticate, mw.Flag(ActionRead, controllers.FlagRead), mw.Flag(ActionManage, controllers.FlagManage)] ctl.PublicFeedback |
| POST | `/api/v1/hr/performance/feedback/requests` | [group: mw.TenantResolver, mw.Authenticate, mw.Flag(ActionRead, controllers.FlagRead), mw.Flag(ActionManage, controllers.FlagManage)] ctl.RequestFeedback |
| GET | `/api/v1/hr/performance/feedback/requests/assigned` | [group: mw.TenantResolver, mw.Authenticate, mw.Flag(ActionRead, controllers.FlagRead), mw.Flag(ActionManage, controllers.FlagManage)] ctl.AssignedFeedbackRequests |
| GET | `/api/v1/hr/performance/feedback/requests/mine` | [group: mw.TenantResolver, mw.Authenticate, mw.Flag(ActionRead, controllers.FlagRead), mw.Flag(ActionManage, controllers.FlagManage)] ctl.MyFeedbackRequests |
| POST | `/api/v1/hr/performance/feedback/requests/:id/decline` | [group: mw.TenantResolver, mw.Authenticate, mw.Flag(ActionRead, controllers.FlagRead), mw.Flag(ActionManage, controllers.FlagManage)] ctl.DeclineFeedbackRequest |

Total routes registered by module route files: **192** (plus `GET /health`).


## B. Database schema (migrations, in order)


#### `identity/000001_create_tenants.up.sql`
- **table `tenants`**
  - `id` UUID — PRIMARY KEY
  - `name` VARCHAR(255) — NOT NULL
  - `slug` VARCHAR(64) — NOT NULL
  - `domain` VARCHAR(255)
  - `status` VARCHAR(20) — NOT NULL DEFAULT 'active'
  - `plan` VARCHAR(50) — NOT NULL DEFAULT 'free'
  - `created_at` TIMESTAMPTZ — NOT NULL DEFAULT CURRENT_TIMESTAMP
  - `updated_at` TIMESTAMPTZ — NOT NULL DEFAULT CURRENT_TIMESTAMP
  - `deleted_at` TIMESTAMPTZ
- UNIQUE index `idx_tenants_slug` on `tenants` (slug) WHERE deleted_at IS NULL
- UNIQUE index `idx_tenants_domain` on `tenants` (domain) WHERE domain IS NOT NULL AND deleted_at IS NULL
- index `idx_tenants_status` on `tenants` (status)
- index `idx_tenants_deleted_at` on `tenants` (deleted_at)

#### `identity/000002_create_users.up.sql`
- **table `users`**
  - `id` UUID — PRIMARY KEY
  - `tenant_id` UUID — NOT NULL REFERENCES tenants(id) ON DELETE CASCADE
  - `email` VARCHAR(255) — NOT NULL
  - `password_hash` VARCHAR(255) — NOT NULL
  - `first_name` VARCHAR(100) — NOT NULL
  - `last_name` VARCHAR(100) — NOT NULL
  - `phone` VARCHAR(20)
  - `avatar_url` VARCHAR(500)
  - `status` VARCHAR(20) — NOT NULL DEFAULT 'active'
  - `email_verified_at` TIMESTAMPTZ
  - `last_login_at` TIMESTAMPTZ
  - `created_at` TIMESTAMPTZ — NOT NULL DEFAULT CURRENT_TIMESTAMP
  - `updated_at` TIMESTAMPTZ — NOT NULL DEFAULT CURRENT_TIMESTAMP
  - `deleted_at` TIMESTAMPTZ
- UNIQUE index `uq_users_tenant_email` on `users` (tenant_id, email) WHERE deleted_at IS NULL
- index `idx_users_tenant_id` on `users` (tenant_id)
- index `idx_users_email` on `users` (email)
- index `idx_users_status` on `users` (status)
- index `idx_users_deleted_at` on `users` (deleted_at)

#### `identity/000003_create_roles.up.sql`
- **table `roles`**
  - `id` UUID — PRIMARY KEY
  - `tenant_id` UUID — NOT NULL REFERENCES tenants(id) ON DELETE CASCADE
  - `name` VARCHAR(100) — NOT NULL
  - `description` VARCHAR(500)
  - `is_system` BOOLEAN — NOT NULL DEFAULT false
  - `created_at` TIMESTAMPTZ — NOT NULL DEFAULT CURRENT_TIMESTAMP
  - `updated_at` TIMESTAMPTZ — NOT NULL DEFAULT CURRENT_TIMESTAMP
  - `deleted_at` TIMESTAMPTZ
- UNIQUE index `uq_roles_tenant_name` on `roles` (tenant_id, name) WHERE deleted_at IS NULL
- index `idx_roles_tenant_id` on `roles` (tenant_id)
- index `idx_roles_deleted_at` on `roles` (deleted_at)

#### `identity/000004_create_permissions.up.sql`
- **table `permissions`**
  - `id` UUID — PRIMARY KEY
  - `resource` VARCHAR(100) — NOT NULL
  - `action` VARCHAR(50) — NOT NULL
  - `description` VARCHAR(500)
  - `created_at` TIMESTAMPTZ — NOT NULL DEFAULT CURRENT_TIMESTAMP
  - `updated_at` TIMESTAMPTZ — NOT NULL DEFAULT CURRENT_TIMESTAMP
- UNIQUE index `uq_permissions_resource_action` on `permissions` (resource, action)
- index `idx_permissions_resource` on `permissions` (resource)

#### `identity/000005_create_user_roles.up.sql`
- **table `user_roles`**
  - `id` UUID — PRIMARY KEY
  - `user_id` UUID — NOT NULL REFERENCES users(id) ON DELETE CASCADE
  - `role_id` UUID — NOT NULL REFERENCES roles(id) ON DELETE CASCADE
  - `tenant_id` UUID — NOT NULL REFERENCES tenants(id) ON DELETE CASCADE
  - `assigned_at` TIMESTAMPTZ — NOT NULL DEFAULT CURRENT_TIMESTAMP
  - `assigned_by` UUID — REFERENCES users(id) ON DELETE SET NULL
  - `created_at` TIMESTAMPTZ — NOT NULL DEFAULT CURRENT_TIMESTAMP
  - `updated_at` TIMESTAMPTZ — NOT NULL DEFAULT CURRENT_TIMESTAMP
- UNIQUE index `uq_user_roles_user_role_tenant` on `user_roles` (user_id, role_id, tenant_id)
- index `idx_user_roles_user_id` on `user_roles` (user_id)
- index `idx_user_roles_role_id` on `user_roles` (role_id)
- index `idx_user_roles_tenant_id` on `user_roles` (tenant_id)

#### `identity/000006_create_role_permissions.up.sql`
- **table `role_permissions`**
  - `id` UUID — PRIMARY KEY
  - `role_id` UUID — NOT NULL REFERENCES roles(id) ON DELETE CASCADE
  - `permission_id` UUID — NOT NULL REFERENCES permissions(id) ON DELETE CASCADE
  - `tenant_id` UUID — NOT NULL REFERENCES tenants(id) ON DELETE CASCADE
  - `created_at` TIMESTAMPTZ — NOT NULL DEFAULT CURRENT_TIMESTAMP
  - `updated_at` TIMESTAMPTZ — NOT NULL DEFAULT CURRENT_TIMESTAMP
- UNIQUE index `uq_role_permissions_role_perm_tenant` on `role_permissions` (role_id, permission_id, tenant_id)
- index `idx_role_permissions_role_id` on `role_permissions` (role_id)
- index `idx_role_permissions_permission_id` on `role_permissions` (permission_id)
- index `idx_role_permissions_tenant_id` on `role_permissions` (tenant_id)

#### `identity/000007_create_sessions.up.sql`
- **table `sessions`**
  - `id` UUID — PRIMARY KEY
  - `user_id` UUID — NOT NULL REFERENCES users(id) ON DELETE CASCADE
  - `tenant_id` UUID — NOT NULL REFERENCES tenants(id) ON DELETE CASCADE
  - `ip_address` VARCHAR(45) — NOT NULL
  - `user_agent` VARCHAR(500) — NOT NULL
  - `expires_at` TIMESTAMPTZ — NOT NULL
  - `revoked_at` TIMESTAMPTZ
  - `created_at` TIMESTAMPTZ — NOT NULL DEFAULT CURRENT_TIMESTAMP
  - `updated_at` TIMESTAMPTZ — NOT NULL DEFAULT CURRENT_TIMESTAMP
- index `idx_sessions_user_id` on `sessions` (user_id)
- index `idx_sessions_tenant_id` on `sessions` (tenant_id)
- index `idx_sessions_user_tenant` on `sessions` (user_id, tenant_id)
- index `idx_sessions_expires_at` on `sessions` (expires_at)

#### `identity/000008_create_refresh_tokens.up.sql`
- **table `refresh_tokens`**
  - `id` UUID — PRIMARY KEY
  - `user_id` UUID — NOT NULL REFERENCES users(id) ON DELETE CASCADE
  - `tenant_id` UUID — NOT NULL REFERENCES tenants(id) ON DELETE CASCADE
  - `token_hash` VARCHAR(64) — NOT NULL UNIQUE
  - `expires_at` TIMESTAMPTZ — NOT NULL
  - `revoked_at` TIMESTAMPTZ
  - `created_at` TIMESTAMPTZ — NOT NULL DEFAULT CURRENT_TIMESTAMP
  - `updated_at` TIMESTAMPTZ — NOT NULL DEFAULT CURRENT_TIMESTAMP
- UNIQUE index `uq_refresh_tokens_token_hash` on `refresh_tokens` (token_hash)
- index `idx_refresh_tokens_user_id` on `refresh_tokens` (user_id)
- index `idx_refresh_tokens_tenant_id` on `refresh_tokens` (tenant_id)
- index `idx_refresh_tokens_user_tenant` on `refresh_tokens` (user_id, tenant_id)

#### `identity/000009_create_password_reset_tokens.up.sql`
- **table `password_reset_tokens`**
  - `id` UUID — PRIMARY KEY
  - `user_id` UUID — NOT NULL REFERENCES users(id) ON DELETE CASCADE
  - `tenant_id` UUID — NOT NULL REFERENCES tenants(id) ON DELETE CASCADE
  - `token_hash` VARCHAR(64) — NOT NULL UNIQUE
  - `expires_at` TIMESTAMPTZ — NOT NULL
  - `used_at` TIMESTAMPTZ
  - `created_at` TIMESTAMPTZ — NOT NULL DEFAULT CURRENT_TIMESTAMP
  - `updated_at` TIMESTAMPTZ — NOT NULL DEFAULT CURRENT_TIMESTAMP
- UNIQUE index `uq_password_reset_tokens_token_hash` on `password_reset_tokens` (token_hash)
- index `idx_password_reset_tokens_user_id` on `password_reset_tokens` (user_id)
- index `idx_password_reset_tokens_tenant_id` on `password_reset_tokens` (tenant_id)

#### `identity/000010_create_mfa_configs.up.sql`
- **table `mfa_configs`**
  - `id` UUID — PRIMARY KEY
  - `user_id` UUID — NOT NULL REFERENCES users(id) ON DELETE CASCADE
  - `tenant_id` UUID — NOT NULL REFERENCES tenants(id) ON DELETE CASCADE
  - `type` VARCHAR(20) — NOT NULL
  - `secret` VARCHAR(500) — NOT NULL
  - `is_enabled` BOOLEAN — NOT NULL DEFAULT false
  - `verified_at` TIMESTAMPTZ
  - `created_at` TIMESTAMPTZ — NOT NULL DEFAULT CURRENT_TIMESTAMP
  - `updated_at` TIMESTAMPTZ — NOT NULL DEFAULT CURRENT_TIMESTAMP
- UNIQUE index `uq_mfa_configs_user_type` on `mfa_configs` (user_id, type)
- index `idx_mfa_configs_user_id` on `mfa_configs` (user_id)
- index `idx_mfa_configs_tenant_id` on `mfa_configs` (tenant_id)

#### `identity/000011_create_tenant_settings.up.sql`
- **table `tenant_settings`**
  - `id` UUID — PRIMARY KEY
  - `tenant_id` UUID — NOT NULL REFERENCES tenants(id) ON DELETE CASCADE
  - `key` VARCHAR(255) — NOT NULL
  - `value` TEXT — NOT NULL
  - `created_at` TIMESTAMPTZ — NOT NULL DEFAULT CURRENT_TIMESTAMP
  - `updated_at` TIMESTAMPTZ — NOT NULL DEFAULT CURRENT_TIMESTAMP
- UNIQUE index `uq_tenant_settings_tenant_key` on `tenant_settings` (tenant_id, key)
- index `idx_tenant_settings_tenant_id` on `tenant_settings` (tenant_id)

#### `identity/000012_create_audit_logs.up.sql`
- **table `audit_logs`**
  - `id` UUID — PRIMARY KEY
  - `tenant_id` UUID — NOT NULL REFERENCES tenants(id) ON DELETE CASCADE
  - `user_id` UUID — REFERENCES users(id) ON DELETE SET NULL
  - `action` VARCHAR(100) — NOT NULL
  - `resource` VARCHAR(100) — NOT NULL
  - `resource_id` VARCHAR(36)
  - `metadata` JSONB
  - `ip_address` VARCHAR(45)
  - `user_agent` VARCHAR(500)
  - `created_at` TIMESTAMPTZ — NOT NULL DEFAULT CURRENT_TIMESTAMP
- index `idx_audit_logs_tenant_id` on `audit_logs` (tenant_id)
- index `idx_audit_logs_user_id` on `audit_logs` (user_id)
- index `idx_audit_logs_action` on `audit_logs` (action)
- index `idx_audit_logs_resource` on `audit_logs` (resource)
- index `idx_audit_logs_created_at` on `audit_logs` (created_at)
- index `idx_audit_logs_tenant_created` on `audit_logs` (tenant_id, created_at)
- index `idx_audit_logs_tenant_action` on `audit_logs` (tenant_id, action)

#### `identity/000013_refresh_token_session_link.up.sql`
- index `idx_refresh_tokens_session_id` on `refresh_tokens` (session_id)
- ALTER `refresh_tokens` ADD COLUMN IF NOT EXISTS session_id UUID
- ALTER `refresh_tokens` ADD COLUMN IF NOT EXISTS revoked_reason VARCHAR(20)

#### `identity/000014_access_model.up.sql`
- **table `user_permission_grants`**
  - `id` UUID — PRIMARY KEY
  - `tenant_id` UUID — NOT NULL REFERENCES tenants(id) ON DELETE CASCADE
  - `user_id` UUID — NOT NULL REFERENCES users(id) ON DELETE CASCADE
  - `permission_id` UUID — NOT NULL REFERENCES permissions(id) ON DELETE CASCADE
  - `scope` VARCHAR(20) — NOT NULL
  - _constraint:_ CONSTRAINT chk_user_permission_grants_scope CHECK (scope IN ('own', 'direct_reports', 'all_reports', 'department', 'tenant'))
  - `expires_at` TIMESTAMPTZ
  - `reason` VARCHAR(500)
  - `granted_by` UUID
  - `created_at` TIMESTAMPTZ — NOT NULL DEFAULT CURRENT_TIMESTAMP
  - `updated_at` TIMESTAMPTZ — NOT NULL DEFAULT CURRENT_TIMESTAMP
- **table `user_permission_blocks`**
  - `id` UUID — PRIMARY KEY
  - `tenant_id` UUID — NOT NULL REFERENCES tenants(id) ON DELETE CASCADE
  - `user_id` UUID — NOT NULL REFERENCES users(id) ON DELETE CASCADE
  - `permission_id` UUID — NOT NULL REFERENCES permissions(id) ON DELETE CASCADE
  - `reason` VARCHAR(500)
  - `created_by` UUID
  - `created_at` TIMESTAMPTZ — NOT NULL DEFAULT CURRENT_TIMESTAMP
  - `updated_at` TIMESTAMPTZ — NOT NULL DEFAULT CURRENT_TIMESTAMP
- index `idx_roles_parent_role_id` on `roles` (parent_role_id)
- UNIQUE index `uq_roles_tenant_template_key` on `roles` (tenant_id, template_key) WHERE template_key IS NOT NULL AND deleted_at IS NULL
- UNIQUE index `uq_user_permission_grants` on `user_permission_grants` (tenant_id, user_id, permission_id)
- UNIQUE index `uq_user_permission_blocks` on `user_permission_blocks` (tenant_id, user_id, permission_id)
- ALTER `permissions` ADD COLUMN module VARCHAR(50) NOT NULL DEFAULT ''
- ALTER `permissions` ADD COLUMN is_sensitive BOOLEAN NOT NULL DEFAULT false
- ALTER `roles` ADD COLUMN parent_role_id UUID REFERENCES roles(id) ON DELETE SET NULL
- ALTER `roles` ADD COLUMN template_key VARCHAR(50)
- ALTER `roles` ADD COLUMN is_automatic BOOLEAN NOT NULL DEFAULT false
- ALTER `roles` ADD COLUMN template_managed BOOLEAN NOT NULL DEFAULT false
- ALTER `role_permissions` ADD COLUMN scope VARCHAR(20) NOT NULL DEFAULT 'tenant' CONSTRAINT chk_role_permissions_scope CHECK (scope IN ('own', 'direct_reports', 'all_reports', 'department', 'tenant'))
- ALTER `user_roles` ADD COLUMN source VARCHAR(20) NOT NULL DEFAULT 'manual' CONSTRAINT chk_user_roles_source CHECK (source IN ('manual', 'automatic'))
- ALTER `tenants` ADD COLUMN access_version BIGINT NOT NULL DEFAULT 0

#### `identity/000015_audit_correlation.up.sql`
- index `idx_audit_logs_correlation_id` on `audit_logs` (correlation_id)
- ALTER `audit_logs` ADD COLUMN correlation_id UUID

#### `identity/000016_create_identity_events_outbox.up.sql`
- **table `identity_events_outbox`**
  - `id` UUID — PRIMARY KEY
  - `tenant_id` UUID — NOT NULL
  - `event_id` UUID — NOT NULL
  - `exchange` VARCHAR(100) — NOT NULL
  - `event_type` VARCHAR(100) — NOT NULL
  - `routing_key` VARCHAR(100) — NOT NULL
  - `payload` JSONB — NOT NULL
  - `published` BOOLEAN — NOT NULL DEFAULT FALSE
  - `published_at` TIMESTAMPTZ
  - `attempts` INT — NOT NULL DEFAULT 0
  - `last_error` TEXT
  - `created_at` TIMESTAMPTZ — NOT NULL DEFAULT CURRENT_TIMESTAMP
- UNIQUE index `uq_identity_outbox_event_id` on `identity_events_outbox` (event_id)
- index `idx_identity_outbox_tenant` on `identity_events_outbox` (tenant_id)
- index `idx_identity_outbox_pending` on `identity_events_outbox` (created_at) WHERE published = FALSE

#### `identity/000017_create_idempotency_records.up.sql`
- **table `idempotency_records`**
  - `id` UUID — PRIMARY KEY
  - `scope` VARCHAR(200) — NOT NULL
  - `method` VARCHAR(10) — NOT NULL
  - `path` VARCHAR(500) — NOT NULL
  - `key` VARCHAR(255) — NOT NULL
  - `request_hash` CHAR(64) — NOT NULL
  - `status` VARCHAR(20) — NOT NULL CONSTRAINT chk_idempotency_status CHECK (status IN ('processing', 'completed'))
  - `response_status` INTEGER — NOT NULL DEFAULT 0
  - `response_body` BYTEA
  - `content_type` VARCHAR(100) — NOT NULL DEFAULT ''
  - `created_at` TIMESTAMPTZ — NOT NULL
  - `expires_at` TIMESTAMPTZ — NOT NULL
- UNIQUE index `uq_idempotency_key` on `idempotency_records` (scope, method, path, key)
- index `idx_idempotency_expires` on `idempotency_records` (expires_at)

#### `identity/000018_create_invitations.up.sql`
- **table `invitations`**
  - `id` UUID — PRIMARY KEY
  - `tenant_id` UUID — NOT NULL REFERENCES tenants(id) ON DELETE CASCADE
  - `user_id` UUID — NOT NULL REFERENCES users(id) ON DELETE CASCADE
  - `email` VARCHAR(255) — NOT NULL
  - `token_hash` CHAR(64) — NOT NULL
  - `expires_at` TIMESTAMPTZ — NOT NULL
  - `accepted_at` TIMESTAMPTZ
  - `revoked_at` TIMESTAMPTZ
  - `created_by` UUID
  - `created_at` TIMESTAMPTZ — NOT NULL DEFAULT CURRENT_TIMESTAMP
- UNIQUE index `uq_invitations_token_hash` on `invitations` (token_hash)
- index `idx_invitations_tenant_user` on `invitations` (tenant_id, user_id)

#### `hr/000001_create_departments.up.sql`
- **table `departments`**
  - `id` UUID — PRIMARY KEY
  - `tenant_id` UUID — NOT NULL
  - `name` VARCHAR(100) — NOT NULL
  - `code` VARCHAR(32)
  - `description` VARCHAR(500)
  - `parent_department_id` UUID — REFERENCES departments(id) ON DELETE RESTRICT
  - `status` VARCHAR(20) — NOT NULL DEFAULT 'active'
  - `created_at` TIMESTAMPTZ — NOT NULL DEFAULT CURRENT_TIMESTAMP
  - `updated_at` TIMESTAMPTZ — NOT NULL DEFAULT CURRENT_TIMESTAMP
  - `deleted_at` TIMESTAMPTZ
- UNIQUE index `uq_departments_tenant_name` on `departments` (tenant_id, lower(name)) WHERE deleted_at IS NULL
- UNIQUE index `uq_departments_tenant_code` on `departments` (tenant_id, code) WHERE deleted_at IS NULL AND code IS NOT NULL
- index `idx_departments_tenant_id` on `departments` (tenant_id)
- index `idx_departments_parent_id` on `departments` (parent_department_id)
- index `idx_departments_status` on `departments` (status)
- index `idx_departments_deleted_at` on `departments` (deleted_at)

#### `hr/000002_create_teams.up.sql`
- **table `teams`**
  - `id` UUID — PRIMARY KEY
  - `tenant_id` UUID — NOT NULL
  - `department_id` UUID — NOT NULL REFERENCES departments(id) ON DELETE RESTRICT
  - `name` VARCHAR(100) — NOT NULL
  - `code` VARCHAR(32)
  - `description` VARCHAR(500)
  - `lead_user_id` UUID
  - `status` VARCHAR(20) — NOT NULL DEFAULT 'active'
  - `created_at` TIMESTAMPTZ — NOT NULL DEFAULT CURRENT_TIMESTAMP
  - `updated_at` TIMESTAMPTZ — NOT NULL DEFAULT CURRENT_TIMESTAMP
  - `deleted_at` TIMESTAMPTZ
- UNIQUE index `uq_teams_dept_name` on `teams` (department_id, lower(name)) WHERE deleted_at IS NULL
- UNIQUE index `uq_teams_tenant_code` on `teams` (tenant_id, code) WHERE deleted_at IS NULL AND code IS NOT NULL
- index `idx_teams_tenant_id` on `teams` (tenant_id)
- index `idx_teams_department_id` on `teams` (department_id)
- index `idx_teams_lead_user_id` on `teams` (lead_user_id)
- index `idx_teams_status` on `teams` (status)
- index `idx_teams_deleted_at` on `teams` (deleted_at)

#### `hr/000003_create_designations.up.sql`
- **table `designations`**
  - `id` UUID — PRIMARY KEY
  - `tenant_id` UUID — NOT NULL
  - `title` VARCHAR(100) — NOT NULL
  - `code` VARCHAR(32)
  - `level` INT
  - `description` VARCHAR(500)
  - `status` VARCHAR(20) — NOT NULL DEFAULT 'active'
  - `created_at` TIMESTAMPTZ — NOT NULL DEFAULT CURRENT_TIMESTAMP
  - `updated_at` TIMESTAMPTZ — NOT NULL DEFAULT CURRENT_TIMESTAMP
  - `deleted_at` TIMESTAMPTZ
- UNIQUE index `uq_designations_tenant_title` on `designations` (tenant_id, lower(title)) WHERE deleted_at IS NULL
- UNIQUE index `uq_designations_tenant_code` on `designations` (tenant_id, code) WHERE deleted_at IS NULL AND code IS NOT NULL
- index `idx_designations_tenant_id` on `designations` (tenant_id)
- index `idx_designations_level` on `designations` (level)
- index `idx_designations_status` on `designations` (status)
- index `idx_designations_deleted_at` on `designations` (deleted_at)

#### `hr/000004_create_mappings.up.sql`
- **table `mappings`**
  - `id` UUID — PRIMARY KEY
  - `tenant_id` UUID — NOT NULL
  - `user_id` UUID — NOT NULL
  - `department_id` UUID — REFERENCES departments(id) ON DELETE RESTRICT
  - `team_id` UUID — REFERENCES teams(id) ON DELETE RESTRICT
  - `designation_id` UUID — REFERENCES designations(id) ON DELETE RESTRICT
  - `is_primary` BOOLEAN — NOT NULL DEFAULT FALSE
  - `manager_user_id` UUID
  - `status` VARCHAR(20) — NOT NULL DEFAULT 'active'
  - `reason` VARCHAR(500)
  - `created_at` TIMESTAMPTZ — NOT NULL DEFAULT CURRENT_TIMESTAMP
  - `updated_at` TIMESTAMPTZ — NOT NULL DEFAULT CURRENT_TIMESTAMP
  - `deleted_at` TIMESTAMPTZ
- UNIQUE index `uq_mappings_primary` on `mappings` (tenant_id, user_id) WHERE is_primary = TRUE AND deleted_at IS NULL
- index `idx_mappings_tenant_id` on `mappings` (tenant_id)
- index `idx_mappings_tenant_user` on `mappings` (tenant_id, user_id)
- index `idx_mappings_user_id` on `mappings` (user_id)
- index `idx_mappings_department_id` on `mappings` (department_id)
- index `idx_mappings_team_id` on `mappings` (team_id)
- index `idx_mappings_designation_id` on `mappings` (designation_id)
- index `idx_mappings_manager_user_id` on `mappings` (manager_user_id)
- index `idx_mappings_primary` on `mappings` (is_primary) WHERE is_primary = TRUE
- index `idx_mappings_status` on `mappings` (status)

#### `hr/000005_create_org_events_outbox.up.sql`
- **table `org_events_outbox`**
  - `id` UUID — PRIMARY KEY
  - `tenant_id` UUID — NOT NULL
  - `event_type` VARCHAR(100) — NOT NULL
  - `routing_key` VARCHAR(200) — NOT NULL
  - `payload` JSONB — NOT NULL
  - `correlation_id` UUID
  - `attempts` INT — NOT NULL DEFAULT 0
  - `next_retry_at` TIMESTAMPTZ
  - `created_at` TIMESTAMPTZ — NOT NULL DEFAULT CURRENT_TIMESTAMP
- index `idx_org_outbox_next_retry` on `org_events_outbox` (next_retry_at) WHERE next_retry_at IS NOT NULL
- index `idx_org_outbox_tenant_id` on `org_events_outbox` (tenant_id)

#### `hr/000006_create_employee_profiles.up.sql`
- **table `employee_profiles`**
  - `id` UUID — PRIMARY KEY DEFAULT gen_random_uuid()
  - `tenant_id` UUID — NOT NULL
  - `user_id` UUID — NOT NULL
  - `employee_code` VARCHAR(32) — NOT NULL
  - `first_name` VARCHAR(100) — NOT NULL
  - `last_name` VARCHAR(100) — NOT NULL
  - `display_name` VARCHAR(200)
  - `gender` VARCHAR(20)
  - `date_of_birth` DATE
  - `marital_status` VARCHAR(30)
  - `blood_group` VARCHAR(10)
  - `avatar_url` TEXT
  - `status` VARCHAR(30) — NOT NULL DEFAULT 'active'
  - `created_at` TIMESTAMPTZ — NOT NULL DEFAULT CURRENT_TIMESTAMP
  - `updated_at` TIMESTAMPTZ — NOT NULL DEFAULT CURRENT_TIMESTAMP
  - `deleted_at` TIMESTAMPTZ
- UNIQUE index `idx_emp_profile_tenant_user` on `employee_profiles` (tenant_id, user_id)
- UNIQUE index `idx_emp_profile_tenant_code` on `employee_profiles` (tenant_id, LOWER(employee_code))
- index `idx_emp_profile_status` on `employee_profiles` (tenant_id, status)
- index `idx_emp_profile_deleted_at` on `employee_profiles` (deleted_at)

#### `hr/000007_create_employment_details.up.sql`
- **table `employment_details`**
  - `id` UUID — PRIMARY KEY DEFAULT gen_random_uuid()
  - `tenant_id` UUID — NOT NULL
  - `employee_profile_id` UUID — NOT NULL REFERENCES employee_profiles(id) ON DELETE CASCADE
  - `employment_type` VARCHAR(30) — NOT NULL DEFAULT 'full_time'
  - `joining_date` DATE — NOT NULL
  - `probation_end_date` DATE
  - `confirmation_date` DATE
  - `notice_period_days` INT — NOT NULL DEFAULT 30
  - `resignation_date` DATE
  - `exit_date` DATE
  - `exit_reason` TEXT
  - `created_at` TIMESTAMPTZ — NOT NULL DEFAULT CURRENT_TIMESTAMP
  - `updated_at` TIMESTAMPTZ — NOT NULL DEFAULT CURRENT_TIMESTAMP
  - `deleted_at` TIMESTAMPTZ
- UNIQUE index `idx_emp_detail_tenant_profile` on `employment_details` (tenant_id, employee_profile_id)
- index `idx_emp_detail_deleted_at` on `employment_details` (deleted_at)

#### `hr/000008_create_employee_contacts.up.sql`
- **table `employee_contacts`**
  - `id` UUID — PRIMARY KEY DEFAULT gen_random_uuid()
  - `tenant_id` UUID — NOT NULL
  - `employee_profile_id` UUID — NOT NULL REFERENCES employee_profiles(id) ON DELETE CASCADE
  - `personal_email` VARCHAR(255)
  - `work_phone` VARCHAR(50)
  - `personal_phone` VARCHAR(50)
  - `current_address` TEXT
  - `permanent_address` TEXT
  - `emergency_contacts` JSONB — DEFAULT '[]'::jsonb
  - `created_at` TIMESTAMPTZ — NOT NULL DEFAULT CURRENT_TIMESTAMP
  - `updated_at` TIMESTAMPTZ — NOT NULL DEFAULT CURRENT_TIMESTAMP
  - `deleted_at` TIMESTAMPTZ
- UNIQUE index `idx_emp_contact_tenant_profile` on `employee_contacts` (tenant_id, employee_profile_id)
- index `idx_emp_contact_deleted_at` on `employee_contacts` (deleted_at)

#### `hr/000009_create_employee_statutory.up.sql`
- **table `employee_statutory`**
  - `id` UUID — PRIMARY KEY DEFAULT gen_random_uuid()
  - `tenant_id` UUID — NOT NULL
  - `employee_profile_id` UUID — NOT NULL REFERENCES employee_profiles(id) ON DELETE CASCADE
  - `tax_id` VARCHAR(100)
  - `national_id` VARCHAR(100)
  - `bank_name` VARCHAR(100)
  - `bank_account_number` VARCHAR(100)
  - `bank_routing_swift` VARCHAR(100)
  - `created_at` TIMESTAMPTZ — NOT NULL DEFAULT CURRENT_TIMESTAMP
  - `updated_at` TIMESTAMPTZ — NOT NULL DEFAULT CURRENT_TIMESTAMP
  - `deleted_at` TIMESTAMPTZ
- UNIQUE index `uq_emp_statutory_profile` on `employee_statutory` (employee_profile_id)
- index `idx_emp_statutory_deleted_at` on `employee_statutory` (deleted_at)

#### `hr/000010_create_employee_documents.up.sql`
- **table `employee_documents`**
  - `id` UUID — PRIMARY KEY DEFAULT gen_random_uuid()
  - `tenant_id` UUID — NOT NULL
  - `employee_profile_id` UUID — NOT NULL REFERENCES employee_profiles(id) ON DELETE CASCADE
  - `document_type` VARCHAR(50) — NOT NULL
  - `file_name` VARCHAR(255) — NOT NULL
  - `file_url` TEXT — NOT NULL
  - `file_size` BIGINT — NOT NULL
  - `mime_type` VARCHAR(100) — NOT NULL
  - `verified_at` TIMESTAMPTZ
  - `verified_by` UUID
  - `created_at` TIMESTAMPTZ — NOT NULL DEFAULT CURRENT_TIMESTAMP
  - `updated_at` TIMESTAMPTZ — NOT NULL DEFAULT CURRENT_TIMESTAMP
  - `deleted_at` TIMESTAMPTZ
- index `idx_emp_docs_tenant_profile` on `employee_documents` (tenant_id, employee_profile_id)
- index `idx_emp_docs_deleted_at` on `employee_documents` (deleted_at)

#### `hr/000011_create_employee_timelines.up.sql`
- **table `employee_timelines`**
  - `id` UUID — PRIMARY KEY DEFAULT gen_random_uuid()
  - `tenant_id` UUID — NOT NULL
  - `employee_profile_id` UUID — NOT NULL REFERENCES employee_profiles(id) ON DELETE CASCADE
  - `event_type` VARCHAR(50) — NOT NULL
  - `effective_date` DATE — NOT NULL
  - `notes` TEXT
  - `metadata` JSONB — DEFAULT '{}'::jsonb
  - `created_at` TIMESTAMPTZ — NOT NULL DEFAULT CURRENT_TIMESTAMP
  - `updated_at` TIMESTAMPTZ — NOT NULL DEFAULT CURRENT_TIMESTAMP
  - `deleted_at` TIMESTAMPTZ
- index `idx_emp_timelines_tenant_profile` on `employee_timelines` (tenant_id, employee_profile_id)
- index `idx_emp_timelines_effective` on `employee_timelines` (tenant_id, effective_date)

#### `hr/000012_create_employee_events_outbox.up.sql`
- **table `employee_events_outbox`**
  - `id` UUID — PRIMARY KEY DEFAULT gen_random_uuid()
  - `tenant_id` UUID — NOT NULL
  - `event_id` UUID — NOT NULL UNIQUE
  - `event_type` VARCHAR(100) — NOT NULL
  - `routing_key` VARCHAR(100) — NOT NULL
  - `payload` JSONB — NOT NULL
  - `published` BOOLEAN — NOT NULL DEFAULT FALSE
  - `published_at` TIMESTAMPTZ
  - `retry_count` INT — NOT NULL DEFAULT 0
  - `last_error` TEXT
  - `created_at` TIMESTAMPTZ — NOT NULL DEFAULT CURRENT_TIMESTAMP
  - `updated_at` TIMESTAMPTZ — NOT NULL DEFAULT CURRENT_TIMESTAMP
  - `deleted_at` TIMESTAMPTZ
- index `idx_emp_outbox_published` on `employee_events_outbox` (published)

#### `hr/000013_create_shifts.up.sql`
- **table `shifts`**
  - `id` UUID — PRIMARY KEY
  - `tenant_id` UUID — NOT NULL
  - `name` VARCHAR(100) — NOT NULL
  - `code` VARCHAR(32)
  - `start_minute` INT — NOT NULL CHECK (start_minute BETWEEN 0 AND 1439)
  - `end_minute` INT — NOT NULL CHECK (end_minute BETWEEN 0 AND 1439)
  - `grace_period_mins` INT — NOT NULL DEFAULT 15 CHECK (grace_period_mins BETWEEN 0 AND 240)
  - `break_duration_mins` INT — NOT NULL DEFAULT 60 CHECK (break_duration_mins BETWEEN 0 AND 480)
  - `full_day_minutes` INT — CHECK (full_day_minutes BETWEEN 1 AND 1440)
  - `half_day_minutes` INT — CHECK (half_day_minutes BETWEEN 1 AND 1440)
  - `timezone` VARCHAR(64) — NOT NULL DEFAULT 'UTC'
  - `is_night_shift` BOOLEAN — NOT NULL DEFAULT FALSE
  - `status` VARCHAR(20) — NOT NULL DEFAULT 'active'
  - `created_at` TIMESTAMPTZ — NOT NULL DEFAULT CURRENT_TIMESTAMP
  - `updated_at` TIMESTAMPTZ — NOT NULL DEFAULT CURRENT_TIMESTAMP
  - `deleted_at` TIMESTAMPTZ
  - _constraint:_ CONSTRAINT chk_shifts_start_ne_end CHECK (start_minute <> end_minute)
- UNIQUE index `uq_shifts_tenant_name` on `shifts` (tenant_id, lower(name)) WHERE deleted_at IS NULL
- UNIQUE index `uq_shifts_tenant_code` on `shifts` (tenant_id, code) WHERE deleted_at IS NULL AND code IS NOT NULL
- index `idx_shifts_tenant_id` on `shifts` (tenant_id)
- index `idx_shifts_status` on `shifts` (status)
- index `idx_shifts_deleted_at` on `shifts` (deleted_at)

#### `hr/000014_create_shift_assignments.up.sql`
- **table `shift_assignments`**
  - `id` UUID — PRIMARY KEY
  - `tenant_id` UUID — NOT NULL
  - `employee_profile_id` UUID — NOT NULL REFERENCES employee_profiles(id) ON DELETE CASCADE
  - `shift_id` UUID — NOT NULL REFERENCES shifts(id) ON DELETE RESTRICT
  - `effective_from` DATE — NOT NULL
  - `effective_to` DATE
  - `created_by_user_id` UUID
  - `created_at` TIMESTAMPTZ — NOT NULL DEFAULT CURRENT_TIMESTAMP
  - `updated_at` TIMESTAMPTZ — NOT NULL DEFAULT CURRENT_TIMESTAMP
  - _constraint:_ CONSTRAINT chk_shift_assignments_range CHECK (effective_to IS NULL OR effective_to >= effective_from)
- index `idx_shift_assignments_employee` on `shift_assignments` (tenant_id, employee_profile_id, effective_from)
- index `idx_shift_assignments_shift` on `shift_assignments` (shift_id)

#### `hr/000015_create_attendance_records.up.sql`
- **table `attendance_records`**
  - `id` UUID — PRIMARY KEY
  - `tenant_id` UUID — NOT NULL
  - `employee_profile_id` UUID — NOT NULL REFERENCES employee_profiles(id) ON DELETE CASCADE
  - `attendance_date` DATE — NOT NULL
  - `shift_id` UUID — REFERENCES shifts(id) ON DELETE SET NULL
  - `first_punch_in` TIMESTAMPTZ
  - `last_punch_out` TIMESTAMPTZ
  - `total_work_minutes` INT — NOT NULL DEFAULT 0
  - `total_break_minutes` INT — NOT NULL DEFAULT 0
  - `late_minutes` INT — NOT NULL DEFAULT 0
  - `overtime_minutes` INT — NOT NULL DEFAULT 0
  - `status` VARCHAR(20) — NOT NULL
  - `source` VARCHAR(20) — NOT NULL DEFAULT 'web'
  - `is_regularized` BOOLEAN — NOT NULL DEFAULT FALSE
  - `created_at` TIMESTAMPTZ — NOT NULL DEFAULT CURRENT_TIMESTAMP
  - `updated_at` TIMESTAMPTZ — NOT NULL DEFAULT CURRENT_TIMESTAMP
- UNIQUE index `uq_attendance_records_employee_date` on `attendance_records` (tenant_id, employee_profile_id, attendance_date)
- index `idx_attendance_records_tenant_date_status` on `attendance_records` (tenant_id, attendance_date, status)

#### `hr/000016_create_attendance_punches.up.sql`
- **table `attendance_punches`**
  - `id` UUID — PRIMARY KEY
  - `tenant_id` UUID — NOT NULL
  - `attendance_record_id` UUID — NOT NULL REFERENCES attendance_records(id) ON DELETE CASCADE
  - `employee_profile_id` UUID — NOT NULL REFERENCES employee_profiles(id) ON DELETE CASCADE
  - `punch_time` TIMESTAMPTZ — NOT NULL
  - `punch_type` VARCHAR(10) — NOT NULL CHECK (punch_type IN ('in', 'out'))
  - `source` VARCHAR(20) — NOT NULL
  - `latitude` NUMERIC(10, 8) — CHECK (latitude BETWEEN -90 AND 90)
  - `longitude` NUMERIC(11, 8) — CHECK (longitude BETWEEN -180 AND 180)
  - `device_id` VARCHAR(100)
  - `ip_address` VARCHAR(45)
  - `is_superseded` BOOLEAN — NOT NULL DEFAULT FALSE
  - `created_at` TIMESTAMPTZ — NOT NULL DEFAULT CURRENT_TIMESTAMP
- index `idx_attendance_punches_employee_time` on `attendance_punches` (tenant_id, employee_profile_id, punch_time)
- index `idx_attendance_punches_record` on `attendance_punches` (attendance_record_id)

#### `hr/000017_create_attendance_regularizations.up.sql`
- **table `attendance_regularizations`**
  - `id` UUID — PRIMARY KEY
  - `tenant_id` UUID — NOT NULL
  - `employee_profile_id` UUID — NOT NULL REFERENCES employee_profiles(id) ON DELETE CASCADE
  - `attendance_date` DATE — NOT NULL
  - `attendance_record_id` UUID — REFERENCES attendance_records(id) ON DELETE SET NULL
  - `requested_punch_in` TIMESTAMPTZ — NOT NULL
  - `requested_punch_out` TIMESTAMPTZ — NOT NULL
  - `reason` VARCHAR(500) — NOT NULL
  - `status` VARCHAR(20) — NOT NULL DEFAULT 'pending'
  - `requested_by_user_id` UUID — NOT NULL
  - `reviewer_user_id` UUID
  - `reviewed_at` TIMESTAMPTZ
  - `review_comment` VARCHAR(500)
  - `created_at` TIMESTAMPTZ — NOT NULL DEFAULT CURRENT_TIMESTAMP
  - `updated_at` TIMESTAMPTZ — NOT NULL DEFAULT CURRENT_TIMESTAMP
  - _constraint:_ CONSTRAINT chk_attendance_regularizations_window CHECK (requested_punch_out > requested_punch_in)
- UNIQUE index `uq_attendance_regularizations_pending` on `attendance_regularizations` (tenant_id, employee_profile_id, attendance_date) WHERE status = 'pending'
- index `idx_attendance_regularizations_employee_date` on `attendance_regularizations` (tenant_id, employee_profile_id, attendance_date)
- index `idx_attendance_regularizations_tenant_status` on `attendance_regularizations` (tenant_id, status)

#### `hr/000018_create_attendance_events_outbox.up.sql`
- **table `attendance_events_outbox`**
  - `id` UUID — PRIMARY KEY
  - `tenant_id` UUID — NOT NULL
  - `event_id` UUID — NOT NULL
  - `event_type` VARCHAR(100) — NOT NULL
  - `routing_key` VARCHAR(100) — NOT NULL
  - `payload` JSONB — NOT NULL
  - `published` BOOLEAN — NOT NULL DEFAULT FALSE
  - `published_at` TIMESTAMPTZ
  - `attempts` INT — NOT NULL DEFAULT 0
  - `last_error` TEXT
  - `created_at` TIMESTAMPTZ — NOT NULL DEFAULT CURRENT_TIMESTAMP
- UNIQUE index `uq_attendance_outbox_event_id` on `attendance_events_outbox` (event_id)
- index `idx_attendance_outbox_tenant` on `attendance_events_outbox` (tenant_id)
- index `idx_attendance_outbox_published` on `attendance_events_outbox` (published) WHERE published = FALSE

#### `hr/000019_create_leave_types.up.sql`
- **table `leave_types`**
  - `id` UUID — PRIMARY KEY
  - `tenant_id` UUID — NOT NULL
  - `name` VARCHAR(100) — NOT NULL
  - `code` VARCHAR(10) — NOT NULL CHECK (code ~ '^[A-Z0-9_]{2,10}$')
  - `is_paid` BOOLEAN — NOT NULL DEFAULT TRUE
  - `annual_allowance` NUMERIC(7,2) — NOT NULL CHECK (annual_allowance BETWEEN 0 AND 365)
  - `accrual` VARCHAR(10) — NOT NULL DEFAULT 'annual' CHECK (accrual IN ('annual', 'monthly'))
  - `carry_forward_limit` NUMERIC(7,2) — NOT NULL DEFAULT 0 CHECK (carry_forward_limit BETWEEN 0 AND 365)
  - `max_consecutive_days` INT — CHECK (max_consecutive_days BETWEEN 1 AND 365)
  - `min_notice_days` INT — NOT NULL DEFAULT 0 CHECK (min_notice_days BETWEEN 0 AND 90)
  - `allow_half_day` BOOLEAN — NOT NULL DEFAULT TRUE
  - `sandwich_rule` BOOLEAN — NOT NULL DEFAULT FALSE
  - `applicable_gender` VARCHAR(10) — NOT NULL DEFAULT 'all' CHECK (applicable_gender IN ('all', 'male', 'female'))
  - `status` VARCHAR(20) — NOT NULL DEFAULT 'active'
  - `created_at` TIMESTAMPTZ — NOT NULL DEFAULT CURRENT_TIMESTAMP
  - `updated_at` TIMESTAMPTZ — NOT NULL DEFAULT CURRENT_TIMESTAMP
  - `deleted_at` TIMESTAMPTZ
- UNIQUE index `uq_leave_types_tenant_name` on `leave_types` (tenant_id, lower(name)) WHERE deleted_at IS NULL
- UNIQUE index `uq_leave_types_tenant_code` on `leave_types` (tenant_id, code) WHERE deleted_at IS NULL
- index `idx_leave_types_tenant_id` on `leave_types` (tenant_id)
- index `idx_leave_types_status` on `leave_types` (status)
- index `idx_leave_types_deleted_at` on `leave_types` (deleted_at)

#### `hr/000020_create_leave_holidays.up.sql`
- **table `leave_holidays`**
  - `id` UUID — PRIMARY KEY
  - `tenant_id` UUID — NOT NULL
  - `holiday_date` DATE — NOT NULL
  - `name` VARCHAR(100) — NOT NULL
  - `is_optional` BOOLEAN — NOT NULL DEFAULT FALSE
  - `created_at` TIMESTAMPTZ — NOT NULL DEFAULT CURRENT_TIMESTAMP
  - `updated_at` TIMESTAMPTZ — NOT NULL DEFAULT CURRENT_TIMESTAMP
  - `deleted_at` TIMESTAMPTZ
- UNIQUE index `uq_leave_holidays_tenant_date` on `leave_holidays` (tenant_id, holiday_date) WHERE deleted_at IS NULL
- index `idx_leave_holidays_deleted_at` on `leave_holidays` (deleted_at)

#### `hr/000021_create_leave_balances.up.sql`
- **table `leave_balances`**
  - `id` UUID — PRIMARY KEY
  - `tenant_id` UUID — NOT NULL
  - `employee_profile_id` UUID — NOT NULL REFERENCES employee_profiles(id) ON DELETE CASCADE
  - `leave_type_id` UUID — NOT NULL REFERENCES leave_types(id) ON DELETE CASCADE
  - `year` INT — NOT NULL CHECK (year BETWEEN 2000 AND 2200)
  - `opening` NUMERIC(7,2) — NOT NULL DEFAULT 0
  - `accrued` NUMERIC(7,2) — NOT NULL DEFAULT 0
  - `adjusted` NUMERIC(7,2) — NOT NULL DEFAULT 0
  - `used` NUMERIC(7,2) — NOT NULL DEFAULT 0
  - `reserved` NUMERIC(7,2) — NOT NULL DEFAULT 0 CHECK (reserved >= 0)
  - `created_at` TIMESTAMPTZ — NOT NULL DEFAULT CURRENT_TIMESTAMP
  - `updated_at` TIMESTAMPTZ — NOT NULL DEFAULT CURRENT_TIMESTAMP
- UNIQUE index `uq_leave_balances_employee_type_year` on `leave_balances` (tenant_id, employee_profile_id, leave_type_id, year)

#### `hr/000022_create_leave_requests.up.sql`
- **table `leave_requests`**
  - `id` UUID — PRIMARY KEY
  - `tenant_id` UUID — NOT NULL
  - `employee_profile_id` UUID — NOT NULL REFERENCES employee_profiles(id) ON DELETE CASCADE
  - `leave_type_id` UUID — NOT NULL REFERENCES leave_types(id)
  - `start_date` DATE — NOT NULL
  - `end_date` DATE — NOT NULL
  - `half_day` VARCHAR(12) — CHECK (half_day IN ('first_half', 'second_half'))
  - `total_days` NUMERIC(7,2) — NOT NULL CHECK (total_days > 0)
  - `reason` TEXT — NOT NULL
  - `status` VARCHAR(20) — NOT NULL DEFAULT 'pending' CHECK (status IN ('pending', 'approved', 'rejected', 'cancelled'))
  - `requested_by_user_id` UUID — NOT NULL
  - `reviewer_user_id` UUID
  - `reviewed_at` TIMESTAMPTZ
  - `review_comment` VARCHAR(500)
  - `cancelled_at` TIMESTAMPTZ
  - `created_at` TIMESTAMPTZ — NOT NULL DEFAULT CURRENT_TIMESTAMP
  - `updated_at` TIMESTAMPTZ — NOT NULL DEFAULT CURRENT_TIMESTAMP
  - _constraint:_ CONSTRAINT chk_leave_requests_range CHECK (start_date <= end_date)
  - _constraint:_ CONSTRAINT chk_leave_requests_half_single CHECK (half_day IS NULL OR start_date = end_date)
- index `idx_leave_requests_employee_start` on `leave_requests` (tenant_id, employee_profile_id, start_date)
- index `idx_leave_requests_tenant_status` on `leave_requests` (tenant_id, status)

#### `hr/000023_create_leave_ledger.up.sql`
- **table `leave_ledger`**
  - `id` UUID — PRIMARY KEY
  - `tenant_id` UUID — NOT NULL
  - `employee_profile_id` UUID — NOT NULL REFERENCES employee_profiles(id) ON DELETE CASCADE
  - `leave_type_id` UUID — NOT NULL REFERENCES leave_types(id) ON DELETE CASCADE
  - `year` INT — NOT NULL
  - `kind` VARCHAR(20) — NOT NULL CHECK (kind IN ('carry_forward', 'accrual', 'adjustment', 'reserve', 'release', 'consume', 'reversal'))
  - `days` NUMERIC(7,2) — NOT NULL CHECK (days <> 0)
  - `leave_request_id` UUID — REFERENCES leave_requests(id)
  - `actor_user_id` UUID
  - `note` VARCHAR(500)
  - `created_at` TIMESTAMPTZ — NOT NULL DEFAULT CURRENT_TIMESTAMP
- index `idx_leave_ledger_balance` on `leave_ledger` (tenant_id, employee_profile_id, leave_type_id, year)

#### `hr/000024_create_leave_events_outbox.up.sql`
- **table `leave_events_outbox`**
  - `id` UUID — PRIMARY KEY
  - `tenant_id` UUID — NOT NULL
  - `event_id` UUID — NOT NULL
  - `event_type` VARCHAR(100) — NOT NULL
  - `routing_key` VARCHAR(100) — NOT NULL
  - `payload` JSONB — NOT NULL
  - `published` BOOLEAN — NOT NULL DEFAULT FALSE
  - `published_at` TIMESTAMPTZ
  - `attempts` INT — NOT NULL DEFAULT 0
  - `last_error` TEXT
  - `created_at` TIMESTAMPTZ — NOT NULL DEFAULT CURRENT_TIMESTAMP
- UNIQUE index `uq_leave_outbox_event_id` on `leave_events_outbox` (event_id)
- index `idx_leave_outbox_tenant` on `leave_events_outbox` (tenant_id)
- index `idx_leave_outbox_published` on `leave_events_outbox` (published) WHERE published = FALSE

#### `hr/000025_create_payroll_structures.up.sql`
- **table `payroll_structures`**
  - `id` UUID — PRIMARY KEY
  - `tenant_id` UUID — NOT NULL
  - `name` VARCHAR(100) — NOT NULL
  - `description` VARCHAR(500)
  - `pf_enabled` BOOLEAN — NOT NULL DEFAULT TRUE
  - `esi_enabled` BOOLEAN — NOT NULL DEFAULT TRUE
  - `pt_enabled` BOOLEAN — NOT NULL DEFAULT TRUE
  - `tds_enabled` BOOLEAN — NOT NULL DEFAULT TRUE
  - `status` VARCHAR(20) — NOT NULL DEFAULT 'active' CHECK (status IN ('active', 'inactive'))
  - `created_at` TIMESTAMPTZ — NOT NULL DEFAULT CURRENT_TIMESTAMP
  - `updated_at` TIMESTAMPTZ — NOT NULL DEFAULT CURRENT_TIMESTAMP
  - `deleted_at` TIMESTAMPTZ
- UNIQUE index `uq_payroll_structures_tenant_name` on `payroll_structures` (tenant_id, lower(name)) WHERE deleted_at IS NULL
- index `idx_payroll_structures_tenant_id` on `payroll_structures` (tenant_id)
- index `idx_payroll_structures_deleted_at` on `payroll_structures` (deleted_at)

#### `hr/000026_create_payroll_components.up.sql`
- **table `payroll_components`**
  - `id` UUID — PRIMARY KEY
  - `tenant_id` UUID — NOT NULL
  - `structure_id` UUID — NOT NULL REFERENCES payroll_structures(id) ON DELETE CASCADE
  - `code` VARCHAR(20) — NOT NULL CHECK (code ~ '^[A-Z0-9_]{2,20}$')
  - `name` VARCHAR(100) — NOT NULL
  - `kind` VARCHAR(10) — NOT NULL CHECK (kind IN ('earning', 'deduction'))
  - `calc` VARCHAR(20) — NOT NULL CHECK (calc IN ('fixed', 'percent_of_ctc', 'percent_of_basic', 'balance'))
  - `value` NUMERIC(14,2) — NOT NULL DEFAULT 0 CHECK (value >= 0)
  - `taxable` BOOLEAN — NOT NULL DEFAULT TRUE
  - `position` INT — NOT NULL
- UNIQUE index `uq_payroll_components_structure_code` on `payroll_components` (structure_id, code)

#### `hr/000027_create_payroll_assignments.up.sql`
- **table `payroll_assignments`**
  - `id` UUID — PRIMARY KEY
  - `tenant_id` UUID — NOT NULL
  - `employee_profile_id` UUID — NOT NULL REFERENCES employee_profiles(id) ON DELETE CASCADE
  - `structure_id` UUID — NOT NULL REFERENCES payroll_structures(id)
  - `annual_ctc` NUMERIC(14,2) — NOT NULL CHECK (annual_ctc > 0)
  - `effective_from` DATE — NOT NULL
  - `effective_to` DATE
  - `created_by_user_id` UUID — NOT NULL
  - `created_at` TIMESTAMPTZ — NOT NULL DEFAULT CURRENT_TIMESTAMP
  - `updated_at` TIMESTAMPTZ — NOT NULL DEFAULT CURRENT_TIMESTAMP
  - _constraint:_ CONSTRAINT chk_payroll_assignments_range CHECK (effective_to IS NULL OR effective_to >= effective_from)
- index `idx_payroll_assignments_employee` on `payroll_assignments` (tenant_id, employee_profile_id, effective_from)

#### `hr/000028_create_payroll_runs.up.sql`
- **table `payroll_runs`**
  - `id` UUID — PRIMARY KEY
  - `tenant_id` UUID — NOT NULL
  - `year` INT — NOT NULL CHECK (year BETWEEN 2000 AND 2200)
  - `month` INT — NOT NULL CHECK (month BETWEEN 1 AND 12)
  - `status` VARCHAR(20) — NOT NULL DEFAULT 'draft' CHECK (status IN ('draft', 'calculated', 'approved', 'finalized'))
  - `employee_count` INT — NOT NULL DEFAULT 0
  - `gross_total` NUMERIC(14,2) — NOT NULL DEFAULT 0
  - `deduction_total` NUMERIC(14,2) — NOT NULL DEFAULT 0
  - `net_total` NUMERIC(14,2) — NOT NULL DEFAULT 0
  - `warnings` JSONB — NOT NULL DEFAULT '[]'
  - `created_by_user_id` UUID — NOT NULL
  - `calculated_by_user_id` UUID
  - `approved_by_user_id` UUID
  - `calculated_at` TIMESTAMPTZ
  - `approved_at` TIMESTAMPTZ
  - `finalized_at` TIMESTAMPTZ
  - `created_at` TIMESTAMPTZ — NOT NULL DEFAULT CURRENT_TIMESTAMP
  - `updated_at` TIMESTAMPTZ — NOT NULL DEFAULT CURRENT_TIMESTAMP
- UNIQUE index `uq_payroll_runs_tenant_period` on `payroll_runs` (tenant_id, year, month)

#### `hr/000029_create_payroll_payslips.up.sql`
- **table `payroll_payslips`**
  - `id` UUID — PRIMARY KEY
  - `tenant_id` UUID — NOT NULL
  - `run_id` UUID — NOT NULL REFERENCES payroll_runs(id) ON DELETE CASCADE
  - `employee_profile_id` UUID — NOT NULL REFERENCES employee_profiles(id) ON DELETE CASCADE
  - `employee_code` VARCHAR(50) — NOT NULL
  - `employee_name` VARCHAR(200) — NOT NULL
  - `structure_id` UUID — NOT NULL REFERENCES payroll_structures(id)
  - `annual_ctc` NUMERIC(14,2) — NOT NULL
  - `days_in_month` INT — NOT NULL CHECK (days_in_month BETWEEN 28 AND 31)
  - `payable_days` NUMERIC(7,2) — NOT NULL CHECK (payable_days >= 0)
  - `lop_days` NUMERIC(7,2) — NOT NULL DEFAULT 0 CHECK (lop_days >= 0)
  - `gross` NUMERIC(14,2) — NOT NULL
  - `deductions` NUMERIC(14,2) — NOT NULL
  - `net` NUMERIC(14,2) — NOT NULL CHECK (net >= 0)
  - `created_at` TIMESTAMPTZ — NOT NULL DEFAULT CURRENT_TIMESTAMP
- UNIQUE index `uq_payroll_payslips_run_employee` on `payroll_payslips` (run_id, employee_profile_id)
- index `idx_payroll_payslips_employee` on `payroll_payslips` (tenant_id, employee_profile_id)

#### `hr/000030_create_payroll_payslip_lines.up.sql`
- **table `payroll_payslip_lines`**
  - `id` UUID — PRIMARY KEY
  - `payslip_id` UUID — NOT NULL REFERENCES payroll_payslips(id) ON DELETE CASCADE
  - `code` VARCHAR(20) — NOT NULL
  - `name` VARCHAR(100) — NOT NULL
  - `kind` VARCHAR(10) — NOT NULL CHECK (kind IN ('earning', 'deduction'))
  - `amount` NUMERIC(14,2) — NOT NULL CHECK (amount >= 0)
  - `position` INT — NOT NULL
- index `idx_payroll_payslip_lines_payslip` on `payroll_payslip_lines` (payslip_id)

#### `hr/000031_create_payroll_events_outbox.up.sql`
- **table `payroll_events_outbox`**
  - `id` UUID — PRIMARY KEY
  - `tenant_id` UUID — NOT NULL
  - `event_id` UUID — NOT NULL
  - `event_type` VARCHAR(100) — NOT NULL
  - `routing_key` VARCHAR(100) — NOT NULL
  - `payload` JSONB — NOT NULL
  - `published` BOOLEAN — NOT NULL DEFAULT FALSE
  - `published_at` TIMESTAMPTZ
  - `attempts` INT — NOT NULL DEFAULT 0
  - `last_error` TEXT
  - `created_at` TIMESTAMPTZ — NOT NULL DEFAULT CURRENT_TIMESTAMP
- UNIQUE index `uq_payroll_outbox_event_id` on `payroll_events_outbox` (event_id)
- index `idx_payroll_outbox_tenant` on `payroll_events_outbox` (tenant_id)
- index `idx_payroll_outbox_published` on `payroll_events_outbox` (published) WHERE published = FALSE

#### `hr/000032_create_recruitment_jobs.up.sql`
- **table `recruitment_jobs`**
  - `id` UUID — PRIMARY KEY
  - `tenant_id` UUID — NOT NULL
  - `title` VARCHAR(150) — NOT NULL
  - `department_id` UUID
  - `designation_id` UUID
  - `headcount` INT — NOT NULL CHECK (headcount BETWEEN 1 AND 1000)
  - `hired_count` INT — NOT NULL DEFAULT 0 CHECK (hired_count >= 0 AND hired_count <= headcount)
  - `location` VARCHAR(100)
  - `employment_type` VARCHAR(20) — NOT NULL CHECK (employment_type IN ('full_time', 'part_time', 'contract', 'intern'))
  - `min_experience_years` INT — NOT NULL DEFAULT 0 CHECK (min_experience_years BETWEEN 0 AND 50)
  - `description` TEXT — NOT NULL
  - `status` VARCHAR(20) — NOT NULL DEFAULT 'draft' CHECK (status IN ('draft', 'open', 'on_hold', 'closed', 'filled'))
  - `opened_at` TIMESTAMPTZ
  - `closed_at` TIMESTAMPTZ
  - `created_by_user_id` UUID — NOT NULL
  - `created_at` TIMESTAMPTZ — NOT NULL DEFAULT CURRENT_TIMESTAMP
  - `updated_at` TIMESTAMPTZ — NOT NULL DEFAULT CURRENT_TIMESTAMP
- index `idx_recruitment_jobs_tenant_status` on `recruitment_jobs` (tenant_id, status)

#### `hr/000033_create_recruitment_candidates.up.sql`
- **table `recruitment_candidates`**
  - `id` UUID — PRIMARY KEY
  - `tenant_id` UUID — NOT NULL
  - `job_id` UUID — NOT NULL REFERENCES recruitment_jobs(id) ON DELETE CASCADE
  - `first_name` VARCHAR(100) — NOT NULL
  - `last_name` VARCHAR(100) — NOT NULL
  - `email` VARCHAR(255) — NOT NULL
  - `phone` VARCHAR(30)
  - `source` VARCHAR(20) — NOT NULL CHECK (source IN ('career_site', 'referral', 'agency', 'linkedin', 'other'))
  - `resume_url` VARCHAR(500)
  - `expected_ctc` NUMERIC(14,2) — CHECK (expected_ctc IS NULL OR expected_ctc >= 0)
  - `notice_period_days` INT — CHECK (notice_period_days IS NULL OR notice_period_days BETWEEN 0 AND 365)
  - `stage` VARCHAR(20) — NOT NULL DEFAULT 'applied' CHECK (stage IN ('applied', 'screening', 'interview', 'offer', 'hired', 'rejected', 'withdrawn'))
  - `hired_user_id` UUID
  - `hired_employee_id` UUID
  - `created_at` TIMESTAMPTZ — NOT NULL DEFAULT CURRENT_TIMESTAMP
  - `updated_at` TIMESTAMPTZ — NOT NULL DEFAULT CURRENT_TIMESTAMP
- UNIQUE index `uq_recruitment_candidates_job_email` on `recruitment_candidates` (job_id, lower(email))
- index `idx_recruitment_candidates_job_stage` on `recruitment_candidates` (tenant_id, job_id, stage)

#### `hr/000034_create_recruitment_stage_events.up.sql`
- **table `recruitment_stage_events`**
  - `id` UUID — PRIMARY KEY
  - `tenant_id` UUID — NOT NULL
  - `candidate_id` UUID — NOT NULL REFERENCES recruitment_candidates(id) ON DELETE CASCADE
  - `from_stage` VARCHAR(20)
  - `to_stage` VARCHAR(20) — NOT NULL
  - `actor_user_id` UUID — NOT NULL
  - `note` VARCHAR(500)
  - `created_at` TIMESTAMPTZ — NOT NULL DEFAULT CURRENT_TIMESTAMP
- index `idx_recruitment_stage_events_candidate` on `recruitment_stage_events` (candidate_id)

#### `hr/000035_create_recruitment_interviews.up.sql`
- **table `recruitment_interviews`**
  - `id` UUID — PRIMARY KEY
  - `tenant_id` UUID — NOT NULL
  - `candidate_id` UUID — NOT NULL REFERENCES recruitment_candidates(id) ON DELETE CASCADE
  - `round_name` VARCHAR(100) — NOT NULL
  - `interviewer_employee_id` UUID — NOT NULL REFERENCES employee_profiles(id)
  - `scheduled_at` TIMESTAMPTZ — NOT NULL
  - `duration_mins` INT — NOT NULL CHECK (duration_mins BETWEEN 15 AND 480)
  - `meeting_link` VARCHAR(500)
  - `status` VARCHAR(20) — NOT NULL DEFAULT 'scheduled' CHECK (status IN ('scheduled', 'completed', 'cancelled'))
  - `rating` INT — CHECK (rating IS NULL OR rating BETWEEN 1 AND 5)
  - `recommendation` VARCHAR(20) — CHECK (recommendation IS NULL OR recommendation IN ('strong_hire', 'hire', 'hold', 'reject'))
  - `feedback` TEXT
  - `feedback_at` TIMESTAMPTZ
  - `created_at` TIMESTAMPTZ — NOT NULL DEFAULT CURRENT_TIMESTAMP
  - `updated_at` TIMESTAMPTZ — NOT NULL DEFAULT CURRENT_TIMESTAMP
- index `idx_recruitment_interviews_interviewer` on `recruitment_interviews` (tenant_id, interviewer_employee_id)
- index `idx_recruitment_interviews_candidate` on `recruitment_interviews` (candidate_id)

#### `hr/000036_create_recruitment_offers.up.sql`
- **table `recruitment_offers`**
  - `id` UUID — PRIMARY KEY
  - `tenant_id` UUID — NOT NULL
  - `candidate_id` UUID — NOT NULL REFERENCES recruitment_candidates(id) ON DELETE CASCADE
  - `offered_ctc` NUMERIC(14,2) — NOT NULL CHECK (offered_ctc > 0)
  - `joining_date` DATE — NOT NULL
  - `expires_on` DATE
  - `status` VARCHAR(20) — NOT NULL DEFAULT 'offered' CHECK (status IN ('offered', 'accepted', 'declined', 'withdrawn'))
  - `decided_at` TIMESTAMPTZ
  - `created_by_user_id` UUID — NOT NULL
  - `created_at` TIMESTAMPTZ — NOT NULL DEFAULT CURRENT_TIMESTAMP
  - `updated_at` TIMESTAMPTZ — NOT NULL DEFAULT CURRENT_TIMESTAMP
- UNIQUE index `uq_recruitment_offers_open` on `recruitment_offers` (candidate_id) WHERE status = 'offered'
- index `idx_recruitment_offers_candidate` on `recruitment_offers` (candidate_id)

#### `hr/000037_create_recruitment_events_outbox.up.sql`
- **table `recruitment_events_outbox`**
  - `id` UUID — PRIMARY KEY
  - `tenant_id` UUID — NOT NULL
  - `event_id` UUID — NOT NULL
  - `event_type` VARCHAR(100) — NOT NULL
  - `routing_key` VARCHAR(100) — NOT NULL
  - `payload` JSONB — NOT NULL
  - `published` BOOLEAN — NOT NULL DEFAULT FALSE
  - `published_at` TIMESTAMPTZ
  - `attempts` INT — NOT NULL DEFAULT 0
  - `last_error` TEXT
  - `created_at` TIMESTAMPTZ — NOT NULL DEFAULT CURRENT_TIMESTAMP
- UNIQUE index `uq_recruitment_outbox_event_id` on `recruitment_events_outbox` (event_id)
- index `idx_recruitment_outbox_tenant` on `recruitment_events_outbox` (tenant_id)
- index `idx_recruitment_outbox_published` on `recruitment_events_outbox` (published) WHERE published = FALSE

#### `hr/000038_create_performance_cycles.up.sql`
- **table `performance_cycles`**
  - `id` UUID — PRIMARY KEY
  - `tenant_id` UUID — NOT NULL
  - `title` VARCHAR(150) — NOT NULL
  - `start_date` DATE — NOT NULL
  - `end_date` DATE — NOT NULL
  - `criteria` JSONB — NOT NULL DEFAULT '[]'::jsonb CHECK (jsonb_typeof(criteria) = 'array')
  - `status` VARCHAR(20) — NOT NULL DEFAULT 'draft' CHECK (status IN ('draft', 'active', 'review', 'completed'))
  - `activated_at` TIMESTAMPTZ
  - `review_started_at` TIMESTAMPTZ
  - `completed_at` TIMESTAMPTZ
  - `created_by_user_id` UUID — NOT NULL
  - `created_at` TIMESTAMPTZ — NOT NULL DEFAULT CURRENT_TIMESTAMP
  - `updated_at` TIMESTAMPTZ — NOT NULL DEFAULT CURRENT_TIMESTAMP
  - _constraint:_ CHECK (end_date >= start_date)
- index `idx_performance_cycles_tenant_status` on `performance_cycles` (tenant_id, status)

#### `hr/000039_create_performance_goals.up.sql`
- **table `performance_goals`**
  - `id` UUID — PRIMARY KEY
  - `tenant_id` UUID — NOT NULL
  - `cycle_id` UUID — NOT NULL REFERENCES performance_cycles(id) ON DELETE CASCADE
  - `employee_id` UUID — NOT NULL REFERENCES employee_profiles(id)
  - `parent_goal_id` UUID — REFERENCES performance_goals(id)
  - `title` VARCHAR(200) — NOT NULL
  - `description` TEXT
  - `weight` NUMERIC(5,2) — NOT NULL CHECK (weight > 0 AND weight <= 100)
  - `progress` NUMERIC(5,2) — NOT NULL DEFAULT 0 CHECK (progress >= 0 AND progress <= 100)
  - `status` VARCHAR(20) — NOT NULL DEFAULT 'active' CHECK (status IN ('active', 'completed', 'cancelled'))
  - `created_by_user_id` UUID — NOT NULL
  - `created_at` TIMESTAMPTZ — NOT NULL DEFAULT CURRENT_TIMESTAMP
  - `updated_at` TIMESTAMPTZ — NOT NULL DEFAULT CURRENT_TIMESTAMP
  - _constraint:_ CHECK (parent_goal_id IS NULL OR parent_goal_id <> id)
- index `idx_performance_goals_cycle_employee` on `performance_goals` (tenant_id, cycle_id, employee_id)
- index `idx_performance_goals_parent` on `performance_goals` (parent_goal_id)

#### `hr/000040_create_performance_key_results.up.sql`
- **table `performance_key_results`**
  - `id` UUID — PRIMARY KEY
  - `tenant_id` UUID — NOT NULL
  - `goal_id` UUID — NOT NULL REFERENCES performance_goals(id) ON DELETE CASCADE
  - `title` VARCHAR(200) — NOT NULL
  - `unit` VARCHAR(20)
  - `start_value` NUMERIC(14,2) — NOT NULL
  - `target_value` NUMERIC(14,2) — NOT NULL
  - `current_value` NUMERIC(14,2) — NOT NULL
  - `created_at` TIMESTAMPTZ — NOT NULL DEFAULT CURRENT_TIMESTAMP
  - `updated_at` TIMESTAMPTZ — NOT NULL DEFAULT CURRENT_TIMESTAMP
  - _constraint:_ CHECK (target_value <> start_value)
- index `idx_performance_key_results_goal` on `performance_key_results` (goal_id)

#### `hr/000041_create_performance_checkins.up.sql`
- **table `performance_checkins`**
  - `id` UUID — PRIMARY KEY
  - `tenant_id` UUID — NOT NULL
  - `goal_id` UUID — NOT NULL REFERENCES performance_goals(id) ON DELETE CASCADE
  - `key_result_id` UUID — REFERENCES performance_key_results(id) ON DELETE CASCADE
  - `from_progress` NUMERIC(5,2) — NOT NULL CHECK (from_progress >= 0 AND from_progress <= 100)
  - `to_progress` NUMERIC(5,2) — NOT NULL CHECK (to_progress >= 0 AND to_progress <= 100)
  - `value` NUMERIC(14,2)
  - `note` VARCHAR(1000)
  - `author_user_id` UUID — NOT NULL
  - `created_at` TIMESTAMPTZ — NOT NULL DEFAULT CURRENT_TIMESTAMP
- index `idx_performance_checkins_goal` on `performance_checkins` (goal_id)

#### `hr/000042_create_performance_reviews.up.sql`
- **table `performance_reviews`**
  - `id` UUID — PRIMARY KEY
  - `tenant_id` UUID — NOT NULL
  - `cycle_id` UUID — NOT NULL REFERENCES performance_cycles(id) ON DELETE CASCADE
  - `employee_id` UUID — NOT NULL REFERENCES employee_profiles(id)
  - `reviewer_employee_id` UUID — NOT NULL REFERENCES employee_profiles(id)
  - `type` VARCHAR(10) — NOT NULL CHECK (type IN ('self', 'manager', 'peer'))
  - `status` VARCHAR(20) — NOT NULL DEFAULT 'pending' CHECK (status IN ('pending', 'submitted', 'expired'))
  - `ratings` JSONB — NOT NULL DEFAULT '[]'::jsonb CHECK (jsonb_typeof(ratings) = 'array')
  - `strengths` TEXT
  - `improvements` TEXT
  - `overall_score` NUMERIC(3,2) — CHECK (overall_score IS NULL OR (overall_score >= 1 AND overall_score <= 5))
  - `potential` VARCHAR(10) — CHECK (potential IS NULL OR potential IN ('low', 'medium', 'high'))
  - `submitted_at` TIMESTAMPTZ
  - `created_at` TIMESTAMPTZ — NOT NULL DEFAULT CURRENT_TIMESTAMP
  - `updated_at` TIMESTAMPTZ — NOT NULL DEFAULT CURRENT_TIMESTAMP
  - _constraint:_ CHECK ((type = 'self') = (reviewer_employee_id = employee_id))
  - _constraint:_ CHECK (status <> 'submitted' OR (overall_score IS NOT NULL AND submitted_at IS NOT NULL))
- UNIQUE index `uq_performance_reviews_assignment` on `performance_reviews` (cycle_id, employee_id, reviewer_employee_id, type)
- index `idx_performance_reviews_reviewer` on `performance_reviews` (tenant_id, reviewer_employee_id)

#### `hr/000043_create_performance_events_outbox.up.sql`
- **table `performance_events_outbox`**
  - `id` UUID — PRIMARY KEY
  - `tenant_id` UUID — NOT NULL
  - `event_id` UUID — NOT NULL
  - `event_type` VARCHAR(100) — NOT NULL
  - `routing_key` VARCHAR(100) — NOT NULL
  - `payload` JSONB — NOT NULL
  - `published` BOOLEAN — NOT NULL DEFAULT FALSE
  - `published_at` TIMESTAMPTZ
  - `attempts` INT — NOT NULL DEFAULT 0
  - `last_error` TEXT
  - `created_at` TIMESTAMPTZ — NOT NULL DEFAULT CURRENT_TIMESTAMP
- UNIQUE index `uq_performance_outbox_event_id` on `performance_events_outbox` (event_id)
- index `idx_performance_outbox_tenant` on `performance_events_outbox` (tenant_id)
- index `idx_performance_outbox_published` on `performance_events_outbox` (published) WHERE published = FALSE

#### `hr/000044_create_performance_calibrations.up.sql`
- **table `performance_calibrations`**
  - `id` UUID — PRIMARY KEY
  - `tenant_id` UUID — NOT NULL
  - `cycle_id` UUID — NOT NULL REFERENCES performance_cycles(id) ON DELETE CASCADE
  - `employee_id` UUID — NOT NULL REFERENCES employee_profiles(id)
  - `before_score` NUMERIC(3,2) — NOT NULL CHECK (before_score >= 1 AND before_score <= 5)
  - `before_potential` VARCHAR(10) — NOT NULL CHECK (before_potential IN ('low', 'medium', 'high'))
  - `after_score` NUMERIC(3,2) — NOT NULL CHECK (after_score >= 1 AND after_score <= 5)
  - `after_potential` VARCHAR(10) — NOT NULL CHECK (after_potential IN ('low', 'medium', 'high'))
  - `reason` VARCHAR(1000) — NOT NULL
  - `calibrated_by_user_id` UUID — NOT NULL
  - `created_at` TIMESTAMPTZ — NOT NULL DEFAULT CURRENT_TIMESTAMP
  - _constraint:_ CHECK (before_score <> after_score OR before_potential <> after_potential)
- index `idx_performance_calibrations_cycle_employee` on `performance_calibrations` (tenant_id, cycle_id, employee_id, created_at DESC)

#### `hr/000045_create_performance_feedback_requests.up.sql`
- **table `performance_feedback_requests`**
  - `id` UUID — PRIMARY KEY
  - `tenant_id` UUID — NOT NULL
  - `requester_employee_id` UUID — NOT NULL REFERENCES employee_profiles(id)
  - `responder_employee_id` UUID — NOT NULL REFERENCES employee_profiles(id)
  - `about_employee_id` UUID — NOT NULL REFERENCES employee_profiles(id)
  - `question` VARCHAR(500) — NOT NULL
  - `status` VARCHAR(20) — NOT NULL DEFAULT 'pending' CHECK (status IN ('pending', 'answered', 'declined'))
  - `feedback_id` UUID
  - `created_at` TIMESTAMPTZ — NOT NULL DEFAULT CURRENT_TIMESTAMP
  - `updated_at` TIMESTAMPTZ — NOT NULL DEFAULT CURRENT_TIMESTAMP
  - _constraint:_ CHECK (responder_employee_id <> about_employee_id)
  - _constraint:_ CHECK ((status = 'answered') = (feedback_id IS NOT NULL))
- UNIQUE index `uq_performance_feedback_requests_pending` on `performance_feedback_requests` (tenant_id, requester_employee_id, responder_employee_id, about_employee_id) WHERE status = 'pending'
- index `idx_performance_feedback_requests_responder` on `performance_feedback_requests` (tenant_id, responder_employee_id, status)
- index `idx_performance_feedback_requests_requester` on `performance_feedback_requests` (tenant_id, requester_employee_id, status)

#### `hr/000046_create_performance_feedback.up.sql`
- **table `performance_feedback`**
  - `id` UUID — PRIMARY KEY
  - `tenant_id` UUID — NOT NULL
  - `from_employee_id` UUID — NOT NULL REFERENCES employee_profiles(id)
  - `to_employee_id` UUID — NOT NULL REFERENCES employee_profiles(id)
  - `kind` VARCHAR(20) — NOT NULL CHECK (kind IN ('praise', 'constructive'))
  - `visibility` VARCHAR(10) — NOT NULL CHECK (visibility IN ('private', 'public'))
  - `message` VARCHAR(2000) — NOT NULL
  - `request_id` UUID — REFERENCES performance_feedback_requests(id)
  - `created_at` TIMESTAMPTZ — NOT NULL DEFAULT CURRENT_TIMESTAMP
  - _constraint:_ CHECK (from_employee_id <> to_employee_id)
  - _constraint:_ CHECK (visibility = 'private' OR kind = 'praise')
- index `idx_performance_feedback_to` on `performance_feedback` (tenant_id, to_employee_id, created_at DESC)
- index `idx_performance_feedback_from` on `performance_feedback` (tenant_id, from_employee_id, created_at DESC)
- index `idx_performance_feedback_public` on `performance_feedback` (tenant_id, created_at DESC) WHERE visibility = 'public'

## C. Code inventory — every file, type and function


### Shared infrastructure


#### `internal/shared/cache/redis.go` (120 lines)
- **type** `RedisClient struct` — RedisClient wraps the go-redis client.
- **func** `NewRedisClient(host string, port int, password string, db int) (*RedisClient, error)` — NewRedisClient initializes and connects to Redis server.
- **func** `(r *RedisClient) Get(ctx context.Context, key string) (string, bool, error)` — Get retrieves a string value from Redis by key. Returns empty and false if not found.
- **func** `(r *RedisClient) Set(ctx context.Context, key string, value string, expiration time.Duration) error` — Set stores a string value in Redis with a specific expiration time.
- **func** `(r *RedisClient) Delete(ctx context.Context, keys ...string) error` — Delete removes one or more keys from Redis.
- **func** `(r *RedisClient) Exists(ctx context.Context, key string) (bool, error)` — Exists checks if a key is present in Redis.
- **func** `(r *RedisClient) Close() error` — Close gracefully disconnects Redis client.
- **func** `(r *RedisClient) DeleteByPrefix(ctx context.Context, prefix string) error` — DeleteByPrefix removes every key that starts with prefix (SCAN based, safe for production).
- **func** `(r *RedisClient) IncrWithExpire(ctx context.Context, key string, window time.Duration) (int64, error)` — IncrWithExpire atomically increments key; the TTL is set only when the key is created (fixed window).
- **func** `(r *RedisClient) Decr(ctx context.Context, key string) error` — Decr atomically decrements key, never going below zero.
- **func** `(r *RedisClient) TTL(ctx context.Context, key string) (time.Duration, error)` — TTL returns the remaining time-to-live of key (negative when absent / no expiry).
- **func** `(r *RedisClient) Ping(ctx context.Context) error` — Ping checks the connection (readiness probes).

#### `internal/shared/config/config.go` (93 lines)
- **type** `Config struct` — Config holds all configurations of the application.
- **type** `PlatformConfig struct` — PlatformConfig holds platform-operator (super admin) settings.
- **type** `ServerConfig struct` — ServerConfig holds server configuration details.
- **type** `DatabaseConfig struct` — DatabaseConfig holds database configuration details.
- **type** `RedisConfig struct` — RedisConfig holds Redis configuration details.
- **type** `RabbitMQConfig struct` — RabbitMQConfig holds RabbitMQ configuration details.
- **type** `JWTConfig struct` — JWTConfig holds JWT configuration details.
- **func** `LoadConfig(configPath string, configName string) (*Config, error)` — LoadConfig reads configuration from file or environment variables.

#### `internal/shared/constants/constants.go` (36 lines)

#### `internal/shared/database/base.go` (30 lines)
- **type** `BaseModel struct` — BaseModel defines the common columns for persistent entities using UUID keys.
- **type** `TenantBaseModel struct` — TenantBaseModel extends BaseModel by adding a tenant scoping ID.
- **func** `(b *BaseModel) BeforeCreate(tx *gorm.DB) error` — BeforeCreate is a GORM hook that auto-generates a UUID v4 key if not already set.

#### `internal/shared/database/database.go` (35 lines)
- **func** `NewDatabase(cfg *config.DatabaseConfig) (*gorm.DB, error)` — NewDatabase initializes a GORM PostgreSQL connection with a configured connection pool.

#### `internal/shared/database/migrate.go` (112 lines)
- **type** `MigrationSet struct` — MigrationSet is one service's migrations, applied into its own schema.
- **func** `RunMigrationSets(ctx context.Context, db *gorm.DB, sets []MigrationSet) ([]string, error)` — RunMigrationSets applies every set in order. Within a set each pending *.up.sql runs in its own transaction with search_path set to the set's schema, and is recorded in <schema>.schema_migrations. It returns "schema/version" for every migration applied by this call.
- **func** `runSet(ctx context.Context, db *gorm.DB, set MigrationSet) ([]string, error)`
- **func** `appliedVersions(tx *gorm.DB, table string) (map[string]bool, error)`
- **func** `applyFile(tx *gorm.DB, set MigrationSet, name, table, version string) error`

#### `internal/shared/database/tx.go` (29 lines)
- **func** `BeginTx(ctx context.Context, db *gorm.DB) *gorm.DB` — BeginTx starts a request-scoped transaction. A nil db (unit tests with mocked services) yields a nil tx, which services treat the same way they treat the nil db they were previously handed.
- **func** `Finish(tx *gorm.DB, err error) error` — Finish commits tx when err is nil and rolls back otherwise. It returns the error the caller must report (the original err, or the commit failure). Safe to call with a nil tx.

#### `internal/shared/errors/db.go` (28 lines)
- **func** `Normalize(err error) error` — Normalize maps well-known persistence failures onto client-facing AppErrors so they never surface as HTTP 500. Unknown errors are returned unchanged.

#### `internal/shared/errors/errors.go` (32 lines)
- **type** `AppError struct` — AppError represents a domain error that maps to an HTTP response.
- **func** `(e *AppError) Error() string`
- **type** `ValidationErrorDetail struct` — ValidationErrorDetail provides details on a specific field validation failure.

#### `internal/shared/idempotency/middleware.go` (150 lines)
- **type** `Scoper func(c *gin.Context) (scope string, ok bool)` — Scoper returns who is calling (e.g. "tenant:user" from a verified token); ok=false skips idempotency handling.
- **func** `Middleware(store Store, scope Scoper, retention time.Duration) gin.HandlerFunc` — Middleware handles Idempotency-Key on POST/PUT/PATCH/DELETE requests that carry one.
- **func** `isWrite(method string) bool`
- **func** `abort(c *gin.Context, status int, code, msg string)`
- **func** `handle(c *gin.Context, store Store, who, key string, retention time.Duration)`
- **func** `replay(c *gin.Context, store Store, existing *Record, requestHash string, now time.Time)` — replay answers a repeated key: same payload gives the stored outcome; a different payload or a request still running gives 409.
- **func** `run(c *gin.Context, store Store, id uuid.UUID)` — run executes the handler, capturing its response; server errors release the key so the client can retry.
- **func** `hash(method, path string, body []byte) string`
- **type** `capture struct` — capture tees the response body.
- **func** `(w *capture) Write(b []byte) (int, error)`
- **func** `(w *capture) WriteString(s string) (int, error)`
- **func** `RunPurger(ctx context.Context, store Store, interval time.Duration)` — RunPurger deletes expired records every interval until ctx is cancelled.

#### `internal/shared/idempotency/store.go` (87 lines)
_Package:_ Package idempotency makes retried commands safe (docs/ARCHITECTURE.md §3.6): a client sends Idempotency-Key with a write; the first request claims the key, later requests with the same key and payload replay the stored outcome, and a reused key with a different payload is rejected. The claim is a database unique constraint, never Redis.
- **type** `Record struct` — Record is one claimed key.
- **func** `(Record) TableName() string { return "idempotency_records" }` — TableName maps to idempotency_records (migration 000060).
- **type** `Store interface` — Store persists claims and outcomes.
- **method** `Store.Claim(ctx context.Context, r *Record) (existing *Record, claimed bool, err error)` — Claim inserts r; when the key already exists it returns the existing record and claimed=false.
- **method** `Store.Complete(ctx context.Context, id uuid.UUID, status int, body []byte, contentType string) error`
- **method** `Store.Release(ctx context.Context, id uuid.UUID) error`
- **method** `Store.PurgeExpired(ctx context.Context, now time.Time) (int64, error)`
- **type** `gormStore struct{ db *gorm.DB }`
- **func** `NewStore(db *gorm.DB) Store { return gormStore{db: db} }` — NewStore returns the database-backed store.
- **func** `(s gormStore) Claim(ctx context.Context, r *Record) (*Record, bool, error)`
- **func** `(s gormStore) Complete(ctx context.Context, id uuid.UUID, status int, body []byte, contentType string) error`
- **func** `(s gormStore) Release(ctx context.Context, id uuid.UUID) error`
- **func** `(s gormStore) PurgeExpired(ctx context.Context, now time.Time) (int64, error)`

#### `internal/shared/logger/logger.go` (37 lines)
- **type** `Logger struct` — Logger is a wrapper around zerolog.Logger
- **func** `NewLogger(env string) *Logger` — NewLogger initializes and returns a structured logger.

#### `internal/shared/middleware/correlation.go` (44 lines)
- **type** `correlationKey struct{}`
- **func** `WithCorrelationID(ctx context.Context, id uuid.UUID) context.Context` — WithCorrelationID returns ctx carrying the request's correlation ID.
- **func** `CorrelationIDFrom(ctx context.Context) (uuid.UUID, bool)` — CorrelationIDFrom returns the correlation ID carried by ctx, if any.
- **func** `CorrelationOrNew(ctx context.Context) uuid.UUID` — CorrelationOrNew returns the ctx correlation ID or a fresh one (system-initiated work).
- **func** `incomingCorrelation(values ...string) uuid.UUID` — incomingCorrelation accepts a caller-supplied ID only when it is a valid UUID; anything else is replaced.

#### `internal/shared/middleware/cors.go` (74 lines)
- **func** `CORS(allowedOrigins []string) gin.HandlerFunc` — CORS handles Cross-Origin Resource Sharing. allowedOrigins is an explicit allow-list; a single "*" entry allows any origin (development only; credentials are then NOT allowed, per the CORS spec). An empty list disables CORS headers. Tenant subdomain origins can be allowed with a "*.example.com" wildcard entry.

#### `internal/shared/middleware/hardening.go` (39 lines)
- **func** `BodyLimit(max int64) gin.HandlerFunc` — BodyLimit rejects request bodies larger than max with 413 before handlers read them.
- **func** `SecurityHeaders() gin.HandlerFunc` — SecurityHeaders sets conservative defaults for an API that never serves HTML.

#### `internal/shared/middleware/rate_limiter.go` (69 lines)
- **type** `RateLimitStore interface` — RateLimitStore is the atomic counter backend required by the limiter (satisfied by cache.RedisClient).
- **method** `RateLimitStore.IncrWithExpire(ctx context.Context, key string, window time.Duration) (int64, error)` — IncrWithExpire atomically increments key and, on first creation, sets its TTL to window.
- **method** `RateLimitStore.Decr(ctx context.Context, key string) error` — Decr atomically decrements key (never below zero).
- **method** `RateLimitStore.TTL(ctx context.Context, key string) (time.Duration, error)` — TTL returns the remaining time-to-live of key.
- **func** `RateLimiter(store RateLimitStore, limit int, window time.Duration) gin.HandlerFunc` — RateLimiter returns a Gin middleware that rate-limits requests by client IP (fixed window, atomic). Every request counts.
- **func** `FailureRateLimiter(store RateLimitStore, limit int, window time.Duration) gin.HandlerFunc` — FailureRateLimiter is like RateLimiter but only FAILED attempts (HTTP >= 400) consume the budget: a successful request refunds its slot. Use it for credential endpoints so legitimate users are not locked out by their own successful logins while brute-force (failed) attempts are still throttled.
- **func** `rateLimiter(store RateLimitStore, limit int, window time.Duration, failuresOnly bool) gin.HandlerFunc`

#### `internal/shared/middleware/request_logger.go` (63 lines)
- **func** `RequestLogger(log *logger.Logger) gin.HandlerFunc` — RequestLogger outputs details of each incoming HTTP request.
- **func** `fmtString(v interface{}) string`

#### `internal/shared/outbox/outbox.go` (82 lines)
_Package:_ Package outbox is the transactional outbox shared by every service (docs/ARCHITECTURE.md §3.7): an event row is written in the same transaction as the business change and a relay publishes it after commit, retrying while the broker is unavailable. Delivery is at least once; consumers deduplicate by event_id.
- **type** `Row struct` — Row is one stored event. Payload is the full JSON envelope sent to the broker.
- **type** `Message struct` — Message is what a service emits.
- **type** `Store interface` — Store reads and writes one outbox table.
- **method** `Store.Add(ctx context.Context, tx *gorm.DB, m Message) error`
- **method** `Store.Pending(ctx context.Context, db *gorm.DB, limit, maxAttempts int) ([]Row, error)`
- **method** `Store.MarkPublished(ctx context.Context, db *gorm.DB, id uuid.UUID, at time.Time) error`
- **method** `Store.RecordFailure(ctx context.Context, db *gorm.DB, id uuid.UUID, reason string) error`
- **type** `store struct{ table string }`
- **func** `NewStore(table string) Store { return store{table: table} }` — NewStore returns a Store over the given table (each service owns its own outbox table).
- **func** `(s store) Add(ctx context.Context, tx *gorm.DB, m Message) error`
- **func** `(s store) Pending(ctx context.Context, db *gorm.DB, limit, maxAttempts int) ([]Row, error)`
- **func** `(s store) MarkPublished(ctx context.Context, db *gorm.DB, id uuid.UUID, at time.Time) error`
- **func** `(s store) RecordFailure(ctx context.Context, db *gorm.DB, id uuid.UUID, reason string) error`

#### `internal/shared/outbox/relay.go` (79 lines)
- **type** `Relay struct` — Relay publishes committed outbox rows.
- **func** `(r *Relay) defaults()`
- **func** `(r *Relay) RelayOnce(ctx context.Context) (int, error)` — RelayOnce publishes one batch and returns how many rows were published.
- **func** `(r *Relay) Run(ctx context.Context)` — Run relays until ctx is cancelled.

#### `internal/shared/queue/publisher.go` (25 lines)
- **type** `EventPublisher interface` — EventPublisher defines the contract to dispatch events to message brokers.
- **method** `EventPublisher.Publish(ctx context.Context, exchange string, routingKey string, event interface{}) error`
- **type** `NoOpPublisher struct{}` — NoOpPublisher is a mock publisher that discards published messages.
- **func** `NewNoOpPublisher() EventPublisher` — NewNoOpPublisher returns a new instance of NoOpPublisher.
- **func** `(n *NoOpPublisher) Publish(ctx context.Context, exchange string, routingKey string, event interface{}) error` — Publish prints log output and returns nil.

#### `internal/shared/queue/rabbitmq.go` (166 lines)
- **type** `RabbitMQPublisher struct` — RabbitMQPublisher implements EventPublisher using amqp091 connection.
- **func** `NewRabbitMQPublisher(host string, port int, user, password string) (*RabbitMQPublisher, error)` — NewRabbitMQPublisher connects to RabbitMQ and starts reconnection monitor.
- **func** `(p *RabbitMQPublisher) connect() error` — connect establishes connection and channel, declaring exchange.
- **func** `(p *RabbitMQPublisher) handleReconnect()` — handleReconnect runs in background to reconnect on connection loss.
- **func** `(p *RabbitMQPublisher) Publish(ctx context.Context, exchange string, routingKey string, event interface{}) error` — Publish serializes event and publishes it to the specified exchange.
- **func** `(p *RabbitMQPublisher) Close() error` — Close gracefully releases connections.

#### `internal/shared/utils/pagination.go` (34 lines)
- **func** `NormalizePagination(page, perPage int) (int, int)` — NormalizePagination applies defaults and bounds: page >= 1, 1 <= per_page <= MaxPerPage.
- **func** `ParsePagination(c *gin.Context) (int, int)` — ParsePagination reads ?page and ?per_page, tolerating junk values (falls back to defaults) and clamping the range.

#### `internal/shared/utils/validator.go` (85 lines)
- **func** `init()`
- **func** `ValidateStruct(s interface{}) []sharedErrors.ValidationErrorDetail` — ValidateStruct checks validation tags of structs and returns mapped field errors.
- **func** `formatValidationErrorMessage(fe validator.FieldError) string` — formatValidationErrorMessage structures a readable message based on the failure tag.
- **func** `BindErrorDetails(err error) []sharedErrors.ValidationErrorDetail` — BindErrorDetails converts a gin ShouldBind* error (validator failures, JSON syntax/type errors, empty body) into client-safe field details without leaking Go struct names.

### Application wiring (internal/app)


#### `internal/app/app.go` (129 lines)
_Package:_ Package app wires Modules 0-2 into a single HTTP engine. cmd/main.go and the integration tests share it so the tested router is exactly the production router.
- **type** `Options struct` — Options carries the infrastructure handles and runtime policy the engine needs.
- **func** `ValidateConfig(cfg *config.Config) error` — ValidateConfig rejects configurations that are unsafe to run.
- **func** `Build(o Options) (*gin.Engine, error)` — Build migrates the schema, seeds the permission catalogue and returns the fully wired engine.

#### `internal/app/health.go` (58 lines)
- **type** `pinger func(ctx context.Context) error` — pinger is a dependency the service needs to serve traffic.
- **func** `registerHealth(router *gin.Engine, db *gorm.DB, redis *cache.RedisClient)` — registerHealth mounts liveness and readiness (docs/ARCHITECTURE.md §3.10). Liveness only says the process serves HTTP; readiness checks Postgres and Redis. RabbitMQ is deliberately not required: the outbox holds events while the broker is down.
- **func** `ready(c *gin.Context, checks map[string]pinger)`

#### `internal/app/identity.go` (43 lines)
- **func** `startIdentityBackground(o Options, id *identity.Module, store idempotency.Store)` — startIdentityBackground runs M0's outbox relay and the idempotency purger while background work is enabled.
- **func** `registerDevMailbox(v1 *gin.RouterGroup, o Options, id *identity.Module)` — registerDevMailbox exposes one-time secrets (reset and invitation tokens) to developers and the test console in development and test only, behind the platform key: GET /api/v1/dev/mailbox?tenant_id=…&email=…

### M0 Identity & Access


#### `internal/identity/module.go` (328 lines)
- **type** `Module struct` — Module encapsulates the Module 0 Identity and Access Management domain.
- **func** `NewModule(` — NewModule constructs a Module container injecting database and message queue handles.
- **func** `(m *Module) SetPlatformAdminKey(key string) { m.platformAdminKey = key }` — SetPlatformAdminKey configures the platform operator key guarding /tenants routes (empty = routes disabled).
- **func** `(m *Module) SeedPermissions() error` — SeedPermissions registers M0's own permission catalogue (models.IdentityCatalogue).
- **func** `(m *Module) RegisterCatalogue(ctx context.Context, defs []access.PermissionDef) error` — RegisterCatalogue registers another service's permission definitions and their predefined-role defaults.
- **func** `(m *Module) SyncRoleTemplates(ctx context.Context) error { return m.catalogue.SyncAllTenants(ctx) }` — SyncRoleTemplates provisions predefined roles in every tenant; call it once all catalogues are registered.
- **func** `(m *Module) RunEventRelay(ctx context.Context) { m.relay.Run(ctx) }` — RunEventRelay publishes committed identity events until ctx is cancelled.
- **func** `(m *Module) EnableDevMailbox() *services.DevMailbox` — EnableDevMailbox keeps one-time secrets (reset and invitation tokens) in memory for development tooling. Never call it outside development.
- **type** `secretRouter struct{ m *Module }` — secretRouter delivers secrets to the dev mailbox when enabled; production delivery arrives with M10 Notifications.
- **func** `(r secretRouter) Deliver(ctx context.Context, msg services.SecretMessage) error`
- **func** `(m *Module) CallerScope(c *gin.Context) (string, bool)` — CallerScope identifies the caller of a request for Idempotency-Key scoping: "platform" for a valid platform key, "tenant:user" for a valid access token, otherwise not scoped.
- **func** `(m *Module) RegisterWellKnown(router gin.IRoutes)` — RegisterWellKnown mounts the public JWKS at the server root (/.well-known/jwks.json) so any service can verify access tokens without sharing a secret (M0 plan B4).
- **func** `(m *Module) SetAutomaticRoleSource(src services.AutomaticRoleSource) { m.automatic.SetSource(src) }` — SetAutomaticRoleSource installs the HR-backed source of the automatic employee/manager roles.
- **func** `(m *Module) RunAutomaticRoleSync(ctx context.Context, interval time.Duration)` — RunAutomaticRoleSync reconciles automatic roles in every tenant each interval until ctx is cancelled.
- **func** `(m *Module) syncUserOnLogin(ctx context.Context, tenantID, userID uuid.UUID)` — syncUserOnLogin refreshes the user's automatic roles at login; failures only delay the sync to the next sweep.
- **func** `(m *Module) Authorizer() middleware.Authorizer { return m.resolver }` — Authorizer exposes access resolution to the permission middleware of dependent modules.
- **func** `(m *Module) RegisterModels() []interface{}` — RegisterModels lists all domain entity models for migrations.
- **func** `(m *Module) RegisterRoutes(router *gin.RouterGroup)` — RegisterRoutes hooks endpoint paths to controller handlers.
- **func** `(m *Module) UserService() services.UserService { return m.userSvc }` — UserService exposes the user domain service to dependent modules (Module 1+).
- **func** `(m *Module) AuditService() services.AuditService { return m.auditSvc }` — AuditService exposes the audit transport to dependent modules.
- **func** `(m *Module) UserRoleRepository() repositories.UserRoleRepository { return m.userRoleRepo }` — UserRoleRepository exposes user-role reads for RBAC middleware in dependent modules.
- **func** `(m *Module) RolePermissionRepository() repositories.RolePermissionRepository` — RolePermissionRepository exposes role-permission reads for RBAC middleware in dependent modules.
- **func** `(m *Module) TenantResolver() gin.HandlerFunc` — TenantResolver builds the subdomain tenant-resolution middleware for dependent modules.
- **func** `(m *Module) AuthMiddleware() gin.HandlerFunc` — AuthMiddleware builds the JWT authentication middleware for dependent modules.

#### `internal/identity/access/catalogue.go` (106 lines)
- **type** `Template struct` — Template describes a predefined role seeded into every tenant.
- **func** `TemplateByKey(key string) (Template, bool)` — TemplateByKey finds a template.
- **type** `PermissionDef struct` — PermissionDef is one catalogue entry a service registers with M0, including the scope each predefined role receives by default.
- **func** `(d PermissionDef) Key() string { return Key(d.Resource, d.Action) }` — Key returns "resource:action".
- **func** `ValidateDefs(defs []PermissionDef) error` — ValidateDefs checks names, unknown templates and invalid scopes, and duplicate keys.
- **func** `DefaultGrants(defs []PermissionDef, templateKey string) []Grant` — DefaultGrants returns the grants a template receives from defs (its own, not inherited).

#### `internal/identity/access/resolve.go` (212 lines)
- **type** `AssignmentSource string` — AssignmentSource says how a user came to hold a role.
- **type** `SourceKind string` — SourceKind explains where an effective permission comes from.
- **type** `RoleNode struct` — RoleNode is one tenant role with its own grants and optional parent.
- **type** `Assignment struct` — Assignment is a role held by the user.
- **type** `UserGrant struct` — UserGrant is a grant given to one user, optionally until ExpiresAt.
- **type** `Input struct` — Input is everything needed to resolve one user's effective access.
- **type** `Source struct` — Source is one reason a permission is held.
- **type** `Entry struct` — Entry is one effective permission: the widest scope held, why, and whether a block removes it.
- **type** `Effective map[string]*Entry` — Effective maps permission keys to entries.
- **func** `(e Effective) ScopeFor(permission string) Scope` — ScopeFor returns the widest unblocked scope for a permission key (ScopeNone when not held).
- **func** `(e Effective) Missing(want []Grant) []Grant` — Missing returns the wanted grants that e does not cover.
- **func** `(e Effective) Permissions() []string` — Permissions lists the held (unblocked) permission keys, sorted.
- **func** `(e Effective) add(permission string, src Source)`
- **func** `Resolve(in Input) (Effective, error)` — Resolve computes effective access: assigned roles and their ancestors, unexpired user grants, minus blocks.
- **func** `ancestry(byID map[uuid.UUID]*RoleNode, id uuid.UUID) ([]*RoleNode, error)` — ancestry returns the role followed by its ancestors; unknown roles yield an empty chain.
- **func** `next(byID map[uuid.UUID]*RoleNode, r *RoleNode) (*RoleNode, bool)`
- **func** `CheckParent(roles []RoleNode, role, parent uuid.UUID) error` — CheckParent validates making parent the parent of role: no cycle, chain within MaxRoleDepth.

#### `internal/identity/access/scope.go` (64 lines)
_Package:_ Package access is the pure access model of M0 (docs/ARCHITECTURE.md §17): scopes, grants, role inheritance and the resolution of a user's effective access. It has no I/O; services feed it rows and cache its result.
- **type** `Scope string` — Scope is how far a granted permission reaches.
- **func** `(s Scope) Valid() bool { return ranks[s] > 0 }` — Valid reports whether s is a known, non-empty scope.
- **func** `ParseScope(raw string) (Scope, error)` — ParseScope validates a scope; empty means tenant (the meaning of legacy grants without a scope).
- **func** `Covers(held, want Scope) bool` — Covers reports whether holding scope held is enough for scope want.
- **func** `Wider(a, b Scope) Scope` — Wider returns the scope with the larger reach.
- **func** `Key(resource, action string) string { return resource + ":" + action }` — Key formats a permission key "resource:action".
- **type** `Grant struct` — Grant is a permission key held at a scope.

#### `internal/identity/controllers/access_controller.go` (180 lines)
- **type** `AccessController struct` — AccessController serves per-user grants and blocks, explain-access and automatic-role sync (docs/ARCHITECTURE.md §17).
- **func** `NewAccessController(db *gorm.DB, svc services.AccessService, automatic services.AutomaticRoles) *AccessController` — NewAccessController creates an AccessController.
- **func** `target(c *gin.Context) (uuid.UUID, uuid.UUID, bool)` — target reads the tenant from context and the user from :id.
- **func** `permissionParam(c *gin.Context) (uuid.UUID, bool)`
- **func** `bindValid(c *gin.Context, req interface{}) bool`
- **func** `(ctrl *AccessController) Explain(c *gin.Context)` — Explain returns a user's effective access with sources (GET /users/:id/access).
- **func** `(ctrl *AccessController) ExplainMe(c *gin.Context)` — ExplainMe returns the caller's own effective access with sources (GET /auth/me/access).
- **func** `respond(c *gin.Context, status int, res interface{}, err error)`
- **func** `(ctrl *AccessController) ListGrants(c *gin.Context)` — ListGrants lists a user's individual grants (GET /users/:id/grants).
- **func** `(ctrl *AccessController) Grant(c *gin.Context)` — Grant gives a user a permission at a scope (POST /users/:id/grants).
- **func** `(ctrl *AccessController) RevokeGrant(c *gin.Context)` — RevokeGrant removes an individual grant (DELETE /users/:id/grants/:permission_id).
- **func** `(ctrl *AccessController) ListBlocks(c *gin.Context)` — ListBlocks lists a user's blocks (GET /users/:id/blocks).
- **func** `(ctrl *AccessController) Block(c *gin.Context)` — Block removes a permission from a user whatever their roles grant (POST /users/:id/blocks).
- **func** `(ctrl *AccessController) Unblock(c *gin.Context)` — Unblock lifts a block (DELETE /users/:id/blocks/:permission_id).
- **func** `(ctrl *AccessController) SyncAutomaticRoles(c *gin.Context)` — SyncAutomaticRoles reconciles the tenant's automatic employee/manager roles now (POST /access/automatic-roles/sync).

#### `internal/identity/controllers/audit_controller.go` (38 lines)
- **type** `AuditController struct` — AuditController exposes the read-only audit trail of a tenant.
- **func** `NewAuditController(db *gorm.DB, auditSvc services.AuditService) *AuditController` — NewAuditController creates an AuditController.
- **func** `(ctrl *AuditController) List(c *gin.Context)` — List handles GET /audit-logs.

#### `internal/identity/controllers/auth_controller.go` (160 lines)
- **type** `AuthController struct`
- **func** `(ctrl *AuthController) OnLogin(hook func(ctx context.Context, tenantID, userID uuid.UUID))` — OnLogin registers a best-effort hook run after every successful login (e.g. automatic-role sync).
- **func** `NewAuthController(db *gorm.DB, authSvc services.AuthService) *AuthController` — NewAuthController creates an AuthController instance.
- **func** `(ctrl *AuthController) Login(c *gin.Context)`
- **func** `(ctrl *AuthController) afterLogin(ctx context.Context, res *dto.LoginResponse)`
- **func** `(ctrl *AuthController) Logout(c *gin.Context)`
- **func** `(ctrl *AuthController) Refresh(c *gin.Context)`
- **func** `(ctrl *AuthController) ForgotPassword(c *gin.Context)`
- **func** `(ctrl *AuthController) ResetPassword(c *gin.Context)`

#### `internal/identity/controllers/invitation_controller.go` (108 lines)
- **type** `InvitationController struct` — InvitationController serves the invitation lifecycle (M0 plan B3).
- **func** `NewInvitationController(db *gorm.DB, users services.UserService, invites services.InvitationService) *InvitationController` — NewInvitationController creates an InvitationController.
- **func** `tenantOf(c *gin.Context) (uuid.UUID, bool)`
- **func** `(ctrl *InvitationController) Create(c *gin.Context)` — Create invites a user: the account is created as invited and the token is sent to them (POST /invitations).
- **func** `(ctrl *InvitationController) List(c *gin.Context)` — List shows the tenant's invitations; ?status=pending limits to open ones (GET /invitations).
- **func** `(ctrl *InvitationController) withID(c *gin.Context) (uuid.UUID, uuid.UUID, bool)`
- **func** `(ctrl *InvitationController) Resend(c *gin.Context)` — Resend issues a fresh token and revokes the old one (POST /invitations/:id/resend).
- **func** `(ctrl *InvitationController) Revoke(c *gin.Context)` — Revoke cancels an open invitation (POST /invitations/:id/revoke).
- **func** `(ctrl *InvitationController) Accept(c *gin.Context)` — Accept sets the password with the invitation token and activates the account (POST /auth/accept-invitation).

#### `internal/identity/controllers/permission_controller.go` (35 lines)
- **type** `PermissionController struct`
- **func** `NewPermissionController(db *gorm.DB, permSvc services.PermissionService) *PermissionController` — NewPermissionController creates a PermissionController.
- **func** `(ctrl *PermissionController) List(c *gin.Context)`

#### `internal/identity/controllers/response.go` (103 lines)
- **type** `successEnvelope struct`
- **type** `errorEnvelope struct`
- **type** `errorDetail struct`
- **func** `respondSuccess(c *gin.Context, status int, data interface{}, meta interface{})` — respondSuccess returns a consistent success format envelope.
- **func** `respondError(c *gin.Context, err error)` — respondError wraps error types (e.g. AppError, Validation errors) into client envelopes.
- **func** `respondValidationError(c *gin.Context, details []sharedErrors.ValidationErrorDetail)` — respondValidationError returns field-level validation failure details.
- **func** `respondBadRequest(c *gin.Context, msg string)` — respondBadRequest returns a 400 in the standard error envelope.
- **func** `respondBindError(c *gin.Context, err error)` — respondBindError converts a request-binding failure into field-level validation details.
- **func** `reqCtx(c *gin.Context) context.Context` — reqCtx returns the request context enriched with the authenticated caller (user id + JWT roles) so services can enforce no-privilege-escalation rules on grants.

#### `internal/identity/controllers/role_controller.go` (329 lines)
- **type** `RoleController struct`
- **func** `NewRoleController(db *gorm.DB, roleSvc services.RoleService) *RoleController` — NewRoleController creates a RoleController.
- **func** `(ctrl *RoleController) Create(c *gin.Context)`
- **func** `(ctrl *RoleController) GetByID(c *gin.Context)`
- **func** `(ctrl *RoleController) List(c *gin.Context)`
- **func** `(ctrl *RoleController) Update(c *gin.Context)`
- **func** `(ctrl *RoleController) Delete(c *gin.Context)`
- **func** `(ctrl *RoleController) AssignPermissions(c *gin.Context)`
- **func** `(ctrl *RoleController) AssignRolesToUser(c *gin.Context)`
- **func** `(ctrl *RoleController) RemovePermission(c *gin.Context)` — RemovePermission detaches a permission from a role (DELETE /roles/:id/permissions/:permission_id).
- **func** `(ctrl *RoleController) RemoveRoleFromUser(c *gin.Context)` — RemoveRoleFromUser detaches a role from a user (DELETE /users/:id/roles/:role_id).
- **func** `(ctrl *RoleController) Me(c *gin.Context)` — Me returns the caller's roles and effective permissions (GET /auth/me).
- **func** `(ctrl *RoleController) Clone(c *gin.Context)` — Clone copies a role's grants and parent into a new custom role (POST /roles/:id/clone).

#### `internal/identity/controllers/tenant_controller.go` (146 lines)
- **type** `TenantController struct`
- **func** `NewTenantController(db *gorm.DB, tenantSvc services.TenantService) *TenantController` — NewTenantController returns a TenantController instance.
- **func** `(ctrl *TenantController) Create(c *gin.Context)`
- **func** `(ctrl *TenantController) GetByID(c *gin.Context)`
- **func** `(ctrl *TenantController) List(c *gin.Context)`
- **func** `(ctrl *TenantController) Update(c *gin.Context)`
- **func** `(ctrl *TenantController) Activate(c *gin.Context)`
- **func** `(ctrl *TenantController) Suspend(c *gin.Context)`

#### `internal/identity/controllers/user_controller.go` (195 lines)
- **type** `UserController struct`
- **func** `NewUserController(db *gorm.DB, userSvc services.UserService) *UserController` — NewUserController creates a UserController.
- **func** `(ctrl *UserController) Create(c *gin.Context)`
- **func** `(ctrl *UserController) GetByID(c *gin.Context)`
- **func** `(ctrl *UserController) List(c *gin.Context)`
- **func** `(ctrl *UserController) Update(c *gin.Context)`
- **func** `(ctrl *UserController) Deactivate(c *gin.Context)`
- **func** `(ctrl *UserController) Activate(c *gin.Context)` — Activate re-enables a deactivated user (POST /users/:id/activate).

#### `internal/identity/dto/access_dto.go` (70 lines)
- **type** `UserGrantRequest struct` — UserGrantRequest grants one permission to one user at a scope, optionally until expires_at.
- **type** `UserBlockRequest struct` — UserBlockRequest removes one permission from one user whatever their roles grant.
- **type** `UserGrantResponse struct` — UserGrantResponse is a stored per-user grant.
- **type** `UserBlockResponse struct` — UserBlockResponse is a stored per-user block.
- **type** `AccessSource struct` — AccessSource explains one reason a permission is held.
- **type** `ExplainedPermission struct` — ExplainedPermission is one effective permission with its scope, sources and whether a block removes it.
- **type** `ExplainAccessResponse struct` — ExplainAccessResponse shows a user's effective access and where every permission comes from.
- **type** `AutomaticRoleSyncResponse struct` — AutomaticRoleSyncResponse reports an automatic-role reconciliation.

#### `internal/identity/dto/audit_dto.go` (26 lines)
- **type** `AuditLogResponse struct` — AuditLogResponse is a read-only view of an audit record.
- **type** `AuditLogListResponse struct` — AuditLogListResponse holds paginated audit records.

#### `internal/identity/dto/auth_dto.go` (66 lines)
- **type** `LoginRequest struct` — LoginRequest contains payload for user authentication.
- **type** `UserLoginSummary struct` — UserLoginSummary maps basic user profile inside a login response.
- **type** `RoleSummary struct` — RoleSummary maps roles in user details response.
- **type** `LoginResponse struct` — LoginResponse contains response payload after successful login.
- **type** `RefreshTokenRequest struct` — RefreshTokenRequest contains payload to swap refresh token for new tokens.
- **type** `RefreshTokenResponse struct` — RefreshTokenResponse holds rotated token credentials.
- **type** `ForgotPasswordRequest struct` — ForgotPasswordRequest triggers reset email generation.
- **type** `ResetPasswordRequest struct` — ResetPasswordRequest consumes reset token to update password.
- **type** `MessageResponse struct` — MessageResponse is a generic success feedback envelope.

#### `internal/identity/dto/invitation_dto.go` (28 lines)
- **type** `InvitationResponse struct` — InvitationResponse describes an invitation (never its token).
- **type** `InviteResponse struct` — InviteResponse is the result of inviting a user: the invited account and its open invitation.
- **type** `AcceptInvitationRequest struct` — AcceptInvitationRequest sets the invited user's password using the token from the invitation.

#### `internal/identity/dto/pagination_dto.go` (31 lines)
- **type** `PaginationRequest struct` — PaginationRequest holds paging query parameters.
- **func** `(p *PaginationRequest) GetPage() int` — GetPage returns the current page, defaulting to 1 if not set.
- **func** `(p *PaginationRequest) GetPerPage() int` — GetPerPage returns the current per page limit, defaulting to 20 if not set.
- **type** `PaginationMeta struct` — PaginationMeta contains pagination metadata returned in responses.

#### `internal/identity/dto/permission_dto.go` (15 lines)
- **type** `PermissionResponse struct` — PermissionResponse represents authorization permission records.
- **type** `PermissionListResponse struct` — PermissionListResponse holds paginated PermissionResponses.

#### `internal/identity/dto/role_dto.go` (88 lines)
- **type** `GrantInput struct` — GrantInput is a permission granted at a scope (docs/ARCHITECTURE.md §17); an empty scope means tenant.
- **type** `CreateRoleRequest struct` — CreateRoleRequest contains payload to create a new role. permission_ids are granted at tenant scope; grants carry explicit scopes. parent_role_id makes the role inherit every grant of the parent.
- **type** `UpdateRoleRequest struct` — UpdateRoleRequest is a partial payload to update role metadata. parent_role_id "" removes the parent.
- **type** `CloneRoleRequest struct` — CloneRoleRequest names the copy of a role.
- **type** `AssignRoleRequest struct` — AssignRoleRequest holds roles to assign to a user.
- **type** `AssignPermissionsRequest struct` — AssignPermissionsRequest holds permissions to assign to a role (at least one of permission_ids or grants). Assigning a permission the role already holds changes its scope.
- **type** `PermissionSummary struct` — PermissionSummary maps minimal permission metadata inside nested responses.
- **type** `RoleResponse struct` — RoleResponse represents the serialization structure for role output.
- **type** `RoleListResponse struct` — RoleListResponse holds paginated RoleResponses.
- **type** `AccessResponse struct` — AccessResponse describes what the authenticated caller may do (roles + effective permissions).
- **type** `ScopedGrant struct` — ScopedGrant is a held permission and its scope.

#### `internal/identity/dto/session_dto.go` (14 lines)
- **type** `SessionResponse struct` — SessionResponse represents serializable details of an active session.

#### `internal/identity/dto/tenant_dto.go` (48 lines)
- **type** `CreateTenantRequest struct` — CreateTenantRequest contains payload to register a new tenant organization.
- **type** `BootstrapAdminRequest struct` — BootstrapAdminRequest describes the first administrator created together with a tenant.
- **type** `UpdateTenantRequest struct` — UpdateTenantRequest contains payload to update tenant fields.
- **type** `TenantResponse struct` — TenantResponse represents serialized tenant details in API responses.
- **type** `TenantListResponse struct` — TenantListResponse represents a paginated list of tenants.

#### `internal/identity/dto/user_dto.go` (51 lines)
- **type** `CreateUserRequest struct` — CreateUserRequest contains payload to create a new user profile.
- **type** `UpdateUserRequest struct` — UpdateUserRequest is a partial payload to update user profile.
- **type** `InviteUserRequest struct` — InviteUserRequest contains payload to invite a new user.
- **type** `UserResponse struct` — UserResponse represents user profile output payload.
- **type** `UserListResponse struct` — UserListResponse holds a paginated array of UserResponses.

#### `internal/identity/events/events.go` (176 lines)
- **type** `Event struct` — Event is the generic envelope for publishing domain events.
- **func** `NewEvent(eventType string, routingKey string, tenantID uuid.UUID, correlationID uuid.UUID, payload interface{}) Event` — NewEvent wraps a domain payload inside the standard event envelope.
- **type** `UserActivatedPayload struct` — UserActivatedPayload reports an invited user who accepted the invitation.
- **type** `TenantCreatedPayload struct`
- **type** `TenantActivatedPayload struct`
- **type** `TenantSuspendedPayload struct`
- **type** `UserCreatedPayload struct`
- **type** `UserInvitedPayload struct`
- **type** `UserDeactivatedPayload struct`
- **type** `RoleAssignedPayload struct`
- **type** `RoleRevokedPayload struct`
- **type** `PermissionAssignedPayload struct`
- **type** `PermissionRevokedPayload struct`
- **type** `PasswordResetPayload struct`
- **type** `SessionRevokedPayload struct`

#### `internal/identity/middleware/audit.go` (59 lines)
- **func** `AuditLog(db *gorm.DB, auditSvc services.AuditService, action, resource string) gin.HandlerFunc` — AuditLog captures API request context and logs actions via the AuditService post-execution.

#### `internal/identity/middleware/auth.go` (108 lines)
- **func** `Authenticate(db *gorm.DB, tokenSvc services.TokenService, sessionSvc services.SessionService) gin.HandlerFunc` — Authenticate verifies the Bearer access token and active session.

#### `internal/identity/middleware/platform.go` (53 lines)
- **func** `PlatformAdmin(key string) gin.HandlerFunc` — PlatformAdmin guards platform-level (cross-tenant) routes such as tenant provisioning. It fails closed: with no configured key every request is refused (503), and a wrong/missing key is 401/403.
- **func** `PlatformKeyValid(got, key string) bool` — PlatformKeyValid compares a presented platform key with the configured one in constant time.

#### `internal/identity/middleware/rbac.go` (104 lines)
- **type** `Authorizer interface` — Authorizer answers which scope a user holds for resource:action (docs/ARCHITECTURE.md §17); services.AccessResolver implements it.
- **method** `Authorizer.ScopeFor(ctx context.Context, tenantID, userID uuid.UUID, resource, action string) (access.Scope, error)`
- **type** `permissionDecision struct` — permissionDecision is the outcome of evaluating resource:action for the authenticated caller.
- **func** `denied(status int, code, msg string) permissionDecision`
- **func** `evaluateScope(c *gin.Context, authz Authorizer, resource, action string) permissionDecision` — evaluateScope resolves the caller's widest scope for resource:action. The tenant_admin system role (from the verified token) holds every permission at tenant scope.
- **func** `forbidden() permissionDecision`
- **func** `abort(c *gin.Context, d permissionDecision)`
- **func** `RequirePermission(authz Authorizer, resource, action string) gin.HandlerFunc` — RequirePermission admits callers holding resource:action at tenant scope. Routes written before scoped access assume tenant-wide reach, so a narrower grant must never pass them.
- **func** `RequireScopedPermission(authz Authorizer, resource, action string, minScope access.Scope) gin.HandlerFunc` — RequireScopedPermission admits callers holding resource:action at minScope or wider and stores the caller's scope under ScopeKey; the owning service must then limit the data to that scope.
- **func** `PermissionFlag(authz Authorizer, resource, action, key string) gin.HandlerFunc` — PermissionFlag never blocks: it stores under key whether the caller holds resource:action at tenant scope, so handlers can vary behaviour (e.g. unmasked PII) on a finer permission than the route requires.
- **func** `ScopeFrom(c *gin.Context) access.Scope` — ScopeFrom returns the scope a scoped route stored for the caller (ScopeNone when absent).

#### `internal/identity/middleware/tenant.go` (84 lines)
- **func** `TenantResolver(db *gorm.DB, tenantRepo repositories.TenantRepository) gin.HandlerFunc` — TenantResolver parses Host subdomains and binds tenant context to requests.

#### `internal/identity/models/audit_log.go` (33 lines)
- **type** `AuditLog struct` — AuditLog represents an immutable append-only record of system actions.
- **func** `(al *AuditLog) BeforeCreate(tx *gorm.DB) error` — BeforeCreate is a GORM hook that auto-generates a UUID v4 key if not already set.

#### `internal/identity/models/invitation.go` (35 lines)
- **type** `Invitation struct` — Invitation lets an invited user activate their account with a single-use token (only its hash is stored).
- **func** `(i *Invitation) BeforeCreate(tx *gorm.DB) error` — BeforeCreate assigns a UUID.
- **func** `(i *Invitation) Pending(now time.Time) bool` — Pending reports whether the invitation can still be accepted at now.

#### `internal/identity/models/mfa_config.go` (29 lines)
- **type** `MFAConfig struct` — MFAConfig represents a multi-factor authentication setup for a user.
- **func** `(m *MFAConfig) BeforeCreate(tx *gorm.DB) error` — BeforeCreate is a GORM hook that auto-generates a UUID v4 key if not already set.

#### `internal/identity/models/password_reset_token.go` (28 lines)
- **type** `PasswordResetToken struct` — PasswordResetToken represents a verification token generated for password reset requests.
- **func** `(pr *PasswordResetToken) BeforeCreate(tx *gorm.DB) error` — BeforeCreate is a GORM hook that auto-generates a UUID v4 key if not already set.

#### `internal/identity/models/permission.go` (28 lines)
- **type** `Permission struct` — Permission represents a global capability definition.
- **func** `(p *Permission) BeforeCreate(tx *gorm.DB) error` — BeforeCreate is a GORM hook that auto-generates a UUID v4 key if not already set.

#### `internal/identity/models/permission_catalog.go` (42 lines)
- **func** `tenantWide(templates ...string) map[string]access.Scope`

#### `internal/identity/models/refresh_token.go` (32 lines)
- **type** `RefreshToken struct` — RefreshToken represents a refresh token used for JWT renewal.
- **func** `(rt *RefreshToken) BeforeCreate(tx *gorm.DB) error` — BeforeCreate is a GORM hook that auto-generates a UUID v4 key if not already set.

#### `internal/identity/models/role.go` (25 lines)
- **type** `Role struct` — Role represents a Role-Based Access Control role scoped to a tenant.

#### `internal/identity/models/role_permission.go` (31 lines)
- **type** `RolePermission struct` — RolePermission represents the association of permissions to roles scoped to a tenant.
- **func** `(rp *RolePermission) BeforeCreate(tx *gorm.DB) error` — BeforeCreate is a GORM hook that auto-generates a UUID v4 key if not already set.

#### `internal/identity/models/session.go` (29 lines)
- **type** `Session struct` — Session represents an active login session.
- **func** `(s *Session) BeforeCreate(tx *gorm.DB) error` — BeforeCreate is a GORM hook that auto-generates a UUID v4 key if not already set.

#### `internal/identity/models/tenant.go` (24 lines)
- **type** `Tenant struct` — Tenant represents an isolated customer organization.

#### `internal/identity/models/tenant_settings.go` (26 lines)
- **type** `TenantSettings struct` — TenantSettings represents a key-value configuration scoped to a tenant.
- **func** `(t *TenantSettings) BeforeCreate(tx *gorm.DB) error` — BeforeCreate is a GORM hook that auto-generates a UUID v4 key if not already set.

#### `internal/identity/models/user.go` (28 lines)
- **type** `User struct` — User represents a user identity within a tenant.

#### `internal/identity/models/user_permission.go` (52 lines)
- **type** `UserPermissionGrant struct` — UserPermissionGrant gives one user a permission at a scope, optionally until ExpiresAt (docs/ARCHITECTURE.md §17).
- **func** `(g *UserPermissionGrant) BeforeCreate(tx *gorm.DB) error` — BeforeCreate assigns a UUID.
- **type** `UserPermissionBlock struct` — UserPermissionBlock removes a permission from one user whatever their roles grant; blocks always win.
- **func** `(b *UserPermissionBlock) BeforeCreate(tx *gorm.DB) error` — BeforeCreate assigns a UUID.

#### `internal/identity/models/user_role.go` (36 lines)
- **type** `UserRole struct` — UserRole represents a many-to-many relationship between users and roles scoped to a tenant.
- **func** `(ur *UserRole) BeforeCreate(tx *gorm.DB) error` — BeforeCreate is a GORM hook that auto-generates a UUID v4 key if not already set.

#### `internal/identity/repositories/access_repository.go` (153 lines)
- **type** `AccessRepository interface` — AccessRepository persists the access-model rows beyond plain role/permission CRUD (docs/ARCHITECTURE.md §17).
- **method** `AccessRepository.AccessVersion(ctx context.Context, db *gorm.DB, tenantID uuid.UUID) (int64, error)`
- **method** `AccessRepository.TenantRoles(ctx context.Context, db *gorm.DB, tenantID uuid.UUID) ([]models.Role, error)`
- **method** `AccessRepository.RoleByTemplateKey(ctx context.Context, db *gorm.DB, tenantID uuid.UUID, key string) (*models.Role, error)`
- **method** `AccessRepository.SetRolePermissionScope(ctx context.Context, tx *gorm.DB, tenantID, roleID, permissionID uuid.UUID, scope string) error`
- **method** `AccessRepository.UserGrants(ctx context.Context, db *gorm.DB, tenantID, userID uuid.UUID) ([]models.UserPermissionGrant, error)`
- **method** `AccessRepository.UpsertUserGrant(ctx context.Context, tx *gorm.DB, g *models.UserPermissionGrant) error`
- **method** `AccessRepository.DeleteUserGrant(ctx context.Context, tx *gorm.DB, tenantID, userID, permissionID uuid.UUID) (bool, error)`
- **method** `AccessRepository.UserBlocks(ctx context.Context, db *gorm.DB, tenantID, userID uuid.UUID) ([]models.UserPermissionBlock, error)`
- **method** `AccessRepository.UpsertUserBlock(ctx context.Context, tx *gorm.DB, b *models.UserPermissionBlock) error`
- **method** `AccessRepository.DeleteUserBlock(ctx context.Context, tx *gorm.DB, tenantID, userID, permissionID uuid.UUID) (bool, error)`
- **method** `AccessRepository.AutomaticAssignments(ctx context.Context, db *gorm.DB, tenantID uuid.UUID) ([]models.UserRole, error)`
- **method** `AccessRepository.UpsertPermission(ctx context.Context, db *gorm.DB, p *models.Permission) error` — UpsertPermission inserts a catalogue entry or refreshes its description, module and sensitivity.
- **method** `AccessRepository.AllPermissions(ctx context.Context, db *gorm.DB) ([]models.Permission, error)`
- **method** `AccessRepository.TenantIDs(ctx context.Context, db *gorm.DB, activeOnly bool) ([]uuid.UUID, error)`
- **type** `accessRepository struct{}`
- **func** `NewAccessRepository() AccessRepository { return &accessRepository{} }` — NewAccessRepository returns an AccessRepository.
- **func** `bumpAccessVersion(ctx context.Context, tx *gorm.DB, tenantID uuid.UUID) error` — bumpAccessVersion invalidates cached effective access for the tenant; every access write calls it in its tx.
- **func** `(r *accessRepository) AccessVersion(ctx context.Context, db *gorm.DB, tenantID uuid.UUID) (int64, error)`
- **func** `(r *accessRepository) TenantRoles(ctx context.Context, db *gorm.DB, tenantID uuid.UUID) ([]models.Role, error)`
- **func** `(r *accessRepository) RoleByTemplateKey(ctx context.Context, db *gorm.DB, tenantID uuid.UUID, key string) (*models.Role, error)`
- **func** `(r *accessRepository) SetRolePermissionScope(ctx context.Context, tx *gorm.DB, tenantID, roleID, permissionID uuid.UUID, scope string) error`
- **func** `(r *accessRepository) UserGrants(ctx context.Context, db *gorm.DB, tenantID, userID uuid.UUID) ([]models.UserPermissionGrant, error)`
- **func** `(r *accessRepository) UpsertUserGrant(ctx context.Context, tx *gorm.DB, g *models.UserPermissionGrant) error`
- **func** `(r *accessRepository) DeleteUserGrant(ctx context.Context, tx *gorm.DB, tenantID, userID, permissionID uuid.UUID) (bool, error)`
- **func** `(r *accessRepository) UserBlocks(ctx context.Context, db *gorm.DB, tenantID, userID uuid.UUID) ([]models.UserPermissionBlock, error)`
- **func** `(r *accessRepository) UpsertUserBlock(ctx context.Context, tx *gorm.DB, b *models.UserPermissionBlock) error`
- **func** `(r *accessRepository) DeleteUserBlock(ctx context.Context, tx *gorm.DB, tenantID, userID, permissionID uuid.UUID) (bool, error)`
- **func** `(r *accessRepository) AutomaticAssignments(ctx context.Context, db *gorm.DB, tenantID uuid.UUID) ([]models.UserRole, error)`
- **func** `(r *accessRepository) UpsertPermission(ctx context.Context, db *gorm.DB, p *models.Permission) error`
- **func** `(r *accessRepository) AllPermissions(ctx context.Context, db *gorm.DB) ([]models.Permission, error)`
- **func** `(r *accessRepository) TenantIDs(ctx context.Context, db *gorm.DB, activeOnly bool) ([]uuid.UUID, error)`
- **func** `deleteAccessRow(ctx context.Context, tx *gorm.DB, model interface{}, tenantID, userID, permissionID uuid.UUID) (bool, error)`

#### `internal/identity/repositories/audit_log_repository.go` (39 lines)
- **type** `auditLogRepository struct{}`
- **func** `NewAuditLogRepository() AuditLogRepository` — NewAuditLogRepository returns a AuditLogRepository.
- **func** `(r *auditLogRepository) Create(ctx context.Context, tx *gorm.DB, log *models.AuditLog) error`
- **func** `(r *auditLogRepository) FindByTenantID(ctx context.Context, db *gorm.DB, tenantID uuid.UUID, page, perPage int) ([]models.AuditLog, int64, error)`

#### `internal/identity/repositories/interfaces.go` (114 lines)
- **type** `TenantRepository interface` — TenantRepository handles CRUD for Tenants.
- **method** `TenantRepository.Create(ctx context.Context, tx *gorm.DB, tenant *models.Tenant) error`
- **method** `TenantRepository.FindByID(ctx context.Context, db *gorm.DB, id uuid.UUID) (*models.Tenant, error)`
- **method** `TenantRepository.FindBySlug(ctx context.Context, db *gorm.DB, slug string) (*models.Tenant, error)`
- **method** `TenantRepository.FindAll(ctx context.Context, db *gorm.DB, page, perPage int) ([]models.Tenant, int64, error)`
- **method** `TenantRepository.Update(ctx context.Context, tx *gorm.DB, tenant *models.Tenant) error`
- **method** `TenantRepository.Delete(ctx context.Context, tx *gorm.DB, id uuid.UUID) error`
- **type** `UserRepository interface` — UserRepository handles CRUD for Users scoped by tenant.
- **method** `UserRepository.Create(ctx context.Context, tx *gorm.DB, user *models.User) error`
- **method** `UserRepository.FindByID(ctx context.Context, db *gorm.DB, tenantID, id uuid.UUID) (*models.User, error)`
- **method** `UserRepository.FindByEmail(ctx context.Context, db *gorm.DB, tenantID uuid.UUID, email string) (*models.User, error)`
- **method** `UserRepository.FindAll(ctx context.Context, db *gorm.DB, tenantID uuid.UUID, page, perPage int) ([]models.User, int64, error)`
- **method** `UserRepository.Update(ctx context.Context, tx *gorm.DB, user *models.User) error`
- **method** `UserRepository.Delete(ctx context.Context, tx *gorm.DB, tenantID, id uuid.UUID) error`
- **type** `RoleRepository interface` — RoleRepository handles CRUD for Roles.
- **method** `RoleRepository.Create(ctx context.Context, tx *gorm.DB, role *models.Role) error`
- **method** `RoleRepository.FindByID(ctx context.Context, db *gorm.DB, tenantID, id uuid.UUID) (*models.Role, error)`
- **method** `RoleRepository.FindByName(ctx context.Context, db *gorm.DB, tenantID uuid.UUID, name string) (*models.Role, error)`
- **method** `RoleRepository.FindAll(ctx context.Context, db *gorm.DB, tenantID uuid.UUID, page, perPage int) ([]models.Role, int64, error)`
- **method** `RoleRepository.Update(ctx context.Context, tx *gorm.DB, role *models.Role) error`
- **method** `RoleRepository.Delete(ctx context.Context, tx *gorm.DB, tenantID, id uuid.UUID) error`
- **type** `PermissionRepository interface` — PermissionRepository handles CRUD for global Permissions.
- **method** `PermissionRepository.Create(ctx context.Context, tx *gorm.DB, permission *models.Permission) error`
- **method** `PermissionRepository.FindByID(ctx context.Context, db *gorm.DB, id uuid.UUID) (*models.Permission, error)`
- **method** `PermissionRepository.FindAll(ctx context.Context, db *gorm.DB, page, perPage int) ([]models.Permission, int64, error)`
- **method** `PermissionRepository.FindByIDs(ctx context.Context, db *gorm.DB, ids []uuid.UUID) ([]models.Permission, error)`
- **type** `UserRoleRepository interface` — UserRoleRepository handles role assignments to users.
- **method** `UserRoleRepository.Create(ctx context.Context, tx *gorm.DB, ur *models.UserRole) error`
- **method** `UserRoleRepository.FindByUserID(ctx context.Context, db *gorm.DB, tenantID, userID uuid.UUID) ([]models.UserRole, error)`
- **method** `UserRoleRepository.FindByUserIDs(ctx context.Context, db *gorm.DB, tenantID uuid.UUID, userIDs []uuid.UUID) ([]models.UserRole, error)` — FindByUserIDs batch-loads assignments (with Role preloaded) for many users in one query.
- **method** `UserRoleRepository.CountByRoleID(ctx context.Context, db *gorm.DB, tenantID, roleID uuid.UUID) (int64, error)` — CountByRoleID returns how many users currently hold the role.
- **method** `UserRoleRepository.Delete(ctx context.Context, tx *gorm.DB, tenantID, userID, roleID uuid.UUID) error`
- **method** `UserRoleRepository.DeleteByUserID(ctx context.Context, tx *gorm.DB, tenantID, userID uuid.UUID) error`
- **type** `RolePermissionRepository interface` — RolePermissionRepository handles permission assignments to roles.
- **method** `RolePermissionRepository.Create(ctx context.Context, tx *gorm.DB, rp *models.RolePermission) error`
- **method** `RolePermissionRepository.FindByRoleID(ctx context.Context, db *gorm.DB, tenantID, roleID uuid.UUID) ([]models.RolePermission, error)`
- **method** `RolePermissionRepository.FindByRoleIDs(ctx context.Context, db *gorm.DB, tenantID uuid.UUID, roleIDs []uuid.UUID) ([]models.RolePermission, error)` — FindByRoleIDs batch-loads assignments (with Permission preloaded) for many roles in one query.
- **method** `RolePermissionRepository.Delete(ctx context.Context, tx *gorm.DB, tenantID, roleID, permissionID uuid.UUID) error`
- **method** `RolePermissionRepository.DeleteByRoleID(ctx context.Context, tx *gorm.DB, tenantID, roleID uuid.UUID) error`
- **type** `SessionRepository interface` — SessionRepository tracks login sessions.
- **method** `SessionRepository.Create(ctx context.Context, tx *gorm.DB, session *models.Session) error`
- **method** `SessionRepository.FindByID(ctx context.Context, db *gorm.DB, id uuid.UUID) (*models.Session, error)`
- **method** `SessionRepository.RevokeByID(ctx context.Context, tx *gorm.DB, id uuid.UUID) error`
- **method** `SessionRepository.RevokeAllByUserID(ctx context.Context, tx *gorm.DB, tenantID, userID uuid.UUID) error`
- **type** `RefreshTokenRepository interface` — RefreshTokenRepository tracks refresh tokens.
- **method** `RefreshTokenRepository.Create(ctx context.Context, tx *gorm.DB, rt *models.RefreshToken) error`
- **method** `RefreshTokenRepository.FindByTokenHash(ctx context.Context, db *gorm.DB, hash string) (*models.RefreshToken, error)`
- **method** `RefreshTokenRepository.RevokeByID(ctx context.Context, tx *gorm.DB, id uuid.UUID) error`
- **method** `RefreshTokenRepository.RevokeBySessionID(ctx context.Context, tx *gorm.DB, sessionID uuid.UUID) error` — RevokeBySessionID revokes every still-active refresh token issued for the session (logout).
- **method** `RefreshTokenRepository.RevokeAllByUserID(ctx context.Context, tx *gorm.DB, tenantID, userID uuid.UUID) error`
- **type** `PasswordResetTokenRepository interface` — PasswordResetTokenRepository tracks password resets.
- **method** `PasswordResetTokenRepository.Create(ctx context.Context, tx *gorm.DB, prt *models.PasswordResetToken) error`
- **method** `PasswordResetTokenRepository.FindByTokenHash(ctx context.Context, db *gorm.DB, hash string) (*models.PasswordResetToken, error)`
- **method** `PasswordResetTokenRepository.MarkAsUsed(ctx context.Context, tx *gorm.DB, id uuid.UUID) error`
- **type** `MFAConfigRepository interface` — MFAConfigRepository configures MFA.
- **method** `MFAConfigRepository.Create(ctx context.Context, tx *gorm.DB, mfa *models.MFAConfig) error`
- **method** `MFAConfigRepository.FindByUserIDAndType(ctx context.Context, db *gorm.DB, tenantID, userID uuid.UUID, mfaType string) (*models.MFAConfig, error)`
- **method** `MFAConfigRepository.Update(ctx context.Context, tx *gorm.DB, mfa *models.MFAConfig) error`
- **type** `TenantSettingsRepository interface` — TenantSettingsRepository handles tenant config key-values.
- **method** `TenantSettingsRepository.Upsert(ctx context.Context, tx *gorm.DB, setting *models.TenantSettings) error`
- **method** `TenantSettingsRepository.FindByTenantID(ctx context.Context, db *gorm.DB, tenantID uuid.UUID) ([]models.TenantSettings, error)`
- **method** `TenantSettingsRepository.FindByKey(ctx context.Context, db *gorm.DB, tenantID uuid.UUID, key string) (*models.TenantSettings, error)`
- **type** `AuditLogRepository interface` — AuditLogRepository records security logs (append-only).
- **method** `AuditLogRepository.Create(ctx context.Context, tx *gorm.DB, log *models.AuditLog) error`
- **method** `AuditLogRepository.FindByTenantID(ctx context.Context, db *gorm.DB, tenantID uuid.UUID, page, perPage int) ([]models.AuditLog, int64, error)`

#### `internal/identity/repositories/invitation_repository.go` (75 lines)
- **type** `InvitationRepository interface` — InvitationRepository persists user invitations.
- **method** `InvitationRepository.Create(ctx context.Context, tx *gorm.DB, inv *models.Invitation) error`
- **method** `InvitationRepository.FindByID(ctx context.Context, db *gorm.DB, tenantID, id uuid.UUID) (*models.Invitation, error)`
- **method** `InvitationRepository.FindByTokenHash(ctx context.Context, db *gorm.DB, hash string) (*models.Invitation, error)`
- **method** `InvitationRepository.LatestForUser(ctx context.Context, db *gorm.DB, tenantID, userID uuid.UUID) (*models.Invitation, error)`
- **method** `InvitationRepository.List(ctx context.Context, db *gorm.DB, tenantID uuid.UUID, pendingOnly bool, now time.Time) ([]models.Invitation, error)`
- **method** `InvitationRepository.MarkAccepted(ctx context.Context, tx *gorm.DB, id uuid.UUID, at time.Time) error`
- **method** `InvitationRepository.RevokeOpenForUser(ctx context.Context, tx *gorm.DB, tenantID, userID uuid.UUID, at time.Time) error`
- **type** `invitationRepository struct{}`
- **func** `NewInvitationRepository() InvitationRepository { return invitationRepository{} }` — NewInvitationRepository returns an InvitationRepository.
- **func** `(invitationRepository) Create(ctx context.Context, tx *gorm.DB, inv *models.Invitation) error`
- **func** `firstInvitation(q *gorm.DB) (*models.Invitation, error)`
- **func** `(invitationRepository) FindByID(ctx context.Context, db *gorm.DB, tenantID, id uuid.UUID) (*models.Invitation, error)`
- **func** `(invitationRepository) FindByTokenHash(ctx context.Context, db *gorm.DB, hash string) (*models.Invitation, error)`
- **func** `(invitationRepository) LatestForUser(ctx context.Context, db *gorm.DB, tenantID, userID uuid.UUID) (*models.Invitation, error)`
- **func** `(invitationRepository) List(ctx context.Context, db *gorm.DB, tenantID uuid.UUID, pendingOnly bool, now time.Time) ([]models.Invitation, error)`
- **func** `(invitationRepository) MarkAccepted(ctx context.Context, tx *gorm.DB, id uuid.UUID, at time.Time) error`
- **func** `(invitationRepository) RevokeOpenForUser(ctx context.Context, tx *gorm.DB, tenantID, userID uuid.UUID, at time.Time) error`

#### `internal/identity/repositories/mfa_config_repository.go` (40 lines)
- **type** `mfaConfigRepository struct{}`
- **func** `NewMFAConfigRepository() MFAConfigRepository` — NewMFAConfigRepository returns a MFAConfigRepository.
- **func** `(r *mfaConfigRepository) Create(ctx context.Context, tx *gorm.DB, mfa *models.MFAConfig) error`
- **func** `(r *mfaConfigRepository) FindByUserIDAndType(ctx context.Context, db *gorm.DB, tenantID, userID uuid.UUID, mfaType string) (*models.MFAConfig, error)`
- **func** `(r *mfaConfigRepository) Update(ctx context.Context, tx *gorm.DB, mfa *models.MFAConfig) error`

#### `internal/identity/repositories/password_reset_repository.go` (42 lines)
- **type** `passwordResetTokenRepository struct{}`
- **func** `NewPasswordResetTokenRepository() PasswordResetTokenRepository` — NewPasswordResetTokenRepository returns a PasswordResetTokenRepository.
- **func** `(r *passwordResetTokenRepository) Create(ctx context.Context, tx *gorm.DB, prt *models.PasswordResetToken) error`
- **func** `(r *passwordResetTokenRepository) FindByTokenHash(ctx context.Context, db *gorm.DB, hash string) (*models.PasswordResetToken, error)`
- **func** `(r *passwordResetTokenRepository) MarkAsUsed(ctx context.Context, tx *gorm.DB, id uuid.UUID) error`

#### `internal/identity/repositories/permission_repository.go` (60 lines)
- **type** `permissionRepository struct{}`
- **func** `NewPermissionRepository() PermissionRepository` — NewPermissionRepository returns a PermissionRepository.
- **func** `(r *permissionRepository) Create(ctx context.Context, tx *gorm.DB, permission *models.Permission) error`
- **func** `(r *permissionRepository) FindByID(ctx context.Context, db *gorm.DB, id uuid.UUID) (*models.Permission, error)`
- **func** `(r *permissionRepository) FindAll(ctx context.Context, db *gorm.DB, page, perPage int) ([]models.Permission, int64, error)`
- **func** `(r *permissionRepository) FindByIDs(ctx context.Context, db *gorm.DB, ids []uuid.UUID) ([]models.Permission, error)`

#### `internal/identity/repositories/role_assignment_repository.go` (138 lines)
- **type** `userRoleRepository struct{}`
- **func** `NewUserRoleRepository() UserRoleRepository` — NewUserRoleRepository returns a UserRoleRepository.
- **func** `(r *userRoleRepository) Create(ctx context.Context, tx *gorm.DB, ur *models.UserRole) error` — Create assigns a role; the source defaults to manual.
- **func** `(r *userRoleRepository) FindByUserID(ctx context.Context, db *gorm.DB, tenantID, userID uuid.UUID) ([]models.UserRole, error)`
- **func** `(r *userRoleRepository) FindByUserIDs(ctx context.Context, db *gorm.DB, tenantID uuid.UUID, userIDs []uuid.UUID) ([]models.UserRole, error)`
- **func** `(r *userRoleRepository) CountByRoleID(ctx context.Context, db *gorm.DB, tenantID, roleID uuid.UUID) (int64, error)`
- **func** `(r *userRoleRepository) Delete(ctx context.Context, tx *gorm.DB, tenantID, userID, roleID uuid.UUID) error`
- **func** `(r *userRoleRepository) DeleteByUserID(ctx context.Context, tx *gorm.DB, tenantID, userID uuid.UUID) error`
- **type** `rolePermissionRepository struct{}`
- **func** `NewRolePermissionRepository() RolePermissionRepository` — NewRolePermissionRepository returns a RolePermissionRepository.
- **func** `(r *rolePermissionRepository) Create(ctx context.Context, tx *gorm.DB, rp *models.RolePermission) error`
- **func** `(r *rolePermissionRepository) FindByRoleID(ctx context.Context, db *gorm.DB, tenantID, roleID uuid.UUID) ([]models.RolePermission, error)`
- **func** `(r *rolePermissionRepository) FindByRoleIDs(ctx context.Context, db *gorm.DB, tenantID uuid.UUID, roleIDs []uuid.UUID) ([]models.RolePermission, error)`
- **func** `(r *rolePermissionRepository) Delete(ctx context.Context, tx *gorm.DB, tenantID, roleID, permissionID uuid.UUID) error`
- **func** `(r *rolePermissionRepository) DeleteByRoleID(ctx context.Context, tx *gorm.DB, tenantID, roleID uuid.UUID) error`

#### `internal/identity/repositories/role_repository.go` (80 lines)
- **type** `roleRepository struct{}`
- **func** `NewRoleRepository() RoleRepository` — NewRoleRepository returns a RoleRepository.
- **func** `(r *roleRepository) Create(ctx context.Context, tx *gorm.DB, role *models.Role) error`
- **func** `(r *roleRepository) FindByID(ctx context.Context, db *gorm.DB, tenantID, id uuid.UUID) (*models.Role, error)`
- **func** `(r *roleRepository) FindByName(ctx context.Context, db *gorm.DB, tenantID uuid.UUID, name string) (*models.Role, error)`
- **func** `(r *roleRepository) FindAll(ctx context.Context, db *gorm.DB, tenantID uuid.UUID, page, perPage int) ([]models.Role, int64, error)`
- **func** `(r *roleRepository) Update(ctx context.Context, tx *gorm.DB, role *models.Role) error`
- **func** `(r *roleRepository) Delete(ctx context.Context, tx *gorm.DB, tenantID, id uuid.UUID) error`

#### `internal/identity/repositories/session_repository.go` (95 lines)
- **type** `sessionRepository struct{}`
- **func** `NewSessionRepository() SessionRepository` — NewSessionRepository returns a SessionRepository.
- **func** `(r *sessionRepository) Create(ctx context.Context, tx *gorm.DB, session *models.Session) error`
- **func** `(r *sessionRepository) FindByID(ctx context.Context, db *gorm.DB, id uuid.UUID) (*models.Session, error)`
- **func** `(r *sessionRepository) RevokeByID(ctx context.Context, tx *gorm.DB, id uuid.UUID) error`
- **func** `(r *sessionRepository) RevokeAllByUserID(ctx context.Context, tx *gorm.DB, tenantID, userID uuid.UUID) error`
- **type** `refreshTokenRepository struct{}`
- **func** `NewRefreshTokenRepository() RefreshTokenRepository` — NewRefreshTokenRepository returns a RefreshTokenRepository.
- **func** `(r *refreshTokenRepository) Create(ctx context.Context, tx *gorm.DB, rt *models.RefreshToken) error`
- **func** `(r *refreshTokenRepository) FindByTokenHash(ctx context.Context, db *gorm.DB, hash string) (*models.RefreshToken, error)`
- **func** `(r *refreshTokenRepository) RevokeByID(ctx context.Context, tx *gorm.DB, id uuid.UUID) error`
- **func** `(r *refreshTokenRepository) RevokeAllByUserID(ctx context.Context, tx *gorm.DB, tenantID, userID uuid.UUID) error`
- **func** `(r *refreshTokenRepository) RevokeBySessionID(ctx context.Context, tx *gorm.DB, sessionID uuid.UUID) error`

#### `internal/identity/repositories/tenant_repository.go` (71 lines)
- **type** `tenantRepository struct{}`
- **func** `NewTenantRepository() TenantRepository` — NewTenantRepository returns a TenantRepository.
- **func** `(r *tenantRepository) Create(ctx context.Context, tx *gorm.DB, tenant *models.Tenant) error`
- **func** `(r *tenantRepository) FindByID(ctx context.Context, db *gorm.DB, id uuid.UUID) (*models.Tenant, error)`
- **func** `(r *tenantRepository) FindBySlug(ctx context.Context, db *gorm.DB, slug string) (*models.Tenant, error)`
- **func** `(r *tenantRepository) FindAll(ctx context.Context, db *gorm.DB, page, perPage int) ([]models.Tenant, int64, error)`
- **func** `(r *tenantRepository) Update(ctx context.Context, tx *gorm.DB, tenant *models.Tenant) error`
- **func** `(r *tenantRepository) Delete(ctx context.Context, tx *gorm.DB, id uuid.UUID) error`

#### `internal/identity/repositories/tenant_settings_repository.go` (46 lines)
- **type** `tenantSettingsRepository struct{}`
- **func** `NewTenantSettingsRepository() TenantSettingsRepository` — NewTenantSettingsRepository returns a TenantSettingsRepository.
- **func** `(r *tenantSettingsRepository) Upsert(ctx context.Context, tx *gorm.DB, setting *models.TenantSettings) error`
- **func** `(r *tenantSettingsRepository) FindByTenantID(ctx context.Context, db *gorm.DB, tenantID uuid.UUID) ([]models.TenantSettings, error)`
- **func** `(r *tenantSettingsRepository) FindByKey(ctx context.Context, db *gorm.DB, tenantID uuid.UUID, key string) (*models.TenantSettings, error)`

#### `internal/identity/repositories/user_repository.go` (72 lines)
- **type** `userRepository struct{}`
- **func** `NewUserRepository() UserRepository` — NewUserRepository returns a UserRepository.
- **func** `(r *userRepository) Create(ctx context.Context, tx *gorm.DB, user *models.User) error`
- **func** `(r *userRepository) FindByID(ctx context.Context, db *gorm.DB, tenantID, id uuid.UUID) (*models.User, error)`
- **func** `(r *userRepository) FindByEmail(ctx context.Context, db *gorm.DB, tenantID uuid.UUID, email string) (*models.User, error)`
- **func** `(r *userRepository) FindAll(ctx context.Context, db *gorm.DB, tenantID uuid.UUID, page, perPage int) ([]models.User, int64, error)`
- **func** `(r *userRepository) Update(ctx context.Context, tx *gorm.DB, user *models.User) error`
- **func** `(r *userRepository) Delete(ctx context.Context, tx *gorm.DB, tenantID, id uuid.UUID) error`

#### `internal/identity/routes/routes.go` (228 lines)
- **func** `RegisterRoutes(` — RegisterRoutes sets up routing groups, initializes middlewares, and binds controller paths.

#### `internal/identity/services/access_explain.go` (71 lines)
- **func** `(s *accessService) Explain(ctx context.Context, db *gorm.DB, tenantID, userID uuid.UUID) (*dto.ExplainAccessResponse, error)` — Explain lists every permission the user holds or is blocked from, with scope and the role or grant behind it.
- **func** `(s *accessService) catalogueMeta(ctx context.Context, db *gorm.DB) (map[string]models.Permission, error)`
- **func** `explained(e *access.Entry, p models.Permission) dto.ExplainedPermission`

#### `internal/identity/services/access_resolver.go` (212 lines)
- **type** `AccessCache interface` — AccessCache stores resolved access sets; implementations must treat errors as misses.
- **method** `AccessCache.Get(ctx context.Context, key string) (string, bool, error)`
- **method** `AccessCache.Set(ctx context.Context, key string, value string, ttl time.Duration) error`
- **type** `AccessResolver interface` — AccessResolver computes a user's effective access (docs/ARCHITECTURE.md §17).
- **method** `AccessResolver.Effective(ctx context.Context, db *gorm.DB, tenantID, userID uuid.UUID) (access.Effective, error)` — Effective resolves through db (pass the transaction when deciding inside one; it bypasses the cache).
- **method** `AccessResolver.ScopeFor(ctx context.Context, tenantID, userID uuid.UUID, resource, action string) (access.Scope, error)` — ScopeFor returns the widest scope held for resource:action, using the cache.
- **method** `AccessResolver.RoleGrants(ctx context.Context, db *gorm.DB, tenantID uuid.UUID, roleIDs []uuid.UUID) (access.Effective, error)` — RoleGrants resolves what holding roleIDs (with their ancestors) grants, for escalation checks.
- **type** `AccessResolverDeps struct` — AccessResolverDeps are the resolver's collaborators.
- **type** `accessResolver struct{ d AccessResolverDeps }`
- **func** `NewAccessResolver(d AccessResolverDeps) AccessResolver` — NewAccessResolver builds an AccessResolver.
- **func** `(r *accessResolver) ScopeFor(ctx context.Context, tenantID, userID uuid.UUID, resource, action string) (access.Scope, error)`
- **func** `(r *accessResolver) cached(ctx context.Context, tenantID, userID uuid.UUID) (access.Effective, error)`
- **func** `(r *accessResolver) Effective(ctx context.Context, db *gorm.DB, tenantID, userID uuid.UUID) (access.Effective, error)`
- **func** `(r *accessResolver) resolve(ctx context.Context, db *gorm.DB, tenantID, userID uuid.UUID) (access.Effective, time.Duration, error)` — resolve loads the rows, resolves them, and returns how long the result stays valid (earliest grant expiry).
- **func** `(r *accessResolver) input(ctx context.Context, db *gorm.DB, tenantID, userID uuid.UUID) (access.Input, error)`
- **func** `roleIDsOf(urs []models.UserRole) []uuid.UUID`
- **func** `(r *accessResolver) RoleGrants(ctx context.Context, db *gorm.DB, tenantID uuid.UUID, roleIDs []uuid.UUID) (access.Effective, error)`
- **func** `(r *accessResolver) roleNodes(ctx context.Context, db *gorm.DB, tenantID uuid.UUID, fallback []uuid.UUID) ([]access.RoleNode, error)` — roleNodes loads every live role of the tenant with its own grants (two queries). Without an access repository only the given roles are known (no inheritance).
- **func** `grantOf(p *models.Permission, scope string) access.Grant`

#### `internal/identity/services/access_service.go` (247 lines)
- **type** `AccessService interface` — AccessService manages per-user grants and blocks and explains effective access (docs/ARCHITECTURE.md §17).
- **method** `AccessService.ListGrants(ctx context.Context, db *gorm.DB, tenantID, userID uuid.UUID) ([]dto.UserGrantResponse, error)`
- **method** `AccessService.Grant(ctx context.Context, tx *gorm.DB, tenantID, userID uuid.UUID, req dto.UserGrantRequest) (*dto.UserGrantResponse, error)`
- **method** `AccessService.RevokeGrant(ctx context.Context, tx *gorm.DB, tenantID, userID, permissionID uuid.UUID) error`
- **method** `AccessService.ListBlocks(ctx context.Context, db *gorm.DB, tenantID, userID uuid.UUID) ([]dto.UserBlockResponse, error)`
- **method** `AccessService.Block(ctx context.Context, tx *gorm.DB, tenantID, userID uuid.UUID, req dto.UserBlockRequest) (*dto.UserBlockResponse, error)`
- **method** `AccessService.Unblock(ctx context.Context, tx *gorm.DB, tenantID, userID, permissionID uuid.UUID) error`
- **method** `AccessService.Explain(ctx context.Context, db *gorm.DB, tenantID, userID uuid.UUID) (*dto.ExplainAccessResponse, error)`
- **type** `AccessServiceDeps struct` — AccessServiceDeps are the access service's collaborators.
- **type** `accessService struct{ d AccessServiceDeps }`
- **func** `NewAccessService(d AccessServiceDeps) AccessService` — NewAccessService builds an AccessService.
- **func** `(s *accessService) ensureUser(ctx context.Context, db *gorm.DB, tenantID, userID uuid.UUID) error`
- **func** `(s *accessService) permission(ctx context.Context, db *gorm.DB, raw string) (*models.Permission, error)`
- **func** `callerID(ctx context.Context) *uuid.UUID`
- **func** `(s *accessService) Grant(ctx context.Context, tx *gorm.DB, tenantID, userID uuid.UUID, req dto.UserGrantRequest) (*dto.UserGrantResponse, error)`
- **func** `(s *accessService) audit(ctx context.Context, tx *gorm.DB, tenantID uuid.UUID, action string, userID uuid.UUID, detail map[string]string)`
- **func** `(s *accessService) RevokeGrant(ctx context.Context, tx *gorm.DB, tenantID, userID, permissionID uuid.UUID) error`
- **func** `(s *accessService) ListGrants(ctx context.Context, db *gorm.DB, tenantID, userID uuid.UUID) ([]dto.UserGrantResponse, error)`
- **func** `(s *accessService) grantResponse(g models.UserPermissionGrant) *dto.UserGrantResponse`
- **func** `(s *accessService) Block(ctx context.Context, tx *gorm.DB, tenantID, userID uuid.UUID, req dto.UserBlockRequest) (*dto.UserBlockResponse, error)`
- **func** `(s *accessService) ensureNotTenantAdmin(ctx context.Context, tx *gorm.DB, tenantID, userID uuid.UUID) error` — ensureNotTenantAdmin rejects blocks on tenant_admin holders: they bypass permission checks, so a block would silently do nothing.
- **func** `(s *accessService) Unblock(ctx context.Context, tx *gorm.DB, tenantID, userID, permissionID uuid.UUID) error`
- **func** `(s *accessService) ListBlocks(ctx context.Context, db *gorm.DB, tenantID, userID uuid.UUID) ([]dto.UserBlockResponse, error)`
- **func** `blockResponse(b models.UserPermissionBlock) *dto.UserBlockResponse`

#### `internal/identity/services/audit_service.go` (118 lines)
- **type** `AuditService interface` — AuditService defines the interface to write immutable security logs.
- **method** `AuditService.Log(ctx context.Context, tx *gorm.DB, tenantID, userID string, action, resource, resourceID string, metadata interface{}, ipAddress, userAgent string) error`
- **method** `AuditService.List(ctx context.Context, db *gorm.DB, tenantID uuid.UUID, page, perPage int) (*dto.AuditLogListResponse, error)`
- **type** `auditService struct`
- **func** `NewAuditService(auditRepo repositories.AuditLogRepository, logger *zerolog.Logger) AuditService` — NewAuditService creates a new AuditService instance.
- **func** `(s *auditService) Log(ctx context.Context, tx *gorm.DB, tenantID, userID string, action, resource, resourceID string, metadata interface{}, ipAddress, userAgent string) error`
- **func** `(s *auditService) List(ctx context.Context, db *gorm.DB, tenantID uuid.UUID, page, perPage int) (*dto.AuditLogListResponse, error)` — List returns the tenant's audit trail, newest first.

#### `internal/identity/services/auth_service.go` (515 lines)
- **type** `AuthService interface` — AuthService handles identity validation use cases.
- **method** `AuthService.Login(ctx context.Context, tx *gorm.DB, req dto.LoginRequest, ipAddress, userAgent string, correlationID uuid.UUID) (*dto.LoginResponse, error)`
- **method** `AuthService.Logout(ctx context.Context, tx *gorm.DB, tenantID, userID, sessionID uuid.UUID, correlationID uuid.UUID) error`
- **method** `AuthService.RefreshToken(ctx context.Context, tx *gorm.DB, req dto.RefreshTokenRequest, correlationID uuid.UUID) (*dto.RefreshTokenResponse, error)`
- **method** `AuthService.ForgotPassword(ctx context.Context, tx *gorm.DB, req dto.ForgotPasswordRequest, correlationID uuid.UUID) error`
- **method** `AuthService.ResetPassword(ctx context.Context, tx *gorm.DB, req dto.ResetPasswordRequest, correlationID uuid.UUID) error`
- **type** `authService struct`
- **func** `NewAuthService(` — NewAuthService creates a new AuthService.
- **func** `(s *authService) Login(ctx context.Context, tx *gorm.DB, req dto.LoginRequest, ipAddress, userAgent string, correlationID uuid.UUID) (*dto.LoginResponse, error)`
- **func** `(s *authService) Logout(ctx context.Context, tx *gorm.DB, tenantID, userID, sessionID uuid.UUID, correlationID uuid.UUID) error`
- **func** `(s *authService) RefreshToken(ctx context.Context, tx *gorm.DB, req dto.RefreshTokenRequest, correlationID uuid.UUID) (*dto.RefreshTokenResponse, error)`
- **func** `(s *authService) ForgotPassword(ctx context.Context, tx *gorm.DB, req dto.ForgotPasswordRequest, correlationID uuid.UUID) error`
- **func** `(s *authService) ResetPassword(ctx context.Context, tx *gorm.DB, req dto.ResetPasswordRequest, correlationID uuid.UUID) error`
- **func** `dummyPasswordHash() []byte` — dummyPasswordHash lazily builds a bcrypt (cost 12, same as real hashes) hash used to equalise login timing.
- **type** `AuthOption func(*authService)` — AuthOption customises an AuthService.
- **func** `WithAuthEvents(sink EventSink) AuthOption { return func(s *authService) { s.sink = sink } }` — WithAuthEvents records auth events through sink (the transactional outbox in production).
- **func** `WithSecretDelivery(d SecretDelivery) AuthOption { return func(s *authService) { s.secrets = d } }` — WithSecretDelivery sends one-time secrets (password-reset links) to users; without it nothing is sent.

#### `internal/identity/services/automatic_roles.go` (206 lines)
- **type** `AutomaticRoleSource interface` — AutomaticRoleSource reports, from HR data, who qualifies for the automatic roles: employees (an active employee record) and managers (at least one active direct report). M0 keeps no copy of the org chart.
- **method** `AutomaticRoleSource.Members(ctx context.Context, tenantID uuid.UUID) (employees, managers []uuid.UUID, err error)`
- **method** `AutomaticRoleSource.MemberOf(ctx context.Context, tenantID, userID uuid.UUID) (employee, manager bool, err error)`
- **type** `AutomaticRoles interface` — AutomaticRoles keeps the automatic employee and manager role assignments in line with HR data.
- **method** `AutomaticRoles.SetSource(src AutomaticRoleSource)` — SetSource installs the HR-backed source; until then syncing is a no-op.
- **method** `AutomaticRoles.SyncTenant(ctx context.Context, tenantID uuid.UUID) (added, removed int, err error)`
- **method** `AutomaticRoles.SyncUser(ctx context.Context, tenantID, userID uuid.UUID) error`
- **method** `AutomaticRoles.Run(ctx context.Context, interval time.Duration)` — Run sweeps every tenant each interval until ctx is cancelled.
- **type** `AutomaticRolesDeps struct` — AutomaticRolesDeps are the reconciler's collaborators.
- **type** `automaticRoles struct`
- **func** `NewAutomaticRoles(d AutomaticRolesDeps) AutomaticRoles` — NewAutomaticRoles builds the reconciler.
- **func** `(a *automaticRoles) SetSource(src AutomaticRoleSource) { a.src = src }`
- **func** `(a *automaticRoles) templateRoles(ctx context.Context, tx *gorm.DB, tenantID uuid.UUID) (employee, manager uuid.UUID, err error)` — templateRoles returns the tenant's automatic role IDs, provisioning predefined roles if missing.
- **func** `(a *automaticRoles) SyncTenant(ctx context.Context, tenantID uuid.UUID) (added, removed int, err error)`
- **func** `(a *automaticRoles) reconcile(ctx context.Context, tx *gorm.DB, tenantID uuid.UUID, want map[[2]uuid.UUID]bool, current []models.UserRole, autoRoles map[uuid.UUID]bool) (int, int, error)` — reconcile adds wanted automatic assignments and removes stale ones among the automatic roles.
- **func** `(a *automaticRoles) SyncUser(ctx context.Context, tenantID, userID uuid.UUID) error`
- **func** `(a *automaticRoles) Run(ctx context.Context, interval time.Duration)`
- **func** `(a *automaticRoles) sweep(ctx context.Context)`
- **func** `(a *automaticRoles) warn(err error, tenantID uuid.UUID)`

#### `internal/identity/services/catalogue.go` (191 lines)
- **type** `Transactor func(ctx context.Context, fn func(tx *gorm.DB) error) error` — Transactor runs fn in one database transaction.
- **func** `dbTransactor(db *gorm.DB) Transactor`
- **type** `Catalogue interface` — Catalogue registers the permission catalogue and keeps every tenant's predefined roles in sync (docs/ARCHITECTURE.md §17).
- **method** `Catalogue.Register(ctx context.Context, defs []access.PermissionDef) error` — Register validates and upserts permission definitions and remembers their predefined-role defaults.
- **method** `Catalogue.ProvisionTenant(ctx context.Context, tx *gorm.DB, tenantID uuid.UUID) error` — ProvisionTenant creates missing predefined roles and adds default grants to roles still template-managed.
- **method** `Catalogue.SyncAllTenants(ctx context.Context) error` — SyncAllTenants provisions every tenant (boot).
- **type** `CatalogueDeps struct` — CatalogueDeps are the catalogue's collaborators.
- **type** `catalogue struct`
- **func** `NewCatalogue(d CatalogueDeps) Catalogue` — NewCatalogue builds a Catalogue.
- **func** `(c *catalogue) Register(ctx context.Context, defs []access.PermissionDef) error`
- **func** `(c *catalogue) registered() []access.PermissionDef`
- **func** `(c *catalogue) SyncAllTenants(ctx context.Context) error`
- **func** `(c *catalogue) ProvisionTenant(ctx context.Context, tx *gorm.DB, tenantID uuid.UUID) error`
- **func** `(c *catalogue) createTemplateRole(ctx context.Context, tx *gorm.DB, tenantID uuid.UUID, tpl access.Template, byKey map[string]*models.Role, names map[string]bool) (*models.Role, error)`
- **func** `(c *catalogue) syncDefaults(ctx context.Context, tx *gorm.DB, role *models.Role, want []access.Grant, permIDs map[string]uuid.UUID) error` — syncDefaults adds missing default grants and resets drifted scopes on a template-managed role.
- **func** `(c *catalogue) permissionIDs(ctx context.Context, db *gorm.DB) (map[string]uuid.UUID, error)`

#### `internal/identity/services/event_sink.go` (36 lines)
- **type** `EventSink interface` — EventSink records an identity event. The production sink writes it to the outbox inside the caller's transaction, so the event exists if and only if the change committed (docs/ARCHITECTURE.md §3.7).
- **method** `EventSink.Emit(ctx context.Context, tx *gorm.DB, evt events.Event) error`
- **type** `outboxSink struct{ store outbox.Store }`
- **func** `NewOutboxSink(store outbox.Store) EventSink { return outboxSink{store: store} }` — NewOutboxSink returns the transactional sink over the identity outbox table.
- **func** `(s outboxSink) Emit(ctx context.Context, tx *gorm.DB, evt events.Event) error`
- **type** `publisherSink struct{ pub queue.EventPublisher }` — publisherSink publishes directly, best effort; used only where no outbox is wired (unit tests, tools).
- **func** `(s publisherSink) Emit(ctx context.Context, _ *gorm.DB, evt events.Event) error`

#### `internal/identity/services/grants.go` (106 lines)
- **type** `Caller struct` — Caller identifies the authenticated principal performing a request. Controllers attach it to the request context so services can enforce "no privilege escalation": nobody may hand out access they do not hold themselves.
- **func** `(c Caller) IsTenantAdmin() bool` — IsTenantAdmin reports whether the caller holds the all-powerful tenant_admin system role.
- **type** `callerKey struct{}`
- **func** `WithCaller(ctx context.Context, c Caller) context.Context` — WithCaller returns a context carrying the authenticated caller.
- **func** `CallerFrom(ctx context.Context) (Caller, bool)` — CallerFrom extracts the caller; ok is false for system-initiated (internal) operations, which are not restricted.
- **func** `forbiddenGrant(msg string) error`
- **func** `restricted(ctx context.Context) (Caller, bool)` — restricted reports whether the escalation rules apply: a real caller who is not tenant_admin.
- **func** `ensureCanGrant(ctx context.Context, db *gorm.DB, r AccessResolver, tenantID uuid.UUID, want []access.Grant) error` — ensureCanGrant refuses to let a non-admin caller hand out any grant wider than one they hold themselves (docs/ARCHITECTURE.md §17.2).
- **func** `ensureCanAssignRoles(ctx context.Context, db *gorm.DB, r AccessResolver, tenantID uuid.UUID, roles []*models.Role) error` — ensureCanAssignRoles refuses to let a non-admin caller assign tenant_admin, or any role (with its inherited grants) reaching further than the caller's own access.
- **func** `ensureCanGrantRoles(ctx context.Context, db *gorm.DB, r AccessResolver, tenantID uuid.UUID, roleIDs []uuid.UUID) error` — ensureCanGrantRoles checks the effective grants of roleIDs (e.g. a new parent) against the caller.

#### `internal/identity/services/invitation_service.go` (197 lines)
- **type** `InvitationService interface` — InvitationService issues, lists, resends, revokes and accepts user invitations (M0 plan B3).
- **method** `InvitationService.Issue(ctx context.Context, tx *gorm.DB, user *models.User) (*models.Invitation, error)`
- **method** `InvitationService.Latest(ctx context.Context, db *gorm.DB, tenantID, userID uuid.UUID) (*dto.InvitationResponse, error)`
- **method** `InvitationService.List(ctx context.Context, db *gorm.DB, tenantID uuid.UUID, pendingOnly bool) ([]dto.InvitationResponse, error)`
- **method** `InvitationService.Resend(ctx context.Context, tx *gorm.DB, tenantID, id uuid.UUID) (*dto.InvitationResponse, error)`
- **method** `InvitationService.Revoke(ctx context.Context, tx *gorm.DB, tenantID, id uuid.UUID) error`
- **method** `InvitationService.Accept(ctx context.Context, tx *gorm.DB, req dto.AcceptInvitationRequest, correlationID uuid.UUID) error`
- **type** `InvitationDeps struct` — InvitationDeps are the invitation service's collaborators.
- **type** `invitationService struct{ d InvitationDeps }`
- **func** `NewInvitationService(d InvitationDeps) InvitationService` — NewInvitationService builds an InvitationService.
- **func** `(s *invitationService) Issue(ctx context.Context, tx *gorm.DB, user *models.User) (*models.Invitation, error)` — Issue revokes any open invitation of the user, stores a new hashed token and delivers the raw token.
- **func** `(s *invitationService) respond(inv *models.Invitation) *dto.InvitationResponse`
- **func** `(s *invitationService) Latest(ctx context.Context, db *gorm.DB, tenantID, userID uuid.UUID) (*dto.InvitationResponse, error)`
- **func** `(s *invitationService) List(ctx context.Context, db *gorm.DB, tenantID uuid.UUID, pendingOnly bool) ([]dto.InvitationResponse, error)`
- **func** `(s *invitationService) openInvitation(ctx context.Context, tx *gorm.DB, tenantID, id uuid.UUID) (*models.Invitation, *models.User, error)` — openInvitation loads an invitation that has not been accepted and whose user is still invited.
- **func** `(s *invitationService) Resend(ctx context.Context, tx *gorm.DB, tenantID, id uuid.UUID) (*dto.InvitationResponse, error)`
- **func** `(s *invitationService) Revoke(ctx context.Context, tx *gorm.DB, tenantID, id uuid.UUID) error`
- **func** `idString(id *uuid.UUID) string`
- **func** `(s *invitationService) Accept(ctx context.Context, tx *gorm.DB, req dto.AcceptInvitationRequest, correlationID uuid.UUID) error` — Accept sets the password and activates the invited user; the token is single use.

#### `internal/identity/services/permission_service.go` (55 lines)
- **type** `PermissionService interface` — PermissionService lists global privilege mappings.
- **method** `PermissionService.ListPermissions(ctx context.Context, db *gorm.DB, page, perPage int) (*dto.PermissionListResponse, error)`
- **type** `permissionService struct`
- **func** `NewPermissionService(permRepo repositories.PermissionRepository) PermissionService` — NewPermissionService creates a new PermissionService.
- **func** `(s *permissionService) ListPermissions(ctx context.Context, db *gorm.DB, page, perPage int) (*dto.PermissionListResponse, error)`

#### `internal/identity/services/role_assign.go` (165 lines)
- **func** `(s *roleService) rolesForAssignment(ctx context.Context, tx *gorm.DB, tenantID uuid.UUID, raw []string) ([]*models.Role, error)`
- **func** `(s *roleService) AssignRolesToUser(ctx context.Context, tx *gorm.DB, tenantID, userID uuid.UUID, req dto.AssignRoleRequest, correlationID uuid.UUID) error`
- **func** `(s *roleService) RemoveRoleFromUser(ctx context.Context, tx *gorm.DB, tenantID, userID, roleID uuid.UUID) error`
- **func** `(s *roleService) assignment(ctx context.Context, tx *gorm.DB, tenantID, userID, roleID uuid.UUID) (*models.UserRole, error)`
- **func** `(s *roleService) GetMyAccess(ctx context.Context, db *gorm.DB, tenantID, userID uuid.UUID) (*dto.AccessResponse, error)`

#### `internal/identity/services/role_grants.go` (236 lines)
- **type** `grantSpec struct` — grantSpec is a validated permission + scope to store on a role.
- **func** `grantsOf(specs []grantSpec) []access.Grant`
- **func** `validationError(msg string) error`
- **func** `(s *roleService) parseGrants(ctx context.Context, db *gorm.DB, ids []string, grants []dto.GrantInput) ([]grantSpec, error)` — parseGrants merges legacy permission_ids (tenant scope) and scoped grants, rejects conflicting duplicates, and verifies every permission exists.
- **func** `(s *roleService) lookupGrants(ctx context.Context, db *gorm.DB, order []uuid.UUID, scopes map[uuid.UUID]access.Scope) ([]grantSpec, error)`
- **func** `(s *roleService) applyGrants(ctx context.Context, tx *gorm.DB, role *models.Role, specs []grantSpec) ([]uuid.UUID, error)` — applyGrants adds new grants and changes the scope of existing ones; it returns the permissions changed.
- **func** `(s *roleService) applyParent(ctx context.Context, tx *gorm.DB, role *models.Role, raw *string) error` — applyParent validates and sets (or with "" clears) the role's parent: same tenant, no cycle, depth limit, and the caller must already hold everything the parent grants.
- **func** `(s *roleService) checkParent(ctx context.Context, tx *gorm.DB, role *models.Role, parentID uuid.UUID) error`
- **func** `(s *roleService) CloneRole(ctx context.Context, tx *gorm.DB, tenantID, id uuid.UUID, req dto.CloneRoleRequest) (*dto.RoleResponse, error)`
- **func** `(s *roleService) loadPermissions(ctx context.Context, db *gorm.DB, tenantID uuid.UUID, roles ...*models.Role) error` — loadPermissions attaches each role's permissions (one batched query).
- **func** `mapRoleToResponse(r *models.Role) *dto.RoleResponse`

#### `internal/identity/services/role_service.go` (316 lines)
- **type** `RoleService interface` — RoleService manages RBAC roles: grants with scopes, inheritance, predefined and automatic roles (docs/ARCHITECTURE.md §17).
- **method** `RoleService.CreateRole(ctx context.Context, tx *gorm.DB, tenantID uuid.UUID, req dto.CreateRoleRequest) (*dto.RoleResponse, error)`
- **method** `RoleService.GetRoleByID(ctx context.Context, db *gorm.DB, tenantID, id uuid.UUID) (*dto.RoleResponse, error)`
- **method** `RoleService.ListRoles(ctx context.Context, db *gorm.DB, tenantID uuid.UUID, page, perPage int) (*dto.RoleListResponse, error)`
- **method** `RoleService.UpdateRole(ctx context.Context, tx *gorm.DB, tenantID, id uuid.UUID, req dto.UpdateRoleRequest) (*dto.RoleResponse, error)`
- **method** `RoleService.DeleteRole(ctx context.Context, tx *gorm.DB, tenantID, id uuid.UUID) error`
- **method** `RoleService.CloneRole(ctx context.Context, tx *gorm.DB, tenantID, id uuid.UUID, req dto.CloneRoleRequest) (*dto.RoleResponse, error)`
- **method** `RoleService.AssignPermissions(ctx context.Context, tx *gorm.DB, tenantID, roleID uuid.UUID, req dto.AssignPermissionsRequest, correlationID uuid.UUID) error`
- **method** `RoleService.AssignRolesToUser(ctx context.Context, tx *gorm.DB, tenantID, userID uuid.UUID, req dto.AssignRoleRequest, correlationID uuid.UUID) error`
- **method** `RoleService.RemovePermission(ctx context.Context, tx *gorm.DB, tenantID, roleID, permissionID uuid.UUID) error`
- **method** `RoleService.RemoveRoleFromUser(ctx context.Context, tx *gorm.DB, tenantID, userID, roleID uuid.UUID) error`
- **method** `RoleService.GetMyAccess(ctx context.Context, db *gorm.DB, tenantID, userID uuid.UUID) (*dto.AccessResponse, error)`
- **type** `roleService struct`
- **type** `RoleOption func(*roleService)` — RoleOption customises a RoleService.
- **func** `WithRoleEvents(sink EventSink) RoleOption { return func(s *roleService) { s.sink = sink } }` — WithRoleEvents records role events through sink (the transactional outbox in production).
- **func** `WithAccessModel(repo repositories.AccessRepository, resolver AccessResolver) RoleOption` — WithAccessModel enables inheritance, scope changes and full access resolution for escalation checks.
- **func** `NewRoleService(` — NewRoleService creates a new RoleService.
- **func** `(s *roleService) findRole(ctx context.Context, db *gorm.DB, tenantID, id uuid.UUID) (*models.Role, error)`
- **func** `(s *roleService) CreateRole(ctx context.Context, tx *gorm.DB, tenantID uuid.UUID, req dto.CreateRoleRequest) (*dto.RoleResponse, error)`
- **func** `(s *roleService) ensureNameFree(ctx context.Context, db *gorm.DB, tenantID uuid.UUID, name string, self *uuid.UUID) error` — ensureNameFree refuses a name another role uses; self is the role being renamed (nil on create).
- **func** `(s *roleService) respond(ctx context.Context, db *gorm.DB, role *models.Role) (*dto.RoleResponse, error)`
- **func** `(s *roleService) GetRoleByID(ctx context.Context, db *gorm.DB, tenantID, id uuid.UUID) (*dto.RoleResponse, error)`
- **func** `(s *roleService) ListRoles(ctx context.Context, db *gorm.DB, tenantID uuid.UUID, page, perPage int) (*dto.RoleListResponse, error)`
- **func** `(s *roleService) UpdateRole(ctx context.Context, tx *gorm.DB, tenantID, id uuid.UUID, req dto.UpdateRoleRequest) (*dto.RoleResponse, error)`
- **func** `(s *roleService) DeleteRole(ctx context.Context, tx *gorm.DB, tenantID, id uuid.UUID) error`
- **func** `(s *roleService) ensureNoChildren(ctx context.Context, db *gorm.DB, tenantID, id uuid.UUID) error` — ensureNoChildren refuses to delete a role other roles inherit from (their access would change silently).
- **func** `(s *roleService) AssignPermissions(ctx context.Context, tx *gorm.DB, tenantID, roleID uuid.UUID, req dto.AssignPermissionsRequest, correlationID uuid.UUID) error`
- **func** `(s *roleService) markEdited(ctx context.Context, tx *gorm.DB, role *models.Role) error` — markEdited takes a predefined role out of template management after a tenant edit (never overwritten again).
- **func** `(s *roleService) RemovePermission(ctx context.Context, tx *gorm.DB, tenantID, roleID, permissionID uuid.UUID) error`

#### `internal/identity/services/secrets.go` (71 lines)
- **type** `SecretMessage struct` — SecretMessage is a one-time secret for one user. It never enters events, audit rows or logs.
- **type** `SecretDelivery interface` — SecretDelivery sends one-time secrets (email in production via M10 Notifications; the dev mailbox in development).
- **method** `SecretDelivery.Deliver(ctx context.Context, m SecretMessage) error`
- **type** `DevMailbox struct` — DevMailbox keeps the latest secrets in memory so developers and the test console can complete reset and invitation flows without email. It must never be enabled outside development.
- **func** `NewDevMailbox() *DevMailbox { return &DevMailbox{} }` — NewDevMailbox returns an empty mailbox.
- **func** `(d *DevMailbox) Deliver(_ context.Context, m SecretMessage) error` — Deliver stores the message, dropping the oldest when full.
- **func** `(d *DevMailbox) For(tenantID uuid.UUID, email string) []SecretMessage` — For returns the messages for an email address in a tenant, newest first.

#### `internal/identity/services/session_service.go` (108 lines)
- **type** `SessionService interface` — SessionService manages user HTTP session mappings.
- **method** `SessionService.CreateSession(ctx context.Context, tx *gorm.DB, userID, tenantID uuid.UUID, ipAddress, userAgent string) (*models.Session, error)`
- **method** `SessionService.ValidateSession(ctx context.Context, db *gorm.DB, sessionID uuid.UUID) (bool, error)`
- **method** `SessionService.RevokeSession(ctx context.Context, tx *gorm.DB, sessionID uuid.UUID) error`
- **method** `SessionService.RevokeAllForUser(ctx context.Context, tx *gorm.DB, tenantID, userID uuid.UUID) error`
- **type** `sessionService struct`
- **func** `NewSessionService(sessionRepo repositories.SessionRepository, redisClient *cache.RedisClient, ttl ...time.Duration) SessionService` — NewSessionService creates a SessionService instance. The optional ttl is the session lifetime and must match the access-token lifetime (defaults to 15 minutes).
- **func** `(s *sessionService) CreateSession(ctx context.Context, tx *gorm.DB, userID, tenantID uuid.UUID, ipAddress, userAgent string) (*models.Session, error)`
- **func** `(s *sessionService) ValidateSession(ctx context.Context, db *gorm.DB, sessionID uuid.UUID) (bool, error)`
- **func** `(s *sessionService) RevokeSession(ctx context.Context, tx *gorm.DB, sessionID uuid.UUID) error`
- **func** `(s *sessionService) RevokeAllForUser(ctx context.Context, tx *gorm.DB, tenantID, userID uuid.UUID) error`

#### `internal/identity/services/signing_keys.go` (97 lines)
- **type** `signingKeys struct` — signingKeys holds the Ed25519 key access tokens are signed with and its key id.
- **func** `ParseSigningKey(raw string) (ed25519.PrivateKey, error)` — ParseSigningKey decodes a base64 (std or URL) Ed25519 seed of 32 bytes.
- **func** `loadSigningKeys(cfg *config.JWTConfig) signingKeys` — loadSigningKeys uses the configured seed, or derives one from the JWT secret (development; production requires an explicit key, enforced by app.ValidateConfig). An invalid configured key falls back to the derived one only because configuration validation rejects it before this is reached.
- **func** `(k signingKeys) verificationKey(cfg *config.JWTConfig) jwt.Keyfunc` — verificationKey picks the key for a token: EdDSA tokens must carry our key id; HS256 tokens are accepted only while the legacy window is open.
- **type** `JWK struct` — JWK is one public key in a JSON Web Key Set.
- **type** `JWKSet struct` — JWKSet is the document served at /.well-known/jwks.json.
- **func** `(s *tokenService) JWKS() JWKSet` — JWKS returns the public verification keys.
- **func** `KeySetOf(ts TokenService) (JWKSet, bool)` — KeySetOf returns the JWKS of a token service that signs with published keys.

#### `internal/identity/services/tenant_service.go` (370 lines)
- **type** `TenantService interface` — TenantService handles tenant lifecycle orchestrations.
- **method** `TenantService.CreateTenant(ctx context.Context, tx *gorm.DB, req dto.CreateTenantRequest, correlationID uuid.UUID) (*dto.TenantResponse, error)`
- **method** `TenantService.GetTenantByID(ctx context.Context, db *gorm.DB, id uuid.UUID) (*dto.TenantResponse, error)`
- **method** `TenantService.GetTenantBySlug(ctx context.Context, db *gorm.DB, slug string) (*dto.TenantResponse, error)`
- **method** `TenantService.ListTenants(ctx context.Context, db *gorm.DB, page, perPage int) (*dto.TenantListResponse, error)`
- **method** `TenantService.UpdateTenant(ctx context.Context, tx *gorm.DB, id uuid.UUID, req dto.UpdateTenantRequest) (*dto.TenantResponse, error)`
- **method** `TenantService.ActivateTenant(ctx context.Context, tx *gorm.DB, id uuid.UUID, correlationID uuid.UUID) (*dto.TenantResponse, error)`
- **method** `TenantService.SuspendTenant(ctx context.Context, tx *gorm.DB, id uuid.UUID, correlationID uuid.UUID) (*dto.TenantResponse, error)`
- **type** `tenantService struct`
- **type** `RoleProvisioner func(ctx context.Context, tx *gorm.DB, tenantID uuid.UUID) error` — RoleProvisioner creates a new tenant's predefined roles inside the tenant-creation transaction.
- **func** `WithTenantEvents(sink EventSink) TenantOption { return func(s *tenantService) { s.sink = sink } }` — WithTenantEvents records tenant events through sink (the transactional outbox in production).
- **func** `WithRoleProvisioner(p RoleProvisioner) TenantOption` — WithRoleProvisioner provisions predefined roles (docs/ARCHITECTURE.md §17) for every new tenant.
- **type** `TenantOption func(*tenantService)` — TenantOption customises a TenantService.
- **func** `WithAdminProvisioning(userRepo repositories.UserRepository, userRoleRepo repositories.UserRoleRepository) TenantOption` — WithAdminProvisioning enables creating the first tenant_admin user together with a tenant.
- **func** `NewTenantService(` — NewTenantService creates a new TenantService.
- **func** `(s *tenantService) CreateTenant(ctx context.Context, tx *gorm.DB, req dto.CreateTenantRequest, correlationID uuid.UUID) (*dto.TenantResponse, error)`
- **func** `(s *tenantService) GetTenantByID(ctx context.Context, db *gorm.DB, id uuid.UUID) (*dto.TenantResponse, error)`
- **func** `(s *tenantService) GetTenantBySlug(ctx context.Context, db *gorm.DB, slug string) (*dto.TenantResponse, error)`
- **func** `(s *tenantService) ListTenants(ctx context.Context, db *gorm.DB, page, perPage int) (*dto.TenantListResponse, error)`
- **func** `(s *tenantService) UpdateTenant(ctx context.Context, tx *gorm.DB, id uuid.UUID, req dto.UpdateTenantRequest) (*dto.TenantResponse, error)`
- **func** `(s *tenantService) ActivateTenant(ctx context.Context, tx *gorm.DB, id uuid.UUID, correlationID uuid.UUID) (*dto.TenantResponse, error)`
- **func** `(s *tenantService) SuspendTenant(ctx context.Context, tx *gorm.DB, id uuid.UUID, correlationID uuid.UUID) (*dto.TenantResponse, error)`
- **func** `mapTenantToResponse(t *models.Tenant) *dto.TenantResponse`
- **func** `pointerString(s string) *string`

#### `internal/identity/services/token_service.go` (118 lines)
- **type** `UserClaims struct` — UserClaims defines the payload structures inside access token JWTs.
- **type** `TokenService interface` — TokenService manages secure token lifecycle operations.
- **method** `TokenService.GenerateAccessToken(userID, tenantID, email string, roles []string, sessionID string) (string, time.Time, error)`
- **method** `TokenService.ValidateAccessToken(tokenStr string) (*UserClaims, error)`
- **method** `TokenService.GenerateOpaqueToken() (string, string, error) // Returns (rawToken, tokenHash, error)`
- **method** `TokenService.HashOpaqueToken(token string) string`
- **method** `TokenService.RefreshTokenTTL() time.Duration` — RefreshTokenTTL is the configured lifetime of refresh tokens.
- **type** `tokenService struct`
- **func** `NewTokenService(cfg *config.JWTConfig) TokenService` — NewTokenService creates a new TokenService. Access tokens are signed with Ed25519 (EdDSA) under a key id published at /.well-known/jwks.json, so any service can verify them without sharing a secret (M0 plan B4).
- **func** `(s *tokenService) GenerateAccessToken(userID, tenantID, email string, roles []string, sessionID string) (string, time.Time, error)`
- **func** `(s *tokenService) ValidateAccessToken(tokenStr string) (*UserClaims, error)`
- **func** `(s *tokenService) GenerateOpaqueToken() (string, string, error)` — GenerateOpaqueToken yields a cryptographically secure random token and its SHA-256 hash.
- **func** `(s *tokenService) HashOpaqueToken(token string) string` — HashOpaqueToken takes raw base64 string and computes SHA-256 hex string.
- **func** `(s *tokenService) RefreshTokenTTL() time.Duration` — RefreshTokenTTL returns the configured refresh token lifetime (days -> duration; defaults to 7 days).

#### `internal/identity/services/user_service.go` (498 lines)
- **type** `UserService interface` — UserService manages tenant user profiles and statuses.
- **method** `UserService.CreateUser(ctx context.Context, tx *gorm.DB, tenantID uuid.UUID, req dto.CreateUserRequest, correlationID uuid.UUID) (*dto.UserResponse, error)`
- **method** `UserService.InviteUser(ctx context.Context, tx *gorm.DB, tenantID uuid.UUID, req dto.InviteUserRequest, correlationID uuid.UUID) (*dto.UserResponse, error)`
- **method** `UserService.GetByID(ctx context.Context, db *gorm.DB, tenantID, id uuid.UUID) (*dto.UserResponse, error)`
- **method** `UserService.ListUsers(ctx context.Context, db *gorm.DB, tenantID uuid.UUID, page, perPage int) (*dto.UserListResponse, error)`
- **method** `UserService.UpdateUser(ctx context.Context, tx *gorm.DB, tenantID, id uuid.UUID, req dto.UpdateUserRequest) (*dto.UserResponse, error)`
- **method** `UserService.DeactivateUser(ctx context.Context, tx *gorm.DB, tenantID, id uuid.UUID, correlationID uuid.UUID) error`
- **method** `UserService.ActivateUser(ctx context.Context, tx *gorm.DB, tenantID, id uuid.UUID) (*dto.UserResponse, error)`
- **type** `UserOption func(*userService)` — UserOption customises a UserService.
- **func** `WithRolePermissions(rp repositories.RolePermissionRepository) UserOption` — WithRolePermissions enables the no-privilege-escalation check (flat roles) when users are created with roles.
- **func** `WithInvitations(inv InvitationService) UserOption` — WithInvitations issues an invitation (token delivered to the user) whenever a user is invited.
- **func** `WithUserEvents(sink EventSink) UserOption { return func(s *userService) { s.sink = sink } }` — WithUserEvents records user events through sink (the transactional outbox in production).
- **func** `WithAccessResolver(r AccessResolver) UserOption` — WithAccessResolver enables the scoped, inheritance-aware no-privilege-escalation check (docs/ARCHITECTURE.md §17).
- **type** `userService struct`
- **type** `UserDeactivationConverger interface` — UserDeactivationConverger converges dependent state when a user is deactivated (Module 1 org mappings, Module 2 employee profile). Registered at boot by internal/app after the modules are wired.
- **method** `UserDeactivationConverger.DeactivateUserMappings(ctx context.Context, tx *gorm.DB, tenantID, userID uuid.UUID, correlationID uuid.UUID) error`
- **func** `AddUserDeactivationConverger(c UserDeactivationConverger)` — AddUserDeactivationConverger registers a cross-module convergence hook that runs inside the deactivation transaction (so by the time the API answers 200, dependent state has converged).
- **func** `SetUserDeactivationConverger(c UserDeactivationConverger)` — SetUserDeactivationConverger replaces all registered hooks with c (nil clears them).
- **func** `NewUserService(` — NewUserService creates a new UserService.
- **func** `(s *userService) resolveRoles(ctx context.Context, tx *gorm.DB, tenantID uuid.UUID, raw []string) ([]*models.Role, error)` — resolveRoles validates the requested role ids (de-duplicated, must exist in the tenant) and enforces that the caller may assign them, BEFORE anything is written.
- **func** `(s *userService) CreateUser(ctx context.Context, tx *gorm.DB, tenantID uuid.UUID, req dto.CreateUserRequest, correlationID uuid.UUID) (*dto.UserResponse, error)`
- **func** `(s *userService) InviteUser(ctx context.Context, tx *gorm.DB, tenantID uuid.UUID, req dto.InviteUserRequest, correlationID uuid.UUID) (*dto.UserResponse, error)`
- **func** `(s *userService) GetByID(ctx context.Context, db *gorm.DB, tenantID, id uuid.UUID) (*dto.UserResponse, error)`
- **func** `(s *userService) attachRoles(ctx context.Context, db *gorm.DB, tenantID uuid.UUID, users ...*dto.UserResponse) error` — attachRoles fills Roles on the given responses with one batched query.
- **func** `(s *userService) ListUsers(ctx context.Context, db *gorm.DB, tenantID uuid.UUID, page, perPage int) (*dto.UserListResponse, error)`
- **func** `(s *userService) UpdateUser(ctx context.Context, tx *gorm.DB, tenantID, id uuid.UUID, req dto.UpdateUserRequest) (*dto.UserResponse, error)`
- **func** `(s *userService) ActivateUser(ctx context.Context, tx *gorm.DB, tenantID, id uuid.UUID) (*dto.UserResponse, error)` — ActivateUser re-enables a deactivated user (previously revoked sessions stay revoked; the user must log in again).
- **func** `(s *userService) DeactivateUser(ctx context.Context, tx *gorm.DB, tenantID, id uuid.UUID, correlationID uuid.UUID) error`
- **func** `mapUserToResponse(u *models.User) *dto.UserResponse`

#### `internal/identity/validators/validators.go` (41 lines)
- **func** `ValidateSlug(slug string) bool` — ValidateSlug checks if the slug conforms to subdomain routing constraints.
- **func** `IsReservedSlug(slug string) bool` — IsReservedSlug checks if the slug is reserved by the platform.

### M1 HR / Organization


#### `internal/hr/organization/access_facts.go` (37 lines)
- **func** `(m *Module) ManagerUserIDs(ctx context.Context, tenantID uuid.UUID) ([]uuid.UUID, error)` — ManagerUserIDs lists the users of the tenant who currently have at least one active direct report (automatic Manager role, docs/ARCHITECTURE.md §17).
- **func** `(m *Module) HasDirectReports(ctx context.Context, tenantID, userID uuid.UUID) (bool, error)` — HasDirectReports reports whether the user manages at least one active mapping in the tenant.
- **func** `(m *Module) DirectManager(ctx context.Context, tenantID, userID uuid.UUID) (*uuid.UUID, error)` — DirectManager returns the manager_user_id of the user's primary active mapping, or nil (HR/Performance D7-03).

#### `internal/hr/organization/catalogue.go` (16 lines)
- **func** `Catalogue() []access.PermissionDef` — Catalogue is HR/Organization's permission catalogue with the scopes predefined roles receive by default (docs/ARCHITECTURE.md §17). The app registers it with M0 at boot.

#### `internal/hr/organization/module.go` (149 lines)
- **type** `Module struct` — Module encapsulates the Module 1 Organization domain.
- **func** `NewModule(` — NewModule wires the organization module on shared handles plus Module 0 services.
- **func** `(m *Module) RegisterModels() []interface{}` — RegisterModels lists org entity models for migrations.
- **func** `(m *Module) Consumer() services.EventConsumer { return m.consumer }` — Consumer exposes the identity-event consumer for broker subscription wiring.
- **func** `(m *Module) ChartInvalidator() services.ChartInvalidator` — ChartInvalidator exposes org-chart cache invalidation.
- **type** `convergeAndInvalidate struct` — convergeAndInvalidate wraps the FR-M005 user-deactivation hook so the cached org chart is dropped too.
- **func** `(c convergeAndInvalidate) DeactivateUserMappings(ctx context.Context, tx *gorm.DB, tenantID, userID, correlationID uuid.UUID) error`
- **func** `(m *Module) UserDeactivationConverger() identityport.DeactivationHook` — UserDeactivationConverger is the FR-M005 hook (mappings converge + chart cache invalidated).
- **func** `(m *Module) MappingService() services.MappingService { return m.mappingSvc }` — MappingService exposes the mapping domain service for cross-module convergence (FR-M005 hook registered in cmd/main.go).
- **func** `(m *Module) DepartmentService() services.DepartmentService { return m.deptSvc }` — DepartmentService exposes read access to departments for dependent modules (Module 6 D6-03).
- **func** `(m *Module) DesignationService() services.DesignationService { return m.desigSvc }` — DesignationService exposes read access to designations for dependent modules (Module 6 D6-03).
- **func** `(m *Module) RegisterRoutes(router *gin.RouterGroup, idp identityport.HTTP)` — RegisterRoutes hooks org endpoint groups onto the API router. The identity module owns tenantResolver/authMiddleware construction; org reuses them.
- **func** `OrgEvents() []string` — OrgEvents returns produced event type documentation for consumers.

#### `internal/hr/organization/controllers/department_controller.go` (154 lines)
- **type** `DepartmentController struct` — DepartmentController handles department endpoints.
- **func** `NewDepartmentController(db *gorm.DB, svc services.DepartmentService) *DepartmentController` — NewDepartmentController returns a DepartmentController.
- **func** `tenantIDOf(c *gin.Context) (uuid.UUID, bool)`
- **func** `(ctrl *DepartmentController) Create(c *gin.Context)` — Create handles POST /departments.
- **func** `(ctrl *DepartmentController) GetByID(c *gin.Context)` — GetByID handles GET /departments/:id.
- **func** `(ctrl *DepartmentController) List(c *gin.Context)` — List handles GET /departments.
- **func** `(ctrl *DepartmentController) Update(c *gin.Context)` — Update handles PATCH /departments/:id.
- **func** `(ctrl *DepartmentController) Deactivate(c *gin.Context)` — Deactivate handles POST /departments/:id/deactivate?force=. FR-D007.

#### `internal/hr/organization/controllers/designation_controller.go` (143 lines)
- **type** `DesignationController struct` — DesignationController handles designation endpoints.
- **func** `NewDesignationController(db *gorm.DB, svc services.DesignationService) *DesignationController` — NewDesignationController returns a DesignationController.
- **func** `(ctrl *DesignationController) Create(c *gin.Context)` — Create handles POST /designations.
- **func** `(ctrl *DesignationController) GetByID(c *gin.Context)` — GetByID handles GET /designations/:id.
- **func** `(ctrl *DesignationController) List(c *gin.Context)` — List handles GET /designations.
- **func** `(ctrl *DesignationController) Update(c *gin.Context)` — Update handles PATCH /designations/:id.
- **func** `(ctrl *DesignationController) Deactivate(c *gin.Context)` — Deactivate handles POST /designations/:id/deactivate. FR-DG004.

#### `internal/hr/organization/controllers/errors.go` (7 lines)

#### `internal/hr/organization/controllers/mapping_controller.go` (149 lines)
- **type** `MappingController struct` — MappingController handles user→org mapping endpoints.
- **func** `NewMappingController(db *gorm.DB, svc services.MappingService) *MappingController` — NewMappingController returns a MappingController.
- **func** `(ctrl *MappingController) Create(c *gin.Context)` — Create handles POST /mappings.
- **func** `(ctrl *MappingController) GetByID(c *gin.Context)` — GetByID handles GET /mappings/:id.
- **func** `(ctrl *MappingController) ListByUser(c *gin.Context)` — ListByUser handles GET /mappings?user_id=.
- **func** `(ctrl *MappingController) Update(c *gin.Context)` — Update handles PATCH /mappings/:id.
- **func** `(ctrl *MappingController) Deactivate(c *gin.Context)` — Deactivate handles POST /mappings/:id/deactivate.

#### `internal/hr/organization/controllers/org_chart_controller.go` (59 lines)
- **type** `OrgChartController struct` — OrgChartController serves the org-chart read model.
- **func** `NewOrgChartController(db *gorm.DB, svc services.OrgChartService) *OrgChartController` — NewOrgChartController returns an OrgChartController.
- **func** `(ctrl *OrgChartController) Chart(c *gin.Context)` — Chart handles GET /org-chart.
- **func** `(ctrl *OrgChartController) UserChain(c *gin.Context)` — UserChain handles GET /org-chart/chain?user_id=.

#### `internal/hr/organization/controllers/response.go` (66 lines)
- **type** `successEnvelope struct`
- **type** `errorEnvelope struct`
- **type** `errorDetail struct`
- **func** `respondSuccess(c *gin.Context, status int, data interface{}, meta interface{})` — respondSuccess returns the standard success envelope.
- **func** `respondError(c *gin.Context, err error)` — respondError maps AppError to the standard error envelope.
- **func** `respondValidationError(c *gin.Context, details []sharedErrors.ValidationErrorDetail)` — respondValidationError returns field-level validation details.
- **func** `respondBadRequest(c *gin.Context, msg string)` — respondBadRequest returns a 400 in the standard error envelope.
- **func** `respondBindError(c *gin.Context, err error)` — respondBindError converts a request-binding failure into field-level validation details.

#### `internal/hr/organization/controllers/team_controller.go` (152 lines)
- **type** `TeamController struct` — TeamController handles team endpoints.
- **func** `NewTeamController(db *gorm.DB, svc services.TeamService) *TeamController` — NewTeamController returns a TeamController.
- **func** `(ctrl *TeamController) Create(c *gin.Context)` — Create handles POST /teams.
- **func** `(ctrl *TeamController) GetByID(c *gin.Context)` — GetByID handles GET /teams/:id.
- **func** `(ctrl *TeamController) List(c *gin.Context)` — List handles GET /teams with optional department_id filter.
- **func** `(ctrl *TeamController) Update(c *gin.Context)` — Update handles PATCH /teams/:id.
- **func** `(ctrl *TeamController) Deactivate(c *gin.Context)` — Deactivate handles POST /teams/:id/deactivate. FR-T006.

#### `internal/hr/organization/dto/deactivate_dto.go` (7 lines)
- **type** `DeactivateRequest struct` — DeactivateRequest carries an optional reason for lifecycle deactivation (departments, teams, designations). Departments also accept ?force=true.

#### `internal/hr/organization/dto/department_dto.go` (39 lines)
- **type** `CreateDepartmentRequest struct` — CreateDepartmentRequest creates a department. FR-D001.
- **type** `UpdateDepartmentRequest struct` — UpdateDepartmentRequest patches a department; code is immutable and ignored. FR-D005.
- **type** `DepartmentResponse struct` — DepartmentResponse serializes a department with hierarchy position.
- **type** `DepartmentListResponse struct` — DepartmentListResponse is a paginated department list.

#### `internal/hr/organization/dto/designation_dto.go` (37 lines)
- **type** `CreateDesignationRequest struct` — CreateDesignationRequest creates a job title/level. FR-DG001.
- **type** `UpdateDesignationRequest struct` — UpdateDesignationRequest patches a designation; code is immutable and ignored.
- **type** `DesignationResponse struct` — DesignationResponse serializes a designation.
- **type** `DesignationListResponse struct` — DesignationListResponse is a paginated designation list.

#### `internal/hr/organization/dto/mapping_dto.go` (48 lines)
- **type** `CreateMappingRequest struct` — CreateMappingRequest maps a user into org structure. FR-M001.
- **type** `UpdateMappingRequest struct` — UpdateMappingRequest patches a mapping (reassignment, primary swap, manager change).
- **type** `DeactivateMappingRequest struct` — DeactivateMappingRequest deactivates with a reason (append-only, FR-M007).
- **type** `MappingResponse struct` — MappingResponse serializes a user→org mapping (IDs only, no PII).
- **type** `MappingListResponse struct` — MappingListResponse is a paginated mapping list.

#### `internal/hr/organization/dto/org_chart_dto.go` (27 lines)
- **type** `OrgChartNode struct` — OrgChartNode is one department node with nested children, teams, and headcount. FR-O001.
- **type** `OrgChartResponse struct` — OrgChartResponse is the tenant forest (roots only at top level).
- **type** `UserChainResponse struct` — UserChainResponse is a user's chain to root plus direct reports. FR-O002.

#### `internal/hr/organization/dto/pagination_dto.go` (24 lines)
- **type** `PaginationMeta struct` — PaginationMeta mirrors the identity pagination envelope for org list responses.
- **func** `NewPaginationMeta(page, perPage int, total int64) PaginationMeta` — NewPaginationMeta builds pagination metadata from a total count.

#### `internal/hr/organization/dto/team_dto.go` (41 lines)
- **type** `CreateTeamRequest struct` — CreateTeamRequest creates a team inside a department. FR-T001.
- **type** `UpdateTeamRequest struct` — UpdateTeamRequest patches a team; code is immutable and ignored. FR-T005.
- **type** `TeamResponse struct` — TeamResponse serializes a team with member count.
- **type** `TeamListResponse struct` — TeamListResponse is a paginated team list.

#### `internal/hr/organization/events/events.go` (101 lines)
- **type** `ConsumedUserPayload struct` — ConsumedUserPayload is the identity user lifecycle payload consumed by org.
- **type** `ConsumedTenantPayload struct` — ConsumedTenantPayload is the identity tenant lifecycle payload consumed by org.
- **type** `Event struct` — Event is the standard envelope (mirrors identity events).
- **func** `NewEvent(eventType, routingKey string, tenantID, correlationID uuid.UUID, payload interface{}) Event` — NewEvent wraps a payload in the standard envelope.
- **type** `DepartmentPayload struct` — DepartmentPayload carries department lifecycle data.
- **type** `TeamPayload struct` — TeamPayload carries team lifecycle data.
- **type** `MappingPayload struct` — MappingPayload carries mapping lifecycle data.

#### `internal/hr/organization/models/department.go` (30 lines)
- **type** `Department struct` — Department represents a company department scoped to a tenant. FR-D001..FR-D008.
- **func** `(d *Department) BeforeCreate(tx *gorm.DB) error` — BeforeCreate generates a UUID v4 key when unset.

#### `internal/hr/organization/models/designation.go` (26 lines)
- **type** `Designation struct` — Designation represents a job title/level scoped to a tenant. FR-DG001..FR-DG004.
- **func** `(d *Designation) BeforeCreate(tx *gorm.DB) error` — BeforeCreate generates a UUID v4 key when unset.

#### `internal/hr/organization/models/mapping.go` (43 lines)
- **type** `Mapping struct` — Mapping links a user to org structure. Append-only: deactivate, never hard delete. FR-M001..FR-M007.
- **func** `(m *Mapping) BeforeCreate(tx *gorm.DB) error` — BeforeCreate generates a UUID v4 key when unset.

#### `internal/hr/organization/models/outbox.go` (33 lines)
- **type** `OrgEventOutbox struct` — OrgEventOutbox buffers produced events when the broker is down; a retry worker republishes them later. Guarantees queue downtime never blocks the API (FR-E005).
- **func** `(OrgEventOutbox) TableName() string { return "org_events_outbox" }` — TableName maps to the org_events_outbox table (migration 000017).
- **func** `(o *OrgEventOutbox) BeforeCreate(tx *gorm.DB) error` — BeforeCreate generates a UUID v4 key when unset.

#### `internal/hr/organization/models/team.go` (27 lines)
- **type** `Team struct` — Team represents a team inside a department, scoped to a tenant. FR-T001..FR-T007.
- **func** `(t *Team) BeforeCreate(tx *gorm.DB) error` — BeforeCreate generates a UUID v4 key when unset.

#### `internal/hr/organization/repositories/department_repository.go` (82 lines)
- **type** `departmentRepository struct{}`
- **func** `NewDepartmentRepository() DepartmentRepository` — NewDepartmentRepository returns a DepartmentRepository.
- **func** `(r *departmentRepository) Create(ctx context.Context, tx *gorm.DB, dept *models.Department) error`
- **func** `(r *departmentRepository) FindByID(ctx context.Context, db *gorm.DB, tenantID, id uuid.UUID) (*models.Department, error)`
- **func** `(r *departmentRepository) FindByName(ctx context.Context, db *gorm.DB, tenantID uuid.UUID, name string) (*models.Department, error)`
- **func** `(r *departmentRepository) FindAll(ctx context.Context, db *gorm.DB, tenantID uuid.UUID, page, perPage int) ([]models.Department, int64, error)`
- **func** `(r *departmentRepository) FindChildren(ctx context.Context, db *gorm.DB, tenantID, parentID uuid.UUID) ([]models.Department, error)`
- **func** `(r *departmentRepository) FindRoots(ctx context.Context, db *gorm.DB, tenantID uuid.UUID) ([]models.Department, error)`
- **func** `(r *departmentRepository) Update(ctx context.Context, tx *gorm.DB, dept *models.Department) error`
- **func** `(r *departmentRepository) Delete(ctx context.Context, tx *gorm.DB, tenantID, id uuid.UUID) error`

#### `internal/hr/organization/repositories/designation_repository.go` (70 lines)
- **type** `designationRepository struct{}`
- **func** `NewDesignationRepository() DesignationRepository` — NewDesignationRepository returns a DesignationRepository.
- **func** `(r *designationRepository) Create(ctx context.Context, tx *gorm.DB, d *models.Designation) error`
- **func** `(r *designationRepository) FindByID(ctx context.Context, db *gorm.DB, tenantID, id uuid.UUID) (*models.Designation, error)`
- **func** `(r *designationRepository) FindByTitle(ctx context.Context, db *gorm.DB, tenantID uuid.UUID, title string) (*models.Designation, error)`
- **func** `(r *designationRepository) FindAll(ctx context.Context, db *gorm.DB, tenantID uuid.UUID, page, perPage int) ([]models.Designation, int64, error)`
- **func** `(r *designationRepository) Update(ctx context.Context, tx *gorm.DB, d *models.Designation) error`
- **func** `(r *designationRepository) Delete(ctx context.Context, tx *gorm.DB, tenantID, id uuid.UUID) error`

#### `internal/hr/organization/repositories/interfaces.go` (56 lines)
- **type** `DepartmentRepository interface` — DepartmentRepository handles CRUD for departments scoped by tenant.
- **method** `DepartmentRepository.Create(ctx context.Context, tx *gorm.DB, dept *models.Department) error`
- **method** `DepartmentRepository.FindByID(ctx context.Context, db *gorm.DB, tenantID, id uuid.UUID) (*models.Department, error)`
- **method** `DepartmentRepository.FindByName(ctx context.Context, db *gorm.DB, tenantID uuid.UUID, name string) (*models.Department, error)`
- **method** `DepartmentRepository.FindAll(ctx context.Context, db *gorm.DB, tenantID uuid.UUID, page, perPage int) ([]models.Department, int64, error)`
- **method** `DepartmentRepository.FindChildren(ctx context.Context, db *gorm.DB, tenantID, parentID uuid.UUID) ([]models.Department, error)`
- **method** `DepartmentRepository.FindRoots(ctx context.Context, db *gorm.DB, tenantID uuid.UUID) ([]models.Department, error)`
- **method** `DepartmentRepository.Update(ctx context.Context, tx *gorm.DB, dept *models.Department) error`
- **method** `DepartmentRepository.Delete(ctx context.Context, tx *gorm.DB, tenantID, id uuid.UUID) error`
- **type** `TeamRepository interface` — TeamRepository handles CRUD for teams scoped by tenant.
- **method** `TeamRepository.Create(ctx context.Context, tx *gorm.DB, team *models.Team) error`
- **method** `TeamRepository.FindByID(ctx context.Context, db *gorm.DB, tenantID, id uuid.UUID) (*models.Team, error)`
- **method** `TeamRepository.FindByName(ctx context.Context, db *gorm.DB, tenantID, departmentID uuid.UUID, name string) (*models.Team, error)`
- **method** `TeamRepository.FindAll(ctx context.Context, db *gorm.DB, tenantID uuid.UUID, departmentID *uuid.UUID, page, perPage int) ([]models.Team, int64, error)`
- **method** `TeamRepository.CountByDepartment(ctx context.Context, db *gorm.DB, tenantID, departmentID uuid.UUID) (int64, error)`
- **method** `TeamRepository.Update(ctx context.Context, tx *gorm.DB, team *models.Team) error`
- **method** `TeamRepository.Delete(ctx context.Context, tx *gorm.DB, tenantID, id uuid.UUID) error`
- **type** `DesignationRepository interface` — DesignationRepository handles CRUD for designations scoped by tenant.
- **method** `DesignationRepository.Create(ctx context.Context, tx *gorm.DB, d *models.Designation) error`
- **method** `DesignationRepository.FindByID(ctx context.Context, db *gorm.DB, tenantID, id uuid.UUID) (*models.Designation, error)`
- **method** `DesignationRepository.FindByTitle(ctx context.Context, db *gorm.DB, tenantID uuid.UUID, title string) (*models.Designation, error)`
- **method** `DesignationRepository.FindAll(ctx context.Context, db *gorm.DB, tenantID uuid.UUID, page, perPage int) ([]models.Designation, int64, error)`
- **method** `DesignationRepository.Update(ctx context.Context, tx *gorm.DB, d *models.Designation) error`
- **method** `DesignationRepository.Delete(ctx context.Context, tx *gorm.DB, tenantID, id uuid.UUID) error`
- **type** `MappingRepository interface` — MappingRepository handles user→org mappings (append-only; deactivate, never hard delete).
- **method** `MappingRepository.Create(ctx context.Context, tx *gorm.DB, m *models.Mapping) error`
- **method** `MappingRepository.FindByID(ctx context.Context, db *gorm.DB, tenantID, id uuid.UUID) (*models.Mapping, error)`
- **method** `MappingRepository.FindPrimaryByUser(ctx context.Context, db *gorm.DB, tenantID, userID uuid.UUID) (*models.Mapping, error)`
- **method** `MappingRepository.FindByUser(ctx context.Context, db *gorm.DB, tenantID, userID uuid.UUID, page, perPage int) ([]models.Mapping, int64, error)`
- **method** `MappingRepository.FindReports(ctx context.Context, db *gorm.DB, tenantID, managerID uuid.UUID) ([]models.Mapping, error)`
- **method** `MappingRepository.FindByTeam(ctx context.Context, db *gorm.DB, tenantID, teamID uuid.UUID) ([]models.Mapping, error)`
- **method** `MappingRepository.CountActiveByDepartment(ctx context.Context, db *gorm.DB, tenantID, departmentID uuid.UUID) (int64, error)`
- **method** `MappingRepository.CountActiveByTeam(ctx context.Context, db *gorm.DB, tenantID, teamID uuid.UUID) (int64, error)`
- **method** `MappingRepository.CountActiveByDesignation(ctx context.Context, db *gorm.DB, tenantID, designationID uuid.UUID) (int64, error)`
- **method** `MappingRepository.Update(ctx context.Context, tx *gorm.DB, m *models.Mapping) error`

#### `internal/hr/organization/repositories/mapping_repository.go` (95 lines)
- **type** `mappingRepository struct{}`
- **func** `NewMappingRepository() MappingRepository` — NewMappingRepository returns a MappingRepository.
- **func** `(r *mappingRepository) Create(ctx context.Context, tx *gorm.DB, m *models.Mapping) error`
- **func** `(r *mappingRepository) FindByID(ctx context.Context, db *gorm.DB, tenantID, id uuid.UUID) (*models.Mapping, error)`
- **func** `(r *mappingRepository) FindPrimaryByUser(ctx context.Context, db *gorm.DB, tenantID, userID uuid.UUID) (*models.Mapping, error)`
- **func** `(r *mappingRepository) FindByUser(ctx context.Context, db *gorm.DB, tenantID, userID uuid.UUID, page, perPage int) ([]models.Mapping, int64, error)`
- **func** `(r *mappingRepository) FindReports(ctx context.Context, db *gorm.DB, tenantID, managerID uuid.UUID) ([]models.Mapping, error)`
- **func** `(r *mappingRepository) FindByTeam(ctx context.Context, db *gorm.DB, tenantID, teamID uuid.UUID) ([]models.Mapping, error)`
- **func** `(r *mappingRepository) CountActiveByDepartment(ctx context.Context, db *gorm.DB, tenantID, departmentID uuid.UUID) (int64, error)`
- **func** `(r *mappingRepository) CountActiveByTeam(ctx context.Context, db *gorm.DB, tenantID, teamID uuid.UUID) (int64, error)`
- **func** `(r *mappingRepository) CountActiveByDesignation(ctx context.Context, db *gorm.DB, tenantID, designationID uuid.UUID) (int64, error)`
- **func** `(r *mappingRepository) Update(ctx context.Context, tx *gorm.DB, m *models.Mapping) error`

#### `internal/hr/organization/repositories/team_repository.go` (80 lines)
- **type** `teamRepository struct{}`
- **func** `NewTeamRepository() TeamRepository` — NewTeamRepository returns a TeamRepository.
- **func** `(r *teamRepository) Create(ctx context.Context, tx *gorm.DB, team *models.Team) error`
- **func** `(r *teamRepository) FindByID(ctx context.Context, db *gorm.DB, tenantID, id uuid.UUID) (*models.Team, error)`
- **func** `(r *teamRepository) FindByName(ctx context.Context, db *gorm.DB, tenantID, departmentID uuid.UUID, name string) (*models.Team, error)` — FindByName looks up a team by case-insensitive name within one department (FR-T002).
- **func** `(r *teamRepository) FindAll(ctx context.Context, db *gorm.DB, tenantID uuid.UUID, departmentID *uuid.UUID, page, perPage int) ([]models.Team, int64, error)`
- **func** `(r *teamRepository) CountByDepartment(ctx context.Context, db *gorm.DB, tenantID, departmentID uuid.UUID) (int64, error)` — CountByDepartment counts ACTIVE teams of a department (inactive teams must not block deactivation).
- **func** `(r *teamRepository) Update(ctx context.Context, tx *gorm.DB, team *models.Team) error`
- **func** `(r *teamRepository) Delete(ctx context.Context, tx *gorm.DB, tenantID, id uuid.UUID) error`

#### `internal/hr/organization/routes/routes.go` (93 lines)
- **type** `Controllers struct` — RegisterRoutes wires organization endpoint groups with Module 0 middleware (tenantResolver + authMiddleware built from the identity module; rate limiting reuses Module 0 middleware on the same Gin engine). Controllers groups the organization HTTP handlers.
- **func** `RegisterRoutes(router *gin.RouterGroup, idp identityport.HTTP, c Controllers, chartInv services.ChartInvalidator)`
- **func** `chartInvalidation(inv services.ChartInvalidator) gin.HandlerFunc` — chartInvalidation drops the tenant's cached org chart after every successful org write (departments, teams, mappings), so GET /org-chart is read-your-writes consistent.

#### `internal/hr/organization/services/department_service.go` (235 lines)
- **type** `DepartmentService interface` — DepartmentService handles department lifecycle and hierarchy moves.
- **method** `DepartmentService.CreateDepartment(ctx context.Context, tx *gorm.DB, tenantID uuid.UUID, req dto.CreateDepartmentRequest, correlationID uuid.UUID) (*dto.DepartmentResponse, error)`
- **method** `DepartmentService.GetDepartment(ctx context.Context, db *gorm.DB, tenantID, id uuid.UUID) (*dto.DepartmentResponse, error)`
- **method** `DepartmentService.ListDepartments(ctx context.Context, db *gorm.DB, tenantID uuid.UUID, page, perPage int) (*dto.DepartmentListResponse, error)`
- **method** `DepartmentService.UpdateDepartment(ctx context.Context, tx *gorm.DB, tenantID, id uuid.UUID, req dto.UpdateDepartmentRequest) (*dto.DepartmentResponse, error)`
- **method** `DepartmentService.DeactivateDepartment(ctx context.Context, tx *gorm.DB, tenantID, id uuid.UUID, force bool, reason *string) (*dto.DepartmentResponse, error)`
- **type** `departmentService struct`
- **func** `NewDepartmentService(` — NewDepartmentService creates a DepartmentService.
- **func** `(s *departmentService) CreateDepartment(ctx context.Context, tx *gorm.DB, tenantID uuid.UUID, req dto.CreateDepartmentRequest, correlationID uuid.UUID) (*dto.DepartmentResponse, error)`
- **func** `(s *departmentService) GetDepartment(ctx context.Context, db *gorm.DB, tenantID, id uuid.UUID) (*dto.DepartmentResponse, error)`
- **func** `(s *departmentService) ListDepartments(ctx context.Context, db *gorm.DB, tenantID uuid.UUID, page, perPage int) (*dto.DepartmentListResponse, error)`
- **func** `(s *departmentService) UpdateDepartment(ctx context.Context, tx *gorm.DB, tenantID, id uuid.UUID, req dto.UpdateDepartmentRequest) (*dto.DepartmentResponse, error)`
- **func** `(s *departmentService) assertMoveAcyclic(ctx context.Context, db *gorm.DB, tenantID, movingID uuid.UUID, newParent *uuid.UUID) error` — assertMoveAcyclic rejects moves that would cycle through the moving node.
- **func** `(s *departmentService) DeactivateDepartment(ctx context.Context, tx *gorm.DB, tenantID, id uuid.UUID, force bool, reason *string) (*dto.DepartmentResponse, error)`

#### `internal/hr/organization/services/designation_service.go` (144 lines)
- **type** `DesignationService interface` — DesignationService handles job title/level lifecycle.
- **method** `DesignationService.CreateDesignation(ctx context.Context, tx *gorm.DB, tenantID uuid.UUID, req dto.CreateDesignationRequest, correlationID uuid.UUID) (*dto.DesignationResponse, error)`
- **method** `DesignationService.GetDesignation(ctx context.Context, db *gorm.DB, tenantID, id uuid.UUID) (*dto.DesignationResponse, error)`
- **method** `DesignationService.ListDesignations(ctx context.Context, db *gorm.DB, tenantID uuid.UUID, page, perPage int) (*dto.DesignationListResponse, error)`
- **method** `DesignationService.UpdateDesignation(ctx context.Context, tx *gorm.DB, tenantID, id uuid.UUID, req dto.UpdateDesignationRequest) (*dto.DesignationResponse, error)`
- **method** `DesignationService.DeactivateDesignation(ctx context.Context, tx *gorm.DB, tenantID, id uuid.UUID, reason *string) (*dto.DesignationResponse, error)`
- **type** `designationService struct`
- **func** `NewDesignationService(repo repositories.DesignationRepository, mappingRepo repositories.MappingRepository, publisher queue.EventPublisher, audit auditLogger) DesignationService` — NewDesignationService creates a DesignationService.
- **func** `mapDesignation(d *models.Designation) *dto.DesignationResponse`
- **func** `(s *designationService) CreateDesignation(ctx context.Context, tx *gorm.DB, tenantID uuid.UUID, req dto.CreateDesignationRequest, correlationID uuid.UUID) (*dto.DesignationResponse, error)`
- **func** `(s *designationService) GetDesignation(ctx context.Context, db *gorm.DB, tenantID, id uuid.UUID) (*dto.DesignationResponse, error)`
- **func** `(s *designationService) ListDesignations(ctx context.Context, db *gorm.DB, tenantID uuid.UUID, page, perPage int) (*dto.DesignationListResponse, error)`
- **func** `(s *designationService) UpdateDesignation(ctx context.Context, tx *gorm.DB, tenantID, id uuid.UUID, req dto.UpdateDesignationRequest) (*dto.DesignationResponse, error)`
- **func** `(s *designationService) DeactivateDesignation(ctx context.Context, tx *gorm.DB, tenantID, id uuid.UUID, reason *string) (*dto.DesignationResponse, error)`

#### `internal/hr/organization/services/event_consumer.go` (45 lines)
- **type** `EventConsumer interface` — EventConsumer handles identity domain events consumed by Module 1 (FR-E001..FR-E004).
- **method** `EventConsumer.HandleIdentityEvent(ctx context.Context, db *gorm.DB, routingKey string, payload json.RawMessage, correlationID uuid.UUID) error` — Handlers are idempotent: redelivery must converge to the same state (C7).
- **type** `eventConsumer struct`
- **func** `NewEventConsumer(mappingSvc MappingService) EventConsumer` — NewEventConsumer creates the consumer over org services.
- **func** `(c *eventConsumer) HandleIdentityEvent(ctx context.Context, db *gorm.DB, routingKey string, payload json.RawMessage, correlationID uuid.UUID) error`

#### `internal/hr/organization/services/helpers.go` (136 lines)
- **type** `userChecker interface` — userChecker validates user existence/activity through Module 0 (injected, mocked in tests).
- **method** `userChecker.GetByID(ctx context.Context, db *gorm.DB, tenantID, id uuid.UUID) (*identityport.User, error)`
- **type** `auditLogger interface` — auditLogger records org mutations through Module 0 audit transport.
- **method** `auditLogger.Log(ctx context.Context, tx *gorm.DB, tenantID, userID, action, resource, resourceID string, metadata interface{}, ip, userAgent string) error`
- **func** `parseUUID(s *string) (*uuid.UUID, error)` — parseUUID parses a UUID string pointer, returning nil when unset.
- **func** `uuidToString(id *uuid.UUID) *string` — uuidToString renders a UUID pointer for DTOs.
- **func** `assertDeptAcyclic(ctx context.Context, db *gorm.DB, tenantID uuid.UUID, deptRepo repositories.DepartmentRepository, startParent *uuid.UUID) error` — assertDeptAcyclic walks ancestors to reject cycles and enforce the depth cap. FR-H001..FR-H004.
- **func** `deptDepth(ctx context.Context, db *gorm.DB, tenantID uuid.UUID, deptRepo repositories.DepartmentRepository, dept *models.Department) (int, []string, error)` — deptDepth computes depth of a department (root = 0).
- **func** `mapDeptToResponse(ctx context.Context, db *gorm.DB, tenantID uuid.UUID, deptRepo repositories.DepartmentRepository, d *models.Department) (*dto.DepartmentResponse, error)`
- **func** `publishOrOutbox(ctx context.Context, tx *gorm.DB, publisher queue.EventPublisher, exchange string, evt events.Event)` — publishOrOutbox publishes an org event; on broker failure the event lands in the outbox table for later retry, so queue downtime never blocks the API (FR-E005). A nil tx (unit tests without a database) skips the outbox write.

#### `internal/hr/organization/services/mapping_service.go` (326 lines)
- **type** `MappingService interface` — MappingService handles user→org mappings with hierarchy guards.
- **method** `MappingService.CreateMapping(ctx context.Context, tx *gorm.DB, tenantID uuid.UUID, req dto.CreateMappingRequest, correlationID uuid.UUID) (*dto.MappingResponse, error)`
- **method** `MappingService.GetMapping(ctx context.Context, db *gorm.DB, tenantID, id uuid.UUID) (*dto.MappingResponse, error)`
- **method** `MappingService.ListUserMappings(ctx context.Context, db *gorm.DB, tenantID, userID uuid.UUID, page, perPage int) (*dto.MappingListResponse, error)`
- **method** `MappingService.UpdateMapping(ctx context.Context, tx *gorm.DB, tenantID, id uuid.UUID, req dto.UpdateMappingRequest) (*dto.MappingResponse, error)`
- **method** `MappingService.DeactivateMapping(ctx context.Context, tx *gorm.DB, tenantID, id uuid.UUID, req dto.DeactivateMappingRequest) (*dto.MappingResponse, error)`
- **method** `MappingService.DeactivateUserMappings(ctx context.Context, tx *gorm.DB, tenantID, userID uuid.UUID, correlationID uuid.UUID) error`
- **type** `mappingService struct`
- **func** `NewMappingService(` — NewMappingService creates a MappingService.
- **func** `mapMapping(m *models.Mapping) *dto.MappingResponse`
- **func** `(s *mappingService) checkUser(ctx context.Context, db *gorm.DB, tenantID, userID uuid.UUID) error` — checkUser resolves a user through Module 0: must exist, same tenant, active. FR-M001.
- **func** `(s *mappingService) checkRefs(ctx context.Context, db *gorm.DB, tenantID uuid.UUID, deptID, teamID, desigID, managerID *uuid.UUID, self uuid.UUID) error` — checkRefs validates department/team/designation/manager references. FR-M001/FR-M003/FR-M004.
- **func** `(s *mappingService) assertManagerAcyclic(ctx context.Context, db *gorm.DB, tenantID, self, manager uuid.UUID) error` — assertManagerAcyclic walks the manager chain to reject cycles. FR-M003.
- **func** `(s *mappingService) CreateMapping(ctx context.Context, tx *gorm.DB, tenantID uuid.UUID, req dto.CreateMappingRequest, correlationID uuid.UUID) (*dto.MappingResponse, error)`
- **func** `(s *mappingService) GetMapping(ctx context.Context, db *gorm.DB, tenantID, id uuid.UUID) (*dto.MappingResponse, error)`
- **func** `(s *mappingService) ListUserMappings(ctx context.Context, db *gorm.DB, tenantID, userID uuid.UUID, page, perPage int) (*dto.MappingListResponse, error)`
- **func** `(s *mappingService) UpdateMapping(ctx context.Context, tx *gorm.DB, tenantID, id uuid.UUID, req dto.UpdateMappingRequest) (*dto.MappingResponse, error)`
- **func** `(s *mappingService) DeactivateMapping(ctx context.Context, tx *gorm.DB, tenantID, id uuid.UUID, req dto.DeactivateMappingRequest) (*dto.MappingResponse, error)`
- **func** `(s *mappingService) DeactivateUserMappings(ctx context.Context, tx *gorm.DB, tenantID, userID uuid.UUID, correlationID uuid.UUID) error` — DeactivateUserMappings converges org state on identity UserDeactivated. FR-M005.

#### `internal/hr/organization/services/org_chart_service.go` (230 lines)
- **type** `OrgChartService interface` — OrgChartService builds the tenant forest, user chains, and headcounts. FR-O001..FR-O004.
- **method** `OrgChartService.GetChart(ctx context.Context, db *gorm.DB, tenantID uuid.UUID, maxDepth int, includeInactive bool) (*dto.OrgChartResponse, error)`
- **method** `OrgChartService.GetUserChain(ctx context.Context, db *gorm.DB, tenantID, userID uuid.UUID) (*dto.UserChainResponse, error)`
- **type** `chartCache interface` — chartCache is the minimal cache surface the org chart needs (satisfied by RedisClient).
- **method** `chartCache.Get(ctx context.Context, key string) (string, bool, error)`
- **method** `chartCache.Set(ctx context.Context, key string, value string, expiration time.Duration) error`
- **method** `chartCache.DeleteByPrefix(ctx context.Context, prefix string) error`
- **type** `ChartInvalidator interface` — ChartInvalidator drops cached org charts of a tenant after any org mutation.
- **method** `ChartInvalidator.InvalidateChart(ctx context.Context, tenantID uuid.UUID)`
- **func** `(s *orgChartService) InvalidateChart(ctx context.Context, tenantID uuid.UUID)` — InvalidateChart removes every cached chart variant (max_depth / include_inactive) for the tenant.
- **type** `orgChartService struct`
- **func** `NewOrgChartService(` — NewOrgChartService creates an OrgChartService. Cache may be nil (compute direct).
- **func** `chartCacheKey(tenantID uuid.UUID, maxDepth int, includeInactive bool) string`
- **func** `(s *orgChartService) GetChart(ctx context.Context, db *gorm.DB, tenantID uuid.UUID, maxDepth int, includeInactive bool) (*dto.OrgChartResponse, error)`
- **func** `(s *orgChartService) buildNode(ctx context.Context, db *gorm.DB, tenantID uuid.UUID, dept *models.Department, depth, maxDepth int, includeInactive bool) (*dto.OrgChartNode, error)`
- **func** `(s *orgChartService) GetUserChain(ctx context.Context, db *gorm.DB, tenantID, userID uuid.UUID) (*dto.UserChainResponse, error)`

#### `internal/hr/organization/services/team_service.go` (241 lines)
- **type** `TeamService interface` — TeamService handles team lifecycle and moves.
- **method** `TeamService.CreateTeam(ctx context.Context, tx *gorm.DB, tenantID uuid.UUID, req dto.CreateTeamRequest, correlationID uuid.UUID) (*dto.TeamResponse, error)`
- **method** `TeamService.GetTeam(ctx context.Context, db *gorm.DB, tenantID, id uuid.UUID) (*dto.TeamResponse, error)`
- **method** `TeamService.ListTeams(ctx context.Context, db *gorm.DB, tenantID uuid.UUID, departmentID *uuid.UUID, page, perPage int) (*dto.TeamListResponse, error)`
- **method** `TeamService.UpdateTeam(ctx context.Context, tx *gorm.DB, tenantID, id uuid.UUID, req dto.UpdateTeamRequest) (*dto.TeamResponse, error)`
- **method** `TeamService.DeactivateTeam(ctx context.Context, tx *gorm.DB, tenantID, id uuid.UUID, reason *string) (*dto.TeamResponse, error)`
- **type** `teamService struct`
- **func** `NewTeamService(` — NewTeamService creates a TeamService.
- **func** `(s *teamService) checkLead(ctx context.Context, db *gorm.DB, tenantID uuid.UUID, leadID *string) (*uuid.UUID, error)`
- **func** `(s *teamService) toResponse(ctx context.Context, db *gorm.DB, tenantID uuid.UUID, t *models.Team) (*dto.TeamResponse, error)`
- **func** `(s *teamService) CreateTeam(ctx context.Context, tx *gorm.DB, tenantID uuid.UUID, req dto.CreateTeamRequest, correlationID uuid.UUID) (*dto.TeamResponse, error)`
- **func** `(s *teamService) GetTeam(ctx context.Context, db *gorm.DB, tenantID, id uuid.UUID) (*dto.TeamResponse, error)`
- **func** `(s *teamService) ListTeams(ctx context.Context, db *gorm.DB, tenantID uuid.UUID, departmentID *uuid.UUID, page, perPage int) (*dto.TeamListResponse, error)`
- **func** `(s *teamService) UpdateTeam(ctx context.Context, tx *gorm.DB, tenantID, id uuid.UUID, req dto.UpdateTeamRequest) (*dto.TeamResponse, error)`
- **func** `(s *teamService) DeactivateTeam(ctx context.Context, tx *gorm.DB, tenantID, id uuid.UUID, reason *string) (*dto.TeamResponse, error)`

#### `internal/hr/organization/validators/validators.go` (29 lines)
- **func** `ValidateOrgCode(code string) bool` — ValidateOrgCode checks an org code against the slug-like format.
- **func** `IsReservedOrgCode(code string) bool` — IsReservedOrgCode checks platform-reserved org codes.

### M1 HR / Employee


#### `internal/hr/employee/access_facts.go` (28 lines)
- **func** `(m *Module) EmployedUserIDs(ctx context.Context, tenantID uuid.UUID) ([]uuid.UUID, error)` — EmployedUserIDs lists the users of the tenant with a current employee record.
- **func** `(m *Module) IsEmployed(ctx context.Context, tenantID, userID uuid.UUID) (bool, error)` — IsEmployed reports whether the user has a current employee record in the tenant.

#### `internal/hr/employee/catalogue.go` (24 lines)
- **func** `Catalogue() []access.PermissionDef` — Catalogue is HR/Employee's permission catalogue with the scopes predefined roles receive by default (docs/ARCHITECTURE.md §17). The app registers it with M0 at boot.

#### `internal/hr/employee/module.go` (130 lines)
- **type** `Module struct` — Module encapsulates the Module 2 Employee domain.
- **func** `NewModule(`
- **func** `(m *Module) RegisterModels() []interface{}`
- **func** `(m *Module) RegisterRoutes(router *gin.RouterGroup, idp identityport.HTTP)` — RegisterRoutes mounts the employee endpoints behind the M0 guards provided by idp.
- **func** `(m *Module) EmployeeService() services.EmployeeService`
- **func** `(m *Module) Directory() api.Directory { return m.empSvc }` — Directory is the employee directory published to the other HR contexts (employee/api).
- **func** `(m *Module) EventConsumer() services.EventConsumer`
- **type** `userDeactivationConverger struct{ svc services.EmployeeService }` — userDeactivationConverger adapts the employee service to the Module 0 deactivation hook (FR-EV001, synchronous).
- **func** `(c userDeactivationConverger) DeactivateUserMappings(ctx context.Context, tx *gorm.DB, tenantID, userID, _ uuid.UUID) error`
- **func** `(m *Module) UserDeactivationConverger() identityport.DeactivationHook` — UserDeactivationConverger returns the hook that keeps employee profiles in step with Module 0 user deactivation.

#### `internal/hr/employee/api/api.go` (19 lines)
_Package:_ Package api is what HR/Employee publishes to the other HR contexts: the employee directory. Other contexts depend on this package and on employee/dto only, never on employee services, repositories or models (import-boundary test in internal/hr).
- **type** `Directory interface` — Directory reads and creates employee records.
- **method** `Directory.GetByID(ctx context.Context, tenantID, id uuid.UUID) (*dto.EmployeeResponse, error)`
- **method** `Directory.GetByUserID(ctx context.Context, tenantID, userID uuid.UUID) (*dto.EmployeeResponse, error)`
- **method** `Directory.List(ctx context.Context, tenantID uuid.UUID, filter dto.EmployeeFilter) ([]dto.EmployeeResponse, int64, error)`
- **method** `Directory.CreateEmployee(ctx context.Context, tenantID uuid.UUID, req dto.CreateEmployeeRequest) (*dto.EmployeeResponse, error)`

#### `internal/hr/employee/controllers/document_controller.go` (99 lines)
- **type** `DocumentController struct`
- **func** `NewDocumentController(svc services.EmployeeDocumentService) *DocumentController`
- **func** `(ctrl *DocumentController) Upload(c *gin.Context)`
- **func** `(ctrl *DocumentController) List(c *gin.Context)`
- **func** `(ctrl *DocumentController) Verify(c *gin.Context)`

#### `internal/hr/employee/controllers/employee_controller.go` (224 lines)
- **type** `EmployeeController struct`
- **func** `NewEmployeeController(svc services.EmployeeService) *EmployeeController`
- **func** `(ctrl *EmployeeController) Create(c *gin.Context)`
- **func** `(ctrl *EmployeeController) GetByID(c *gin.Context)`
- **func** `(ctrl *EmployeeController) GetMe(c *gin.Context)`
- **func** `(ctrl *EmployeeController) UpdateMe(c *gin.Context)`
- **func** `(ctrl *EmployeeController) List(c *gin.Context)`
- **func** `(ctrl *EmployeeController) Update(c *gin.Context)`
- **func** `(ctrl *EmployeeController) TransitionStatus(c *gin.Context)`
- **func** `(ctrl *EmployeeController) Deactivate(c *gin.Context)`

#### `internal/hr/employee/controllers/response.go` (93 lines)
- **type** `successEnvelope struct`
- **type** `errorEnvelope struct`
- **type** `errorDetail struct`
- **func** `respondSuccess(c *gin.Context, status int, data interface{}, meta interface{})`
- **func** `respondError(c *gin.Context, err error)`
- **func** `getTenantID(c *gin.Context) (uuid.UUID, bool)`
- **func** `getUserID(c *gin.Context) (uuid.UUID, bool)`
- **func** `respondBadRequest(c *gin.Context, msg string)` — respondBadRequest returns a 400 in the standard error envelope.
- **func** `respondBindError(c *gin.Context, err error)` — respondBindError converts a request-binding failure into field-level validation details.
- **func** `respondValidationError(c *gin.Context, details []sharedErrors.ValidationErrorDetail)` — respondValidationError returns field-level validation details.

#### `internal/hr/employee/controllers/statutory_controller.go` (78 lines)
- **type** `StatutoryController struct`
- **func** `NewStatutoryController(svc services.EmployeeStatutoryService) *StatutoryController`
- **func** `(ctrl *StatutoryController) Get(c *gin.Context)`
- **func** `(ctrl *StatutoryController) Upsert(c *gin.Context)`

#### `internal/hr/employee/controllers/timeline_controller.go` (39 lines)
- **type** `TimelineController struct`
- **func** `NewTimelineController(svc services.EmployeeTimelineService) *TimelineController`
- **func** `(ctrl *TimelineController) List(c *gin.Context)`

#### `internal/hr/employee/dto/document_dto.go` (28 lines)
- **type** `UploadDocumentRequest struct`
- **type** `DocumentResponse struct`

#### `internal/hr/employee/dto/employee_dto.go` (104 lines)
- **type** `CreateEmployeeRequest struct`
- **type** `UpdateEmployeeRequest struct`
- **type** `UpdateSelfContactRequest struct`
- **type** `TransitionStatusRequest struct`
- **type** `EmployeeFilter struct`
- **type** `EmployeeResponse struct`
- **type** `EmploymentDetail struct`
- **type** `EmployeeContact struct`

#### `internal/hr/employee/dto/statutory_dto.go` (21 lines)
- **type** `UpdateStatutoryRequest struct`
- **type** `StatutoryResponse struct`

#### `internal/hr/employee/dto/timeline_dto.go` (17 lines)
- **type** `TimelineResponse struct`

#### `internal/hr/employee/events/events.go` (45 lines)
- **type** `EventEnvelope struct`
- **func** `NewEventEnvelope(eventType, routingKey string, tenantID uuid.UUID, payload interface{}) EventEnvelope`

#### `internal/hr/employee/models/contact.go` (22 lines)
- **type** `EmployeeContact struct` — EmployeeContact stores personal contact details and emergency contacts.
- **func** `(EmployeeContact) TableName() string`

#### `internal/hr/employee/models/document.go` (25 lines)
- **type** `EmployeeDocument struct` — EmployeeDocument holds metadata and verification state of employee documents.
- **func** `(EmployeeDocument) TableName() string`

#### `internal/hr/employee/models/employment.go` (26 lines)
- **type** `EmploymentDetail struct` — EmploymentDetail tracks employment contracts, lifecycle dates, and notice periods.
- **func** `(EmploymentDetail) TableName() string`

#### `internal/hr/employee/models/outbox.go` (25 lines)
- **type** `EmployeeEventsOutbox struct` — EmployeeEventsOutbox provides durable outbox persistence for domain events.
- **func** `(EmployeeEventsOutbox) TableName() string`

#### `internal/hr/employee/models/profile.go` (36 lines)
- **type** `EmployeeProfile struct` — EmployeeProfile represents the core operational profile of an employee.
- **func** `(EmployeeProfile) TableName() string`

#### `internal/hr/employee/models/statutory.go` (21 lines)
- **type** `EmployeeStatutory struct` — EmployeeStatutory stores sensitive tax identifiers and banking coordinates.
- **func** `(EmployeeStatutory) TableName() string`

#### `internal/hr/employee/models/timeline.go` (22 lines)
- **type** `EmployeeTimeline struct` — EmployeeTimeline records chronological career, status, and org transitions.
- **func** `(EmployeeTimeline) TableName() string`

#### `internal/hr/employee/repositories/document_repository.go` (60 lines)
- **type** `employeeDocumentRepository struct`
- **func** `NewEmployeeDocumentRepository(db *gorm.DB) EmployeeDocumentRepository`
- **func** `(r *employeeDocumentRepository) Create(ctx context.Context, tx *gorm.DB, doc *models.EmployeeDocument) error`
- **func** `(r *employeeDocumentRepository) GetByID(ctx context.Context, tenantID, docID uuid.UUID) (*models.EmployeeDocument, error)`
- **func** `(r *employeeDocumentRepository) ListByProfileID(ctx context.Context, tenantID, profileID uuid.UUID) ([]models.EmployeeDocument, error)`
- **func** `(r *employeeDocumentRepository) Update(ctx context.Context, tx *gorm.DB, doc *models.EmployeeDocument) error`

#### `internal/hr/employee/repositories/interfaces.go` (40 lines)
- **type** `EmployeeProfileRepository interface`
- **method** `EmployeeProfileRepository.Create(ctx context.Context, tx *gorm.DB, profile *models.EmployeeProfile) error`
- **method** `EmployeeProfileRepository.GetByID(ctx context.Context, tenantID, id uuid.UUID) (*models.EmployeeProfile, error)`
- **method** `EmployeeProfileRepository.GetByUserID(ctx context.Context, tenantID, userID uuid.UUID) (*models.EmployeeProfile, error)`
- **method** `EmployeeProfileRepository.GetByCode(ctx context.Context, tenantID uuid.UUID, code string) (*models.EmployeeProfile, error)`
- **method** `EmployeeProfileRepository.List(ctx context.Context, tenantID uuid.UUID, filter dto.EmployeeFilter) ([]models.EmployeeProfile, int64, error)`
- **method** `EmployeeProfileRepository.Update(ctx context.Context, tx *gorm.DB, profile *models.EmployeeProfile) error`
- **method** `EmployeeProfileRepository.UpdateEmploymentDetail(ctx context.Context, tx *gorm.DB, detail *models.EmploymentDetail) error`
- **method** `EmployeeProfileRepository.UpdateContact(ctx context.Context, tx *gorm.DB, contact *models.EmployeeContact) error`
- **method** `EmployeeProfileRepository.Delete(ctx context.Context, tx *gorm.DB, tenantID, id uuid.UUID) error`
- **method** `EmployeeProfileRepository.WithTx(tx *gorm.DB) EmployeeProfileRepository`
- **type** `EmployeeStatutoryRepository interface`
- **method** `EmployeeStatutoryRepository.GetByProfileID(ctx context.Context, tenantID, profileID uuid.UUID) (*models.EmployeeStatutory, error)`
- **method** `EmployeeStatutoryRepository.Upsert(ctx context.Context, tx *gorm.DB, statutory *models.EmployeeStatutory) error`
- **type** `EmployeeDocumentRepository interface`
- **method** `EmployeeDocumentRepository.Create(ctx context.Context, tx *gorm.DB, doc *models.EmployeeDocument) error`
- **method** `EmployeeDocumentRepository.GetByID(ctx context.Context, tenantID, docID uuid.UUID) (*models.EmployeeDocument, error)`
- **method** `EmployeeDocumentRepository.ListByProfileID(ctx context.Context, tenantID, profileID uuid.UUID) ([]models.EmployeeDocument, error)`
- **method** `EmployeeDocumentRepository.Update(ctx context.Context, tx *gorm.DB, doc *models.EmployeeDocument) error`
- **type** `EmployeeTimelineRepository interface`
- **method** `EmployeeTimelineRepository.Create(ctx context.Context, tx *gorm.DB, timeline *models.EmployeeTimeline) error`
- **method** `EmployeeTimelineRepository.ListByProfileID(ctx context.Context, tenantID, profileID uuid.UUID) ([]models.EmployeeTimeline, error)`

#### `internal/hr/employee/repositories/profile_repository.go` (152 lines)
- **type** `employeeProfileRepository struct`
- **func** `NewEmployeeProfileRepository(db *gorm.DB) EmployeeProfileRepository`
- **func** `(r *employeeProfileRepository) WithTx(tx *gorm.DB) EmployeeProfileRepository`
- **func** `(r *employeeProfileRepository) Create(ctx context.Context, tx *gorm.DB, profile *models.EmployeeProfile) error`
- **func** `(r *employeeProfileRepository) GetByID(ctx context.Context, tenantID, id uuid.UUID) (*models.EmployeeProfile, error)`
- **func** `(r *employeeProfileRepository) GetByUserID(ctx context.Context, tenantID, userID uuid.UUID) (*models.EmployeeProfile, error)`
- **func** `(r *employeeProfileRepository) GetByCode(ctx context.Context, tenantID uuid.UUID, code string) (*models.EmployeeProfile, error)`
- **func** `(r *employeeProfileRepository) List(ctx context.Context, tenantID uuid.UUID, filter dto.EmployeeFilter) ([]models.EmployeeProfile, int64, error)`
- **func** `(r *employeeProfileRepository) Update(ctx context.Context, tx *gorm.DB, profile *models.EmployeeProfile) error`
- **func** `(r *employeeProfileRepository) UpdateEmploymentDetail(ctx context.Context, tx *gorm.DB, detail *models.EmploymentDetail) error`
- **func** `(r *employeeProfileRepository) UpdateContact(ctx context.Context, tx *gorm.DB, contact *models.EmployeeContact) error`
- **func** `(r *employeeProfileRepository) Delete(ctx context.Context, tx *gorm.DB, tenantID, id uuid.UUID) error`

#### `internal/hr/employee/repositories/statutory_repository.go` (46 lines)
- **type** `employeeStatutoryRepository struct`
- **func** `NewEmployeeStatutoryRepository(db *gorm.DB) EmployeeStatutoryRepository`
- **func** `(r *employeeStatutoryRepository) GetByProfileID(ctx context.Context, tenantID, profileID uuid.UUID) (*models.EmployeeStatutory, error)`
- **func** `(r *employeeStatutoryRepository) Upsert(ctx context.Context, tx *gorm.DB, statutory *models.EmployeeStatutory) error`

#### `internal/hr/employee/repositories/timeline_repository.go` (37 lines)
- **type** `employeeTimelineRepository struct`
- **func** `NewEmployeeTimelineRepository(db *gorm.DB) EmployeeTimelineRepository`
- **func** `(r *employeeTimelineRepository) Create(ctx context.Context, tx *gorm.DB, timeline *models.EmployeeTimeline) error`
- **func** `(r *employeeTimelineRepository) ListByProfileID(ctx context.Context, tenantID, profileID uuid.UUID) ([]models.EmployeeTimeline, error)`

#### `internal/hr/employee/routes/routes.go` (66 lines)
- **type** `Controllers struct` — RegisterRoutes wires employee endpoint groups with Module 0 middleware and RBAC. Controllers groups the employee HTTP handlers.
- **func** `RegisterRoutes(router *gin.RouterGroup, idp identityport.HTTP, c Controllers)`

#### `internal/hr/employee/services/document_service.go` (121 lines)
- **type** `EmployeeDocumentService interface`
- **method** `EmployeeDocumentService.Upload(ctx context.Context, tenantID, profileID uuid.UUID, req dto.UploadDocumentRequest) (*dto.DocumentResponse, error)`
- **method** `EmployeeDocumentService.List(ctx context.Context, tenantID, profileID uuid.UUID) ([]dto.DocumentResponse, error)`
- **method** `EmployeeDocumentService.Verify(ctx context.Context, tenantID, profileID, docID, verifierID uuid.UUID) (*dto.DocumentResponse, error)`
- **type** `employeeDocumentService struct`
- **func** `NewEmployeeDocumentService(`
- **func** `(s *employeeDocumentService) Upload(ctx context.Context, tenantID, profileID uuid.UUID, req dto.UploadDocumentRequest) (*dto.DocumentResponse, error)`
- **func** `(s *employeeDocumentService) List(ctx context.Context, tenantID, profileID uuid.UUID) ([]dto.DocumentResponse, error)`
- **func** `(s *employeeDocumentService) Verify(ctx context.Context, tenantID, profileID, docID, verifierID uuid.UUID) (*dto.DocumentResponse, error)`
- **func** `toDocumentResponse(d *models.EmployeeDocument) *dto.DocumentResponse`

#### `internal/hr/employee/services/employee_service.go` (560 lines)
- **type** `EmployeeService interface`
- **method** `EmployeeService.CreateEmployee(ctx context.Context, tenantID uuid.UUID, req dto.CreateEmployeeRequest) (*dto.EmployeeResponse, error)`
- **method** `EmployeeService.GetByID(ctx context.Context, tenantID, id uuid.UUID) (*dto.EmployeeResponse, error)`
- **method** `EmployeeService.GetByUserID(ctx context.Context, tenantID, userID uuid.UUID) (*dto.EmployeeResponse, error)`
- **method** `EmployeeService.List(ctx context.Context, tenantID uuid.UUID, filter dto.EmployeeFilter) ([]dto.EmployeeResponse, int64, error)`
- **method** `EmployeeService.Update(ctx context.Context, tenantID, id uuid.UUID, req dto.UpdateEmployeeRequest) (*dto.EmployeeResponse, error)`
- **method** `EmployeeService.UpdateSelfContact(ctx context.Context, tenantID, userID uuid.UUID, req dto.UpdateSelfContactRequest) (*dto.EmployeeResponse, error)`
- **method** `EmployeeService.TransitionStatus(ctx context.Context, tenantID, id uuid.UUID, req dto.TransitionStatusRequest) (*dto.EmployeeResponse, error)`
- **method** `EmployeeService.Deactivate(ctx context.Context, tenantID, id uuid.UUID) error`
- **method** `EmployeeService.ConvergeUserDeactivation(ctx context.Context, tx *gorm.DB, tenantID, userID uuid.UUID) error` — profile or the profile is already final (resigned / terminated / inactive).
- **type** `employeeService struct`
- **func** `NewEmployeeService(`
- **func** `(s *employeeService) CreateEmployee(ctx context.Context, tenantID uuid.UUID, req dto.CreateEmployeeRequest) (*dto.EmployeeResponse, error)`
- **func** `(s *employeeService) GetByID(ctx context.Context, tenantID, id uuid.UUID) (*dto.EmployeeResponse, error)`
- **func** `(s *employeeService) GetByUserID(ctx context.Context, tenantID, userID uuid.UUID) (*dto.EmployeeResponse, error)`
- **func** `(s *employeeService) List(ctx context.Context, tenantID uuid.UUID, filter dto.EmployeeFilter) ([]dto.EmployeeResponse, int64, error)`
- **func** `(s *employeeService) Update(ctx context.Context, tenantID, id uuid.UUID, req dto.UpdateEmployeeRequest) (*dto.EmployeeResponse, error)`
- **func** `(s *employeeService) UpdateSelfContact(ctx context.Context, tenantID, userID uuid.UUID, req dto.UpdateSelfContactRequest) (*dto.EmployeeResponse, error)`
- **func** `(s *employeeService) TransitionStatus(ctx context.Context, tenantID, id uuid.UUID, req dto.TransitionStatusRequest) (*dto.EmployeeResponse, error)`
- **func** `(s *employeeService) Deactivate(ctx context.Context, tenantID, id uuid.UUID) error`
- **func** `toEmployeeResponse(p *models.EmployeeProfile) *dto.EmployeeResponse`
- **func** `applyStatusEffects(d *models.EmploymentDetail, req dto.TransitionStatusRequest, oldStatus, newStatus string, now time.Time) string` — applyStatusEffects applies the lifecycle side effects of a status change to the employment details and returns the timeline event type: FR-ED003 (confirmation), FR-ED004 (notice: resignation + tentative exit date from the notice period) and FR-ED005 (final exit stamps the exit date). detail may be nil.
- **func** `(s *employeeService) ConvergeUserDeactivation(ctx context.Context, tx *gorm.DB, tenantID, userID uuid.UUID) error`

#### `internal/hr/employee/services/event_consumer.go` (52 lines)
- **type** `EventConsumer interface`
- **method** `EventConsumer.HandleUserDeactivated(ctx context.Context, tenantID, userID uuid.UUID) error`
- **type** `eventConsumer struct`
- **func** `NewEventConsumer(empSvc EmployeeService, logger *zerolog.Logger) EventConsumer`
- **func** `(c *eventConsumer) HandleUserDeactivated(ctx context.Context, tenantID, userID uuid.UUID) error`

#### `internal/hr/employee/services/statutory_service.go` (102 lines)
- **type** `EmployeeStatutoryService interface`
- **method** `EmployeeStatutoryService.GetByProfileID(ctx context.Context, tenantID, profileID uuid.UUID, sensitive bool) (*dto.StatutoryResponse, error)`
- **method** `EmployeeStatutoryService.Upsert(ctx context.Context, tenantID, profileID uuid.UUID, req dto.UpdateStatutoryRequest) (*dto.StatutoryResponse, error)`
- **type** `employeeStatutoryService struct`
- **func** `NewEmployeeStatutoryService(`
- **func** `(s *employeeStatutoryService) GetByProfileID(ctx context.Context, tenantID, profileID uuid.UUID, sensitive bool) (*dto.StatutoryResponse, error)`
- **func** `(s *employeeStatutoryService) Upsert(ctx context.Context, tenantID, profileID uuid.UUID, req dto.UpdateStatutoryRequest) (*dto.StatutoryResponse, error)`
- **func** `maskString(s string) string`

#### `internal/hr/employee/services/timeline_service.go` (66 lines)
- **type** `EmployeeTimelineService interface`
- **method** `EmployeeTimelineService.List(ctx context.Context, tenantID, profileID uuid.UUID) ([]dto.TimelineResponse, error)`
- **type** `employeeTimelineService struct`
- **func** `NewEmployeeTimelineService(`
- **func** `(s *employeeTimelineService) List(ctx context.Context, tenantID, profileID uuid.UUID) ([]dto.TimelineResponse, error)`
- **func** `toTimelineResponse(t *models.EmployeeTimeline) *dto.TimelineResponse`

#### `internal/hr/employee/validators/validators.go` (97 lines)
- **func** `ValidateEmployeeCode(code string) error`
- **func** `ValidateStatus(status string) error`
- **func** `ValidateEmploymentType(empType string) error`
- **func** `ValidateTransition(from, to string) error` — ValidateTransition enforces the employee status state machine (from != to).
- **func** `ValidateEmergencyContacts(raw string) error` — ValidateEmergencyContacts checks FR-EC001: a JSON array of {name, relation, phone} objects (name and phone required). An empty string is accepted and means "no contacts".

### M1 HR / Attendance


#### `internal/hr/attendance/catalogue.go` (19 lines)
- **func** `Catalogue() []access.PermissionDef` — Catalogue is HR/Attendance's permission catalogue with the scopes predefined roles receive by default (docs/ARCHITECTURE.md §17). The app registers it with M0 at boot.

#### `internal/hr/attendance/module.go` (116 lines)
_Package:_ Package attendance wires Module 3 (Attendance, Shifts & Time Tracking) into the application.
- **type** `Module struct` — Module is the Module 3 DI container.
- **func** `NewModule(db *gorm.DB, publisher queue.EventPublisher, employees employeeAPI.Directory, audit identityport.Auditor) *Module` — NewModule wires repositories, services and controllers over Module 0 audit and Module 2 employees.
- **func** `(m *Module) RegisterModels() []interface{}` — RegisterModels lists Module 3 models for AutoMigrate (dev path; SQL 000026–000031 in prod — A3-06).
- **type** `RouteDeps struct` — RouteDeps is the Module 0 + shared infrastructure the routes need (connections C4).
- **func** `(m *Module) RegisterRoutes(rg *gin.RouterGroup, d RouteDeps)` — RegisterRoutes mounts Module 3 under rg using Module 0 tenant/auth/RBAC middleware.
- **func** `(m *Module) LeaveSync() services.LeaveSync { return m.leaveSync }` — LeaveSync exposes contract C10 for Module 4 (D3-17).
- **func** `(m *Module) Relay() *services.OutboxRelay { return m.relay }` — Relay exposes the outbox relay; cmd/main.go runs it until shutdown.
- **type** `employeeDirectory struct` — employeeDirectory adapts the HR/Employee directory to services.EmployeeDirectory (connections C2).
- **func** `(d employeeDirectory) GetByUserID(ctx context.Context, tenantID, userID uuid.UUID) (*employeeDTO.EmployeeResponse, error)`
- **func** `(d employeeDirectory) GetByID(ctx context.Context, tenantID, id uuid.UUID) (*employeeDTO.EmployeeResponse, error)`
- **func** `(d employeeDirectory) CountWorking(ctx context.Context, tenantID uuid.UUID) (int64, error)` — CountWorking sums Module 2 list totals for the statuses allowed to work (AT-002, AT-020).

#### `internal/hr/attendance/calc/time.go` (118 lines)
_Package:_ Package calc holds Module 3's pure time math (spec AT-005..AT-010). No I/O, no dependencies.
- **type** `ShiftSpec struct` — ShiftSpec is the subset of a shift needed for attendance math. Location nil means UTC.
- **func** `ParseHHMM(s string) (int, error)` — ParseHHMM converts "HH:MM" to minutes from midnight.
- **func** `twoDigits(s string) (int, bool)`
- **func** `FormatHHMM(minutes int) string` — FormatHHMM renders minutes from midnight as "HH:MM".
- **func** `IsNightShift(start, end int) bool { return end <= start }` — IsNightShift reports whether a shift crosses midnight (FR-SH003).
- **func** `SpanMinutes(start, end int) int` — SpanMinutes is the shift length including midnight wrap.
- **func** `ExpectedMinutes(s *ShiftSpec) int` — ExpectedMinutes is span minus break, never negative; nil shift → DefaultExpectedMinutes (AT-008).
- **func** `Thresholds(s *ShiftSpec) (full, half int)` — Thresholds returns full- and half-day minutes, defaulting to expected and expected/2 (AT-010).
- **func** `location(s *ShiftSpec) *time.Location`
- **func** `AttendanceDate(at time.Time, s *ShiftSpec) time.Time` — AttendanceDate returns the day an IN at `at` belongs to, as UTC midnight (AT-005). Night shifts own early-morning punches up to end+4h, clamped so they never pass the shift start.
- **func** `ShiftStart(date time.Time, s *ShiftSpec) time.Time` — ShiftStart is the shift's start instant on the given attendance date.

#### `internal/hr/attendance/calc/totals.go` (121 lines)
- **type** `Punch struct` — Punch is one non-superseded IN/OUT instant.
- **type** `Totals struct` — Totals is the computed state of one attendance record (FR-AR002).
- **func** `sorted(punches []Punch) []Punch`
- **func** `OpenSessionStart(punches []Punch, now time.Time) (time.Time, bool)` — OpenSessionStart returns the last IN without a later OUT, if younger than SessionAbandonAfter (AT-003/AT-006).
- **func** `ComputeTotals(punches []Punch, s *ShiftSpec, date, now time.Time) Totals` — ComputeTotals derives work, break, late, overtime and status for one record (AT-006..AT-010). A dangling IN followed by another IN is dropped; durations are summed then floored to minutes.
- **func** `lateMinutes(firstIn *time.Time, s *ShiftSpec, date time.Time) int` — lateMinutes is AT-007: minutes past shift start + grace; 0 without a shift.
- **func** `status(t Totals, s *ShiftSpec) string` — status is AT-010: open session → present; else by full/half-day thresholds.

#### `internal/hr/attendance/controllers/attendance_controller.go` (110 lines)
- **type** `AttendanceController struct` — AttendanceController serves punches and attendance reads (A1–A3, A7–A9).
- **func** `NewAttendanceController(p services.PunchService, q services.QueryService) *AttendanceController` — NewAttendanceController builds an AttendanceController.
- **func** `(ctl *AttendanceController) Punch(c *gin.Context)` — Punch handles POST /attendance/punch (A1).
- **func** `(ctl *AttendanceController) Today(c *gin.Context)` — Today handles GET /attendance/me/today (A2).
- **func** `(ctl *AttendanceController) Mine(c *gin.Context)` — Mine handles GET /attendance/me (A3); no employee id is accepted (NFR-SEC002).
- **func** `(ctl *AttendanceController) List(c *gin.Context)` — List handles GET /attendance/records (A7).
- **func** `(ctl *AttendanceController) Detail(c *gin.Context)` — Detail handles GET /attendance/records/:id (A8).
- **func** `(ctl *AttendanceController) Summary(c *gin.Context)` — Summary handles GET /attendance/summary (A9).

#### `internal/hr/attendance/controllers/regularization_controller.go` (101 lines)
- **type** `RegularizationController struct` — RegularizationController serves correction requests (A4–A6, A10–A12).
- **func** `NewRegularizationController(s services.RegularizationService) *RegularizationController` — NewRegularizationController builds a RegularizationController.
- **func** `(ctl *RegularizationController) Create(c *gin.Context)` — Create handles POST /attendance/regularizations (A4).
- **func** `(ctl *RegularizationController) ListMine(c *gin.Context)` — ListMine handles GET /attendance/regularizations/me (A5).
- **func** `(ctl *RegularizationController) List(c *gin.Context)` — List handles GET /attendance/regularizations (A10).
- **type** `regAction func(ctx context.Context, a services.Actor, id uuid.UUID, req dto.ReviewRequest) (*dto.RegularizationResponse, error)`
- **func** `(ctl *RegularizationController) act(c *gin.Context, fn regAction, withBody bool)` — act runs an id-addressed action with an optional review body.
- **func** `(ctl *RegularizationController) Cancel(c *gin.Context)` — Cancel handles POST /attendance/regularizations/:id/cancel (A6).
- **func** `(ctl *RegularizationController) Approve(c *gin.Context) { ctl.act(c, ctl.svc.Approve, true) }` — Approve handles POST /attendance/regularizations/:id/approve (A11).
- **func** `(ctl *RegularizationController) Reject(c *gin.Context) { ctl.act(c, ctl.svc.Reject, true) }` — Reject handles POST /attendance/regularizations/:id/reject (A12).

#### `internal/hr/attendance/controllers/respond.go` (106 lines)
_Package:_ Package controllers is Module 3's thin HTTP edge: parse → validate → service → envelope.
- **type** `errorBody struct`
- **func** `ok(c *gin.Context, status int, data interface{})`
- **func** `okList(c *gin.Context, data interface{}, meta dto.PageMeta)`
- **func** `fail(c *gin.Context, err error)` — fail maps AppErrors to their status after shared normalization (unique violation → 409, not found → 404); anything else is an opaque 500 (no internals leaked).
- **func** `bindJSON(c *gin.Context, dst interface{}) bool` — bindJSON decodes and validates a body; failures become VALIDATION_ERROR with field details.
- **func** `snake(s string) string`
- **func** `actorFrom(c *gin.Context) (services.Actor, bool)` — actorFrom builds the caller from Module 0 middleware context only (NFR-SEC002).
- **func** `pathID(c *gin.Context, name string) (uuid.UUID, bool)` — pathID parses a :param UUID; malformed ids are 400.
- **func** `page(c *gin.Context) dto.Page`

#### `internal/hr/attendance/controllers/shift_controller.go` (137 lines)
- **type** `ShiftController struct` — ShiftController serves shifts and assignments (S1–S7).
- **func** `NewShiftController(s services.ShiftService, a services.AssignmentService) *ShiftController` — NewShiftController builds a ShiftController.
- **func** `(ctl *ShiftController) Create(c *gin.Context)` — Create handles POST /shifts (S1).
- **func** `(ctl *ShiftController) List(c *gin.Context)` — List handles GET /shifts (S2).
- **func** `(ctl *ShiftController) Get(c *gin.Context)` — Get handles GET /shifts/:id (S3).
- **func** `(ctl *ShiftController) Update(c *gin.Context)` — Update handles PATCH /shifts/:id (S4).
- **func** `(ctl *ShiftController) Deactivate(c *gin.Context)` — Deactivate handles POST /shifts/:id/deactivate (S5).
- **func** `(ctl *ShiftController) Assign(c *gin.Context)` — Assign handles POST /shifts/:id/assignments (S6).
- **func** `(ctl *ShiftController) ListAssignments(c *gin.Context)` — ListAssignments handles GET /shift-assignments?employee_id= (S7).

#### `internal/hr/attendance/dto/dto.go` (212 lines)
_Package:_ Package dto holds Module 3 request/response shapes (specification.md §5). Requests ≠ responses.
- **type** `PunchRequest struct` — PunchRequest is A1. No time field exists: punch time is the server clock (AT-001).
- **type** `CreateShiftRequest struct` — CreateShiftRequest is S1 (FR-SH001).
- **type** `UpdateShiftRequest struct` — UpdateShiftRequest is S4; code is immutable and therefore absent (FR-SH005).
- **type** `AssignShiftRequest struct` — AssignShiftRequest is S6 (FR-SA001). Dates are YYYY-MM-DD.
- **type** `CreateRegularizationRequest struct` — CreateRegularizationRequest is A4 (FR-RG001).
- **type** `ReviewRequest struct` — ReviewRequest is A11/A12; reject requires a comment (AT-016, enforced in service).
- **type** `ShiftResponse struct` — ShiftResponse serializes a shift.
- **type** `AssignmentResponse struct` — AssignmentResponse serializes a shift assignment.
- **type** `PunchResponse struct` — PunchResponse serializes a punch; the IP address is never exposed (FR-EV003, security.md).
- **type** `RecordResponse struct` — RecordResponse serializes an attendance record.
- **type** `PunchResult struct` — PunchResult is A1's response.
- **type** `RecordDetailResponse struct` — RecordDetailResponse is A8's response.
- **type** `TodayResponse struct` — TodayResponse is A2's response.
- **type** `RegularizationResponse struct` — RegularizationResponse serializes a regularization request.
- **type** `SummaryResponse struct` — SummaryResponse is A9's response (AT-020).
- **type** `PageMeta struct` — PageMeta is the list envelope meta.
- **type** `Page struct` — Page is a clamped page request (G13: bad input never errors).
- **func** `ParsePage(pageStr, perPageStr string) Page` — ParsePage clamps raw query values to valid bounds (spec §5, EC-16).
- **func** `(p Page) Offset() int { return (p.Page - 1) * p.PerPage }` — Offset is the SQL offset for this page.
- **func** `(p Page) Meta(total int64) PageMeta` — Meta builds list metadata for a total count.

#### `internal/hr/attendance/events/events.go` (76 lines)
_Package:_ Package events defines Module 3 produced events (FR-EV001). Payloads carry no IP/device/coordinates (FR-EV003).
- **type** `Envelope struct` — Envelope is the standard JAAS event envelope.
- **type** `Source struct` — Source identifies who and when an event came from; OccurredAt comes from the service clock.
- **func** `New(routingKey string, src Source, payload interface{}) Envelope` — New wraps a payload in the envelope.
- **type** `PunchPayload struct` — PunchPayload is published for attendance.punch.in/out.
- **type** `RegularizationPayload struct` — RegularizationPayload is published for attendance.regularization.*.

#### `internal/hr/attendance/models/attendance.go` (117 lines)
- **type** `AttendanceRecord struct` — AttendanceRecord is one employee's computed day (FR-AR001/FR-AR002). Never deleted (AT-019).
- **func** `(r *AttendanceRecord) BeforeCreate(tx *gorm.DB) error` — BeforeCreate assigns a UUID v4 when unset.
- **type** `AttendancePunch struct` — AttendancePunch is an immutable IN/OUT event; corrections supersede, never delete (AT-017, AT-019).
- **func** `(p *AttendancePunch) BeforeCreate(tx *gorm.DB) error` — BeforeCreate assigns a UUID v4 when unset.
- **type** `Regularization struct` — Regularization is an employee's timesheet correction request (FR-RG001..FR-RG003).
- **func** `(Regularization) TableName() string { return "attendance_regularizations" }` — TableName maps to attendance_regularizations (migration 000030).
- **func** `(g *Regularization) BeforeCreate(tx *gorm.DB) error` — BeforeCreate assigns a UUID v4 when unset.
- **type** `OutboxEvent struct` — OutboxEvent is a transactional-outbox row written with the business change (FR-EV002, D3-06).
- **func** `(OutboxEvent) TableName() string { return "attendance_events_outbox" }` — TableName maps to attendance_events_outbox (migration 000031).
- **func** `(o *OutboxEvent) BeforeCreate(tx *gorm.DB) error` — BeforeCreate assigns a UUID v4 when unset.

#### `internal/hr/attendance/models/constants.go` (40 lines)
_Package:_ Package models holds Module 3 attendance persistence models (specification.md §8).

#### `internal/hr/attendance/models/shift.go` (48 lines)
- **type** `Shift struct` — Shift is a tenant work-time template; times are minutes from midnight in Timezone (D3-03). FR-SH001. Integer fields carry no GORM default so an explicit 0 (e.g. zero grace) is stored as 0.
- **type** `ShiftAssignment struct` — ShiftAssignment binds an employee to a shift for an inclusive date range (AT-011). FR-SA001.
- **func** `(a *ShiftAssignment) BeforeCreate(tx *gorm.DB) error` — BeforeCreate assigns a UUID v4 when unset.

#### `internal/hr/attendance/repositories/interfaces.go` (98 lines)
_Package:_ Package repositories is Module 3 data access: tenant_id on every scoped query, `db`/`tx` passed per call.
- **type** `ShiftRepository interface` — ShiftRepository persists shifts (FR-SH).
- **method** `ShiftRepository.Create(ctx context.Context, tx *gorm.DB, s *models.Shift) error`
- **method** `ShiftRepository.Update(ctx context.Context, tx *gorm.DB, s *models.Shift) error`
- **method** `ShiftRepository.FindByID(ctx context.Context, db *gorm.DB, tenantID, id uuid.UUID) (*models.Shift, error)`
- **method** `ShiftRepository.FindByName(ctx context.Context, db *gorm.DB, tenantID uuid.UUID, name string) (*models.Shift, error)`
- **method** `ShiftRepository.FindByCode(ctx context.Context, db *gorm.DB, tenantID uuid.UUID, code string) (*models.Shift, error)`
- **method** `ShiftRepository.List(ctx context.Context, db *gorm.DB, tenantID uuid.UUID, status string, page dto.Page) ([]models.Shift, int64, error)`
- **type** `AssignmentRepository interface` — AssignmentRepository persists shift assignments (FR-SA).
- **method** `AssignmentRepository.Create(ctx context.Context, tx *gorm.DB, a *models.ShiftAssignment) error`
- **method** `AssignmentRepository.Update(ctx context.Context, tx *gorm.DB, a *models.ShiftAssignment) error`
- **method** `AssignmentRepository.Overlapping(ctx context.Context, db *gorm.DB, tenantID, employeeID uuid.UUID, from time.Time, to *time.Time) ([]models.ShiftAssignment, error)` — Overlapping returns assignments intersecting [from, to]; to nil = open-ended.
- **method** `AssignmentRepository.ActiveFor(ctx context.Context, db *gorm.DB, tenantID, employeeID uuid.UUID, date time.Time) (*models.ShiftAssignment, error)`
- **method** `AssignmentRepository.CountCoveringFrom(ctx context.Context, db *gorm.DB, tenantID, shiftID uuid.UUID, date time.Time) (int64, error)`
- **method** `AssignmentRepository.ListByEmployee(ctx context.Context, db *gorm.DB, tenantID, employeeID uuid.UUID, page dto.Page) ([]models.ShiftAssignment, int64, error)`
- **type** `RecordFilter struct` — RecordFilter narrows admin record lists (A7).
- **type** `StatusCounts struct` — StatusCounts is the per-date aggregate behind A9 (AT-020).
- **type** `RecordRepository interface` — RecordRepository persists attendance records (FR-AR).
- **method** `RecordRepository.LockEmployee(ctx context.Context, tx *gorm.DB, tenantID, employeeID uuid.UUID) error` — LockEmployee serializes writes for one employee until the tx ends (AT-022).
- **method** `RecordRepository.FindOrCreate(ctx context.Context, tx *gorm.DB, seed *models.AttendanceRecord) (*models.AttendanceRecord, error)` — FindOrCreate returns the record for (employee, date), inserting `seed` if absent.
- **method** `RecordRepository.Update(ctx context.Context, tx *gorm.DB, r *models.AttendanceRecord) error`
- **method** `RecordRepository.Delete(ctx context.Context, tx *gorm.DB, tenantID, id uuid.UUID) error` — Delete removes a leave-only record (D3-17); records with punches are never deleted.
- **method** `RecordRepository.FindByID(ctx context.Context, db *gorm.DB, tenantID, id uuid.UUID) (*models.AttendanceRecord, error)`
- **method** `RecordRepository.FindByDate(ctx context.Context, db *gorm.DB, tenantID, employeeID uuid.UUID, date time.Time) (*models.AttendanceRecord, error)`
- **method** `RecordRepository.ListForEmployee(ctx context.Context, db *gorm.DB, tenantID, employeeID uuid.UUID, from, to time.Time) ([]models.AttendanceRecord, error)`
- **method** `RecordRepository.List(ctx context.Context, db *gorm.DB, tenantID uuid.UUID, f RecordFilter, page dto.Page) ([]models.AttendanceRecord, int64, error)`
- **method** `RecordRepository.CountByStatus(ctx context.Context, db *gorm.DB, tenantID uuid.UUID, date time.Time) (StatusCounts, error)`
- **type** `PunchRepository interface` — PunchRepository persists append-only punches (AT-019).
- **method** `PunchRepository.Create(ctx context.Context, tx *gorm.DB, p *models.AttendancePunch) error`
- **method** `PunchRepository.LastForEmployee(ctx context.Context, db *gorm.DB, tenantID, employeeID uuid.UUID) (*models.AttendancePunch, error)` — LastForEmployee returns the newest non-superseded punch.
- **method** `PunchRepository.ActiveForRecord(ctx context.Context, db *gorm.DB, tenantID, recordID uuid.UUID) ([]models.AttendancePunch, error)`
- **method** `PunchRepository.AllForRecord(ctx context.Context, db *gorm.DB, tenantID, recordID uuid.UUID) ([]models.AttendancePunch, error)`
- **method** `PunchRepository.SupersedeForRecord(ctx context.Context, tx *gorm.DB, tenantID, recordID uuid.UUID) error`
- **type** `RegularizationFilter struct` — RegularizationFilter narrows regularization lists (A5/A10).
- **type** `RegularizationRepository interface` — RegularizationRepository persists correction requests (FR-RG).
- **method** `RegularizationRepository.Create(ctx context.Context, tx *gorm.DB, g *models.Regularization) error`
- **method** `RegularizationRepository.Update(ctx context.Context, tx *gorm.DB, g *models.Regularization) error`
- **method** `RegularizationRepository.FindByID(ctx context.Context, db *gorm.DB, tenantID, id uuid.UUID) (*models.Regularization, error)`
- **method** `RegularizationRepository.FindPending(ctx context.Context, db *gorm.DB, tenantID, employeeID uuid.UUID, date time.Time) (*models.Regularization, error)`
- **method** `RegularizationRepository.List(ctx context.Context, db *gorm.DB, tenantID uuid.UUID, f RegularizationFilter, page dto.Page) ([]models.Regularization, int64, error)`
- **type** `OutboxRepository interface` — OutboxRepository persists outbox rows. Relay methods are cross-tenant by design (system worker).
- **method** `OutboxRepository.Create(ctx context.Context, tx *gorm.DB, e *models.OutboxEvent) error`
- **method** `OutboxRepository.MarkPublished(ctx context.Context, db *gorm.DB, id uuid.UUID, at time.Time) error`
- **method** `OutboxRepository.RecordFailure(ctx context.Context, db *gorm.DB, id uuid.UUID, reason string) error`
- **method** `OutboxRepository.FetchUnpublished(ctx context.Context, db *gorm.DB, limit, maxAttempts int) ([]models.OutboxEvent, error)`
- **func** `day(t time.Time) string { return t.Format("2006-01-02") }` — day renders a calendar date for `?::date` comparisons (session-timezone independent).

#### `internal/hr/attendance/repositories/punch_repository.go` (140 lines)
- **type** `punchRepository struct{}`
- **func** `NewPunchRepository() PunchRepository { return &punchRepository{} }` — NewPunchRepository returns a PunchRepository.
- **func** `(r *punchRepository) Create(ctx context.Context, tx *gorm.DB, p *models.AttendancePunch) error`
- **func** `(r *punchRepository) LastForEmployee(ctx context.Context, db *gorm.DB, tenantID, employeeID uuid.UUID) (*models.AttendancePunch, error)`
- **func** `(r *punchRepository) ActiveForRecord(ctx context.Context, db *gorm.DB, tenantID, recordID uuid.UUID) ([]models.AttendancePunch, error)`
- **func** `(r *punchRepository) AllForRecord(ctx context.Context, db *gorm.DB, tenantID, recordID uuid.UUID) ([]models.AttendancePunch, error)`
- **func** `(r *punchRepository) SupersedeForRecord(ctx context.Context, tx *gorm.DB, tenantID, recordID uuid.UUID) error` — SupersedeForRecord flags every punch of a record superseded (AT-017); nothing is deleted.
- **type** `regularizationRepository struct{}`
- **func** `NewRegularizationRepository() RegularizationRepository { return &regularizationRepository{} }` — NewRegularizationRepository returns a RegularizationRepository.
- **func** `(r *regularizationRepository) Create(ctx context.Context, tx *gorm.DB, g *models.Regularization) error`
- **func** `(r *regularizationRepository) Update(ctx context.Context, tx *gorm.DB, g *models.Regularization) error`
- **func** `(r *regularizationRepository) first(ctx context.Context, db *gorm.DB, query string, args ...interface{}) (*models.Regularization, error)`
- **func** `(r *regularizationRepository) FindByID(ctx context.Context, db *gorm.DB, tenantID, id uuid.UUID) (*models.Regularization, error)`
- **func** `(r *regularizationRepository) FindPending(ctx context.Context, db *gorm.DB, tenantID, employeeID uuid.UUID, date time.Time) (*models.Regularization, error)`
- **func** `(r *regularizationRepository) List(ctx context.Context, db *gorm.DB, tenantID uuid.UUID, f RegularizationFilter, page dto.Page) ([]models.Regularization, int64, error)`
- **type** `outboxRepository struct{}`
- **func** `NewOutboxRepository() OutboxRepository { return &outboxRepository{} }` — NewOutboxRepository returns an OutboxRepository.
- **func** `(r *outboxRepository) Create(ctx context.Context, tx *gorm.DB, e *models.OutboxEvent) error`
- **func** `(r *outboxRepository) MarkPublished(ctx context.Context, db *gorm.DB, id uuid.UUID, at time.Time) error`
- **func** `(r *outboxRepository) RecordFailure(ctx context.Context, db *gorm.DB, id uuid.UUID, reason string) error`
- **func** `(r *outboxRepository) FetchUnpublished(ctx context.Context, db *gorm.DB, limit, maxAttempts int) ([]models.OutboxEvent, error)`

#### `internal/hr/attendance/repositories/record_repository.go` (118 lines)
- **type** `recordRepository struct{}`
- **func** `NewRecordRepository() RecordRepository { return &recordRepository{} }` — NewRecordRepository returns a RecordRepository.
- **func** `(r *recordRepository) LockEmployee(ctx context.Context, tx *gorm.DB, tenantID, employeeID uuid.UUID) error` — LockEmployee takes a transaction-scoped advisory lock keyed on tenant+employee (AT-022, D3-09).
- **func** `(r *recordRepository) FindOrCreate(ctx context.Context, tx *gorm.DB, seed *models.AttendanceRecord) (*models.AttendanceRecord, error)` — FindOrCreate inserts seed unless (tenant, employee, date) exists, then loads the stored row.
- **func** `(r *recordRepository) Update(ctx context.Context, tx *gorm.DB, rec *models.AttendanceRecord) error`
- **func** `(r *recordRepository) Delete(ctx context.Context, tx *gorm.DB, tenantID, id uuid.UUID) error`
- **func** `(r *recordRepository) first(ctx context.Context, db *gorm.DB, query string, args ...interface{}) (*models.AttendanceRecord, error)`
- **func** `(r *recordRepository) FindByID(ctx context.Context, db *gorm.DB, tenantID, id uuid.UUID) (*models.AttendanceRecord, error)`
- **func** `(r *recordRepository) FindByDate(ctx context.Context, db *gorm.DB, tenantID, employeeID uuid.UUID, date time.Time) (*models.AttendanceRecord, error)`
- **func** `(r *recordRepository) ListForEmployee(ctx context.Context, db *gorm.DB, tenantID, employeeID uuid.UUID, from, to time.Time) ([]models.AttendanceRecord, error)`
- **func** `(r *recordRepository) List(ctx context.Context, db *gorm.DB, tenantID uuid.UUID, f RecordFilter, page dto.Page) ([]models.AttendanceRecord, int64, error)`
- **func** `(r *recordRepository) CountByStatus(ctx context.Context, db *gorm.DB, tenantID uuid.UUID, date time.Time) (StatusCounts, error)` — CountByStatus aggregates one tenant-date in a single query (NFR-P003).

#### `internal/hr/attendance/repositories/shift_repository.go` (128 lines)
- **type** `shiftRepository struct{}`
- **func** `NewShiftRepository() ShiftRepository { return &shiftRepository{} }` — NewShiftRepository returns a ShiftRepository.
- **func** `(r *shiftRepository) Create(ctx context.Context, tx *gorm.DB, s *models.Shift) error`
- **func** `(r *shiftRepository) Update(ctx context.Context, tx *gorm.DB, s *models.Shift) error`
- **func** `(r *shiftRepository) first(ctx context.Context, db *gorm.DB, query string, args ...interface{}) (*models.Shift, error)`
- **func** `(r *shiftRepository) FindByID(ctx context.Context, db *gorm.DB, tenantID, id uuid.UUID) (*models.Shift, error)`
- **func** `(r *shiftRepository) FindByName(ctx context.Context, db *gorm.DB, tenantID uuid.UUID, name string) (*models.Shift, error)`
- **func** `(r *shiftRepository) FindByCode(ctx context.Context, db *gorm.DB, tenantID uuid.UUID, code string) (*models.Shift, error)`
- **func** `(r *shiftRepository) List(ctx context.Context, db *gorm.DB, tenantID uuid.UUID, status string, page dto.Page) ([]models.Shift, int64, error)`
- **type** `assignmentRepository struct{}`
- **func** `NewAssignmentRepository() AssignmentRepository { return &assignmentRepository{} }` — NewAssignmentRepository returns an AssignmentRepository.
- **func** `(r *assignmentRepository) Create(ctx context.Context, tx *gorm.DB, a *models.ShiftAssignment) error`
- **func** `(r *assignmentRepository) Update(ctx context.Context, tx *gorm.DB, a *models.ShiftAssignment) error`
- **func** `(r *assignmentRepository) Overlapping(ctx context.Context, db *gorm.DB, tenantID, employeeID uuid.UUID, from time.Time, to *time.Time) ([]models.ShiftAssignment, error)`
- **func** `(r *assignmentRepository) ActiveFor(ctx context.Context, db *gorm.DB, tenantID, employeeID uuid.UUID, date time.Time) (*models.ShiftAssignment, error)`
- **func** `(r *assignmentRepository) CountCoveringFrom(ctx context.Context, db *gorm.DB, tenantID, shiftID uuid.UUID, date time.Time) (int64, error)`
- **func** `(r *assignmentRepository) ListByEmployee(ctx context.Context, db *gorm.DB, tenantID, employeeID uuid.UUID, page dto.Page) ([]models.ShiftAssignment, int64, error)`

#### `internal/hr/attendance/routes/routes.go` (60 lines)
_Package:_ Package routes mounts Module 3 endpoints (specification.md §5) behind Module 0 middleware.
- **type** `Middleware struct` — Middleware is the Module 0 / shared middleware Module 3 depends on (connections C4).
- **type** `Controllers struct` — Controllers groups Module 3 handlers.
- **func** `RegisterRoutes(rg *gin.RouterGroup, mw Middleware, ctl Controllers)` — RegisterRoutes mounts all 19 Module 3 routes; self endpoints need only authentication (NFR-SEC002).

#### `internal/hr/attendance/services/assignment_service.go` (125 lines)
- **type** `AssignmentService interface` — AssignmentService binds employees to shifts (FR-SA001..FR-SA003).
- **method** `AssignmentService.Assign(ctx context.Context, a Actor, shiftID uuid.UUID, req dto.AssignShiftRequest) (*dto.AssignmentResponse, error)`
- **method** `AssignmentService.List(ctx context.Context, tenantID uuid.UUID, employeeID string, page dto.Page) ([]dto.AssignmentResponse, dto.PageMeta, error)`
- **type** `assignmentService struct{ base }`
- **func** `NewAssignmentService(d Deps) AssignmentService { return &assignmentService{base{d}} }` — NewAssignmentService builds an AssignmentService.
- **func** `parseRange(req dto.AssignShiftRequest) (time.Time, *time.Time, error)`
- **func** `(s *assignmentService) Assign(ctx context.Context, a Actor, shiftID uuid.UUID, req dto.AssignShiftRequest) (*dto.AssignmentResponse, error)` — Assign creates an assignment; an earlier open-ended one is closed the day before (AT-011).
- **func** `(s *assignmentService) closeOrReject(ctx context.Context, tx *gorm.DB, next *models.ShiftAssignment) error` — closeOrReject auto-closes one earlier open-ended assignment; any other overlap is a conflict.
- **func** `(s *assignmentService) List(ctx context.Context, tenantID uuid.UUID, employeeID string, page dto.Page) ([]dto.AssignmentResponse, dto.PageMeta, error)`

#### `internal/hr/attendance/services/errors.go` (36 lines)
- **func** `appErr(status int, code, msg string) *sharedErrors.AppError`
- **func** `invalid(field, msg string) *sharedErrors.AppError` — invalid builds a 400 VALIDATION_ERROR naming the offending field.

#### `internal/hr/attendance/services/helpers.go` (153 lines)
- **type** `base struct{ Deps }` — base carries Deps plus helpers shared by all services.
- **func** `utcDay(t time.Time) time.Time` — utcDay truncates an instant to its UTC calendar date.
- **func** `(b base) employeeFor(ctx context.Context, a Actor, requireWorking bool) (*employeeDTO.EmployeeResponse, error)` — employeeFor resolves the caller's employee profile (AT-002); requireWorking enforces status.
- **func** `specFor(s *models.Shift) *calc.ShiftSpec` — specFor converts a stored shift to calc input; nil shift → nil spec (AT-008 defaults).
- **func** `(b base) shiftOn(ctx context.Context, db *gorm.DB, tenantID, employeeID uuid.UUID, date time.Time) (*models.Shift, error)` — shiftOn returns the shift assigned to an employee on a date, or nil.
- **func** `(b base) shiftByID(ctx context.Context, db *gorm.DB, tenantID uuid.UUID, id *uuid.UUID) (*models.Shift, error)` — shiftByID loads a record's shift; nil id → nil shift.
- **func** `(b base) resolveDay(ctx context.Context, db *gorm.DB, tenantID, employeeID uuid.UUID, at time.Time) (time.Time, *models.Shift, error)` — resolveDay picks the attendance date and shift for an instant (AT-005): probe the UTC day, attribute with that shift's zone/night rule, and re-resolve if attribution moved the date.
- **func** `(b base) recompute(ctx context.Context, tx *gorm.DB, rec *models.AttendanceRecord, shift *models.Shift, now time.Time) error` — recompute rebuilds a record's totals from its active punches (AT-006..AT-010).
- **func** `(b base) writeOutbox(ctx context.Context, tx *gorm.DB, a Actor, routingKey string, payload interface{}) (*models.OutboxEvent, error)` — writeOutbox stores the event inside the business transaction (FR-EV002, D3-06).
- **type** `auditEntry struct` — auditEntry is one audit row: action, resource type, resource id, metadata (positional order).
- **func** `(b base) afterCommit(ctx context.Context, a Actor, e auditEntry, row *models.OutboxEvent)` — afterCommit audits and publishes best-effort; failures never surface (D3-06, D3-07).
- **func** `parseUUIDField(field, raw string) (uuid.UUID, error)`

#### `internal/hr/attendance/services/leave_sync.go` (81 lines)
- **type** `LeaveSync interface` — LeaveSync is Module 3's contract C10 for Module 4: it marks or clears `on_leave` day records on the caller's transaction, under the Module 3 per-employee lock (AT-022, D3-17).
- **method** `LeaveSync.MarkLeave(ctx context.Context, tx *gorm.DB, tenantID, employeeID uuid.UUID, dates []time.Time) error`
- **method** `LeaveSync.ClearLeave(ctx context.Context, tx *gorm.DB, tenantID, employeeID uuid.UUID, dates []time.Time) error`
- **type** `leaveSync struct{ base }`
- **func** `NewLeaveSync(d Deps) LeaveSync { return &leaveSync{base{d}} }` — NewLeaveSync builds the LeaveSync port.
- **func** `(s *leaveSync) MarkLeave(ctx context.Context, tx *gorm.DB, tenantID, employeeID uuid.UUID, dates []time.Time) error` — MarkLeave sets each date to on_leave, creating leave-sourced records; punched days keep their status (D4-09).
- **func** `(s *leaveSync) markOne(ctx context.Context, tx *gorm.DB, seed *models.AttendanceRecord) error`
- **func** `(s *leaveSync) ClearLeave(ctx context.Context, tx *gorm.DB, tenantID, employeeID uuid.UUID, dates []time.Time) error` — ClearLeave removes leave-only records and recomputes any other record still marked on_leave.
- **func** `(s *leaveSync) clearOne(ctx context.Context, tx *gorm.DB, tenantID, employeeID uuid.UUID, d time.Time) error`

#### `internal/hr/attendance/services/mappers.go` (64 lines)
- **func** `uuidStr(id *uuid.UUID) *string`
- **func** `mapShift(s *models.Shift) *dto.ShiftResponse`
- **func** `mapAssignment(a *models.ShiftAssignment) dto.AssignmentResponse`
- **func** `mapRecord(r *models.AttendanceRecord) dto.RecordResponse`
- **func** `mapPunch(p *models.AttendancePunch) dto.PunchResponse` — mapPunch never copies IPAddress (security.md AS-T7).
- **func** `mapRegularization(g *models.Regularization) dto.RegularizationResponse`

#### `internal/hr/attendance/services/outbox_relay.go` (59 lines)
- **type** `OutboxRelay struct` — OutboxRelay republishes outbox rows the request path could not publish (D3-06).
- **func** `NewOutboxRelay(d Deps) *OutboxRelay { return &OutboxRelay{base: base{d}, interval: relayInterval} }` — NewOutboxRelay builds a relay over the module dependencies.
- **func** `(r *OutboxRelay) RelayOnce(ctx context.Context) (int, error)` — RelayOnce publishes one batch and returns how many rows were published.
- **func** `(r *OutboxRelay) Run(ctx context.Context)` — Run ticks RelayOnce until ctx is cancelled (started by cmd/main.go, stopped on shutdown).

#### `internal/hr/attendance/services/ports.go` (75 lines)
_Package:_ Package services holds Module 3 business rules (specification.md AT-001..AT-022).
- **type** `EmployeeDirectory interface` — EmployeeDirectory is Module 3's view of Module 2 (connections C2).
- **method** `EmployeeDirectory.GetByUserID(ctx context.Context, tenantID, userID uuid.UUID) (*employeeDTO.EmployeeResponse, error)`
- **method** `EmployeeDirectory.GetByID(ctx context.Context, tenantID, id uuid.UUID) (*employeeDTO.EmployeeResponse, error)`
- **method** `EmployeeDirectory.CountWorking(ctx context.Context, tenantID uuid.UUID) (int64, error)` — CountWorking counts employees whose status allows work (AT-002 statuses).
- **type** `AuditLogger interface` — AuditLogger is Module 3's view of the Module 0 AuditService (connections C3).
- **method** `AuditLogger.Log(ctx context.Context, tx *gorm.DB, tenantID, userID string, action, resource, resourceID string, metadata interface{}, ipAddress, userAgent string) error`
- **type** `TxRunner interface` — TxRunner opens database transactions; services never touch *gorm.DB transactions directly (D3-05).
- **method** `TxRunner.InTx(ctx context.Context, fn func(tx *gorm.DB) error) error`
- **method** `TxRunner.DB() *gorm.DB`
- **type** `Clock func() time.Time` — Clock returns the current instant; injected so goldens run on fixed time (AT-001).
- **type** `Actor struct` — Actor is the authenticated caller, taken from Module 0 middleware context only (NFR-SEC002).
- **type** `Repos struct` — Repos bundles the module repositories.
- **type** `Deps struct` — Deps is everything a Module 3 service needs; built once in module.go.
- **type** `gormRunner struct{ db *gorm.DB }`
- **func** `NewTxRunner(db *gorm.DB) TxRunner { return gormRunner{db: db} }` — NewTxRunner wraps a GORM handle as a TxRunner.
- **func** `(g gormRunner) InTx(ctx context.Context, fn func(tx *gorm.DB) error) error`
- **func** `(g gormRunner) DB() *gorm.DB { return g.db }`

#### `internal/hr/attendance/services/punch_service.go` (150 lines)
- **type** `PunchService interface` — PunchService records self-service IN/OUT punches (FR-PU001, AT-001..AT-006, AT-022).
- **method** `PunchService.Punch(ctx context.Context, a Actor, req dto.PunchRequest) (*dto.PunchResult, error)`
- **type** `punchService struct{ base }`
- **func** `NewPunchService(d Deps) PunchService { return &punchService{base{d}} }` — NewPunchService builds a PunchService.
- **type** `punchTx struct` — punchTx carries one punch's state through the transaction.
- **func** `(s *punchService) Punch(ctx context.Context, a Actor, req dto.PunchRequest) (*dto.PunchResult, error)`
- **func** `(s *punchService) targetRecord(ctx context.Context, tx *gorm.DB, a Actor, st *punchTx) error` — targetRecord applies AT-003..AT-006 and loads (or creates) the record the punch belongs to.
- **func** `sourceOf(req dto.PunchRequest) string`
- **func** `newPunch(a Actor, st *punchTx) *models.AttendancePunch`
- **func** `routingFor(punchType string) string`
- **func** `punchPayload(p *models.AttendancePunch, r *models.AttendanceRecord) events.PunchPayload` — punchPayload whitelists event fields (FR-EV003: no IP/device/coordinates).

#### `internal/hr/attendance/services/query_service.go` (177 lines)
- **type** `RecordQuery struct` — RecordQuery holds raw admin filters (A7); validated by the service.
- **type** `QueryService interface` — QueryService serves attendance reads (FR-AR003..FR-AR005).
- **method** `QueryService.Today(ctx context.Context, a Actor) (*dto.TodayResponse, error)`
- **method** `QueryService.Mine(ctx context.Context, a Actor, from, to string) ([]dto.RecordResponse, error)`
- **method** `QueryService.List(ctx context.Context, tenantID uuid.UUID, q RecordQuery, page dto.Page) ([]dto.RecordResponse, dto.PageMeta, error)`
- **method** `QueryService.Detail(ctx context.Context, tenantID, id uuid.UUID) (*dto.RecordDetailResponse, error)`
- **method** `QueryService.Summary(ctx context.Context, tenantID uuid.UUID, date string) (*dto.SummaryResponse, error)`
- **type** `queryService struct{ base }`
- **func** `NewQueryService(d Deps) QueryService { return &queryService{base{d}} }` — NewQueryService builds a QueryService.
- **func** `(s *queryService) Today(ctx context.Context, a Actor) (*dto.TodayResponse, error)` — Today shows the open session's record if any, else today's (AT-005) record and shift.
- **func** `(s *queryService) Mine(ctx context.Context, a Actor, from, to string) ([]dto.RecordResponse, error)`
- **func** `(s *queryService) List(ctx context.Context, tenantID uuid.UUID, q RecordQuery, page dto.Page) ([]dto.RecordResponse, dto.PageMeta, error)`
- **func** `(s *queryService) Detail(ctx context.Context, tenantID, id uuid.UUID) (*dto.RecordDetailResponse, error)`
- **func** `(s *queryService) Summary(ctx context.Context, tenantID uuid.UUID, date string) (*dto.SummaryResponse, error)` — Summary computes AT-020 from one aggregate query plus the Module 2 workforce count.

#### `internal/hr/attendance/services/regularization_review.go` (123 lines)
- **func** `(s *regularizationService) Approve(ctx context.Context, a Actor, id uuid.UUID, req dto.ReviewRequest) (*dto.RegularizationResponse, error)` — Approve applies the requested window to the record in one transaction (AT-016, AT-017).
- **func** `(s *regularizationService) applyToRecord(ctx context.Context, tx *gorm.DB, g *models.Regularization) error` — applyToRecord supersedes existing punches and inserts the requested in/out (AT-017, AT-019).
- **func** `(s *regularizationService) Reject(ctx context.Context, a Actor, id uuid.UUID, req dto.ReviewRequest) (*dto.RegularizationResponse, error)` — Reject closes a pending request with a mandatory comment (AT-016).
- **func** `(s *regularizationService) markReviewed(g *models.Regularization, a Actor, status string, comment *string)`
- **func** `regPayload(g *models.Regularization) events.RegularizationPayload`

#### `internal/hr/attendance/services/regularization_service.go` (201 lines)
- **type** `RegularizationService interface` — RegularizationService handles timesheet corrections (FR-RG001..FR-RG003, AT-014..AT-018).
- **method** `RegularizationService.Create(ctx context.Context, a Actor, req dto.CreateRegularizationRequest) (*dto.RegularizationResponse, error)`
- **method** `RegularizationService.ListMine(ctx context.Context, a Actor, status string, page dto.Page) ([]dto.RegularizationResponse, dto.PageMeta, error)`
- **method** `RegularizationService.List(ctx context.Context, tenantID uuid.UUID, status, employeeID string, page dto.Page) ([]dto.RegularizationResponse, dto.PageMeta, error)`
- **method** `RegularizationService.Cancel(ctx context.Context, a Actor, id uuid.UUID) (*dto.RegularizationResponse, error)`
- **method** `RegularizationService.Approve(ctx context.Context, a Actor, id uuid.UUID, req dto.ReviewRequest) (*dto.RegularizationResponse, error)`
- **method** `RegularizationService.Reject(ctx context.Context, a Actor, id uuid.UUID, req dto.ReviewRequest) (*dto.RegularizationResponse, error)`
- **type** `regularizationService struct{ base }`
- **func** `NewRegularizationService(d Deps) RegularizationService { return &regularizationService{base{d}} }` — NewRegularizationService builds a RegularizationService.
- **func** `(s *regularizationService) Create(ctx context.Context, a Actor, req dto.CreateRegularizationRequest) (*dto.RegularizationResponse, error)`
- **func** `(s *regularizationService) checkWindow(date time.Time, req dto.CreateRegularizationRequest, spec *calc.ShiftSpec) error` — checkWindow enforces AT-014 in the shift's time zone.
- **func** `(s *regularizationService) list(ctx context.Context, tenantID uuid.UUID, f repositories.RegularizationFilter, page dto.Page) ([]dto.RegularizationResponse, dto.PageMeta, error)`
- **func** `(s *regularizationService) ListMine(ctx context.Context, a Actor, status string, page dto.Page) ([]dto.RegularizationResponse, dto.PageMeta, error)`
- **func** `(s *regularizationService) List(ctx context.Context, tenantID uuid.UUID, status, employeeID string, page dto.Page) ([]dto.RegularizationResponse, dto.PageMeta, error)`
- **func** `(s *regularizationService) loadPending(ctx context.Context, tenantID, id uuid.UUID) (*models.Regularization, error)`
- **func** `(s *regularizationService) Cancel(ctx context.Context, a Actor, id uuid.UUID) (*dto.RegularizationResponse, error)` — Cancel lets the requester withdraw a pending request; others get 404 (AT-018).
- **func** `(s *regularizationService) guardSelfReview(ctx context.Context, a Actor, g *models.Regularization) error` — guardSelfReview blocks reviewers acting on their own request (AT-016).

#### `internal/hr/attendance/services/shift_service.go` (216 lines)
- **type** `ShiftService interface` — ShiftService manages shift templates (FR-SH001..FR-SH006).
- **method** `ShiftService.Create(ctx context.Context, a Actor, req dto.CreateShiftRequest) (*dto.ShiftResponse, error)`
- **method** `ShiftService.Get(ctx context.Context, tenantID, id uuid.UUID) (*dto.ShiftResponse, error)`
- **method** `ShiftService.List(ctx context.Context, tenantID uuid.UUID, status string, page dto.Page) ([]dto.ShiftResponse, dto.PageMeta, error)`
- **method** `ShiftService.Update(ctx context.Context, a Actor, id uuid.UUID, req dto.UpdateShiftRequest) (*dto.ShiftResponse, error)`
- **method** `ShiftService.Deactivate(ctx context.Context, a Actor, id uuid.UUID) (*dto.ShiftResponse, error)`
- **type** `shiftService struct{ base }`
- **func** `NewShiftService(d Deps) ShiftService { return &shiftService{base{d}} }` — NewShiftService builds a ShiftService.
- **func** `(s *shiftService) Create(ctx context.Context, a Actor, req dto.CreateShiftRequest) (*dto.ShiftResponse, error)`
- **func** `applyTimes(shift *models.Shift, start, end *string) error` — applyTimes parses HH:MM pointers onto the shift and derives is_night_shift (FR-SH003, AT-013).
- **func** `(s *shiftService) validate(ctx context.Context, shift *models.Shift) error` — validate checks code, timezone, thresholds and per-tenant uniqueness (FR-SH002, AT-013).
- **func** `(s *shiftService) load(ctx context.Context, tenantID, id uuid.UUID) (*models.Shift, error)`
- **func** `(s *shiftService) Get(ctx context.Context, tenantID, id uuid.UUID) (*dto.ShiftResponse, error)`
- **func** `(s *shiftService) List(ctx context.Context, tenantID uuid.UUID, status string, page dto.Page) ([]dto.ShiftResponse, dto.PageMeta, error)`
- **func** `(s *shiftService) Update(ctx context.Context, a Actor, id uuid.UUID, req dto.UpdateShiftRequest) (*dto.ShiftResponse, error)` — Update applies PATCH fields; code is immutable (FR-SH005); existing records are untouched (AT-021).
- **func** `(s *shiftService) Deactivate(ctx context.Context, a Actor, id uuid.UUID) (*dto.ShiftResponse, error)` — Deactivate is blocked while assignments cover today (shift zone) or later (AT-012).

#### `internal/hr/attendance/validators/validators.go` (86 lines)
_Package:_ Package validators holds Module 3 field rules that struct tags cannot express (AT-013, spec §5).
- **func** `ValidateShiftCode(code string) error` — ValidateShiftCode checks the optional shift code format (FR-SH001).
- **func** `LoadTimezone(name string) (*time.Location, error)` — LoadTimezone resolves an IANA zone; empty means UTC. "Local" is rejected (server-dependent).
- **func** `ParseDate(s string) (time.Time, error)` — ParseDate parses a YYYY-MM-DD calendar date as UTC midnight.
- **func** `DateRange(fromStr, toStr string, today time.Time, maxDays int) (time.Time, time.Time, error)` — DateRange parses an inclusive from/to query; defaults to the 7 days ending today; caps at maxDays.
- **func** `ValidateThresholds(full, half *int) error` — ValidateThresholds enforces half ≤ full when both are set (AT-013).

### M1 HR / Leave


#### `internal/hr/leave/catalogue.go` (19 lines)
- **func** `Catalogue() []access.PermissionDef` — Catalogue is HR/Leave's permission catalogue with the scopes predefined roles receive by default (docs/ARCHITECTURE.md §17). The app registers it with M0 at boot.

#### `internal/hr/leave/module.go` (84 lines)
_Package:_ Package leave wires Module 4 (Leave & Absence Management) into the application.
- **type** `Module struct` — Module is the Module 4 DI container.
- **type** `Ports struct` — Ports are the cross-module collaborators Module 4 consumes (connections C2, C3, C8).
- **func** `NewModule(db *gorm.DB, p Ports) *Module` — NewModule wires repositories, services and controllers.
- **func** `(m *Module) RegisterModels() []interface{}` — RegisterModels lists Module 4 models for AutoMigrate (dev path; SQL 000032–000037 in prod).
- **type** `RouteDeps struct` — RouteDeps is the Module 0 infrastructure the routes need (connections C4).
- **func** `(m *Module) RegisterRoutes(rg *gin.RouterGroup, d RouteDeps)` — RegisterRoutes mounts Module 4 under rg using Module 0 tenant/auth/RBAC middleware.
- **func** `(m *Module) UnpaidLeave() services.UnpaidLeave { return m.unpaid }` — UnpaidLeave exposes the read-only loss-of-pay port for Module 5 (D5-03).
- **func** `(m *Module) Relay() *services.OutboxRelay { return m.relay }` — Relay exposes the outbox relay; cmd/main.go runs it until shutdown.

#### `internal/hr/leave/calc/accrual.go` (50 lines)
- **type** `Policy struct` — Policy is the accrual part of a leave type.
- **func** `Accrued(p Policy, joining *time.Time, year int, asOf time.Time) Days` — Accrued is the entitlement earned in leave year `year` as of asOf (FR-BL002): allowance × creditedMonths / 12 in hundredths. Annual credits every month from the joining month (or January) through December up front; monthly credits only through asOf's month. The cumulative formula makes month 12 land exactly on the allowance, so December absorbs the rounding remainder.
- **func** `CarryForward(prevAvailable, limit Days) Days` — CarryForward is the opening balance moved into a new year: previous available clamped to [0, limit] (FR-BL003).

#### `internal/hr/leave/calc/count.go` (64 lines)
- **func** `DateOf(t time.Time) time.Time` — DateOf returns the UTC calendar date of t at midnight (A4-03).
- **func** `IsWeekend(day time.Time) bool` — IsWeekend reports Saturday/Sunday — the fixed v1 weekly off (D4-05).
- **func** `DaysBetween(a, b time.Time) int { return int(b.Sub(a).Hours() / 24) }` — DaysBetween counts whole days from a to b (dates at midnight UTC).
- **type** `CountInput struct` — CountInput describes a leave range; Holidays lists non-optional holiday dates only.
- **type** `Count struct` — Count is the priced range: Total leave days and the full working dates to mark in attendance.
- **func** `CountDays(in CountInput) Count` — CountDays prices a range per LV-006: working days only, sandwich adds non-working days between the first and last working day, a half day is 0.50 and marks no attendance date (D4-09).

#### `internal/hr/leave/calc/days.go` (14 lines)
_Package:_ Package calc is HR/Leave's pure leave arithmetic: day counting, accrual, carry-forward. The Days amount type lives in the HR kernel; the aliases keep this package's API unchanged.
- **type** `Days = kernel.Days` — Days is an amount of leave in hundredths of a day (kernel.Days).
- **func** `FromWhole(n int) Days { return kernel.FromWhole(n) }` — FromWhole converts whole days to Days.
- **func** `ParseDays(s string) (Days, error) { return kernel.ParseDays(s) }` — ParseDays parses a decimal day amount with at most two decimals.

#### `internal/hr/leave/controllers/balance_controller.go` (75 lines)
- **type** `BalanceController struct{ svc services.BalanceService }` — BalanceController serves balances, adjustments and the ledger (B1–B4).
- **func** `NewBalanceController(s services.BalanceService) *BalanceController` — NewBalanceController builds a BalanceController.
- **func** `(ctl *BalanceController) Mine(c *gin.Context)` — Mine handles GET /leave/balances/me (B1); no employee id is accepted (NFR-SEC002).
- **func** `(ctl *BalanceController) ForEmployee(c *gin.Context)` — ForEmployee handles GET /leave/balances?employee_id= (B2).
- **func** `(ctl *BalanceController) Adjust(c *gin.Context)` — Adjust handles POST /leave/balances/adjust (B3).
- **func** `(ctl *BalanceController) Ledger(c *gin.Context)` — Ledger handles GET /leave/ledger?employee_id=&leave_type_id= (B4).

#### `internal/hr/leave/controllers/policy_controller.go` (151 lines)
- **type** `PolicyController struct` — PolicyController serves leave types and holidays (L1–L5, H1–H3).
- **func** `NewPolicyController(t services.TypeService, h services.HolidayService) *PolicyController` — NewPolicyController builds a PolicyController.
- **func** `(ctl *PolicyController) CreateType(c *gin.Context)` — CreateType handles POST /leave/types (L1).
- **func** `(ctl *PolicyController) ListTypes(c *gin.Context)` — ListTypes handles GET /leave/types (L2).
- **func** `(ctl *PolicyController) GetType(c *gin.Context)` — GetType handles GET /leave/types/:id (L3).
- **func** `(ctl *PolicyController) UpdateType(c *gin.Context)` — UpdateType handles PATCH /leave/types/:id (L4).
- **func** `(ctl *PolicyController) DeactivateType(c *gin.Context)` — DeactivateType handles POST /leave/types/:id/deactivate (L5).
- **func** `(ctl *PolicyController) CreateHoliday(c *gin.Context)` — CreateHoliday handles POST /leave/holidays (H1).
- **func** `(ctl *PolicyController) ListHolidays(c *gin.Context)` — ListHolidays handles GET /leave/holidays?year= (H2).
- **func** `(ctl *PolicyController) DeleteHoliday(c *gin.Context)` — DeleteHoliday handles DELETE /leave/holidays/:id (H3).

#### `internal/hr/leave/controllers/request_controller.go` (133 lines)
- **type** `RequestController struct{ svc services.RequestService }` — RequestController serves the leave request lifecycle (R1–R8).
- **func** `NewRequestController(s services.RequestService) *RequestController` — NewRequestController builds a RequestController.
- **func** `(ctl *RequestController) Preview(c *gin.Context)` — Preview handles POST /leave/requests/preview (R1).
- **func** `(ctl *RequestController) Apply(c *gin.Context)` — Apply handles POST /leave/requests (R2); the employee is the caller (NFR-SEC002).
- **func** `(ctl *RequestController) ListMine(c *gin.Context)` — ListMine handles GET /leave/requests/me (R3).
- **func** `(ctl *RequestController) List(c *gin.Context)` — List handles GET /leave/requests (R5).
- **func** `(ctl *RequestController) Get(c *gin.Context)` — Get handles GET /leave/requests/:id (R6).
- **type** `reqAction func(ctx context.Context, a services.Actor, id uuid.UUID, req dto.ReviewRequest) (*dto.LeaveRequestResponse, error)`
- **func** `(ctl *RequestController) act(c *gin.Context, fn reqAction, withBody bool)` — act runs an id-addressed action with an optional review body.
- **func** `(ctl *RequestController) Cancel(c *gin.Context)` — Cancel handles POST /leave/requests/:id/cancel (R4).
- **func** `(ctl *RequestController) Approve(c *gin.Context) { ctl.act(c, ctl.svc.Approve, true) }` — Approve handles POST /leave/requests/:id/approve (R7).
- **func** `(ctl *RequestController) Reject(c *gin.Context) { ctl.act(c, ctl.svc.Reject, true) }` — Reject handles POST /leave/requests/:id/reject (R8).

#### `internal/hr/leave/controllers/respond.go` (106 lines)
_Package:_ Package controllers is Module 4's thin HTTP edge: parse → validate → service → envelope.
- **type** `errorBody struct`
- **func** `ok(c *gin.Context, status int, data interface{})`
- **func** `okList(c *gin.Context, data interface{}, meta dto.PageMeta)`
- **func** `fail(c *gin.Context, err error)` — fail maps AppErrors to their status after shared normalization (unique violation → 409, not found → 404); anything else is an opaque 500 (no internals leaked).
- **func** `bindJSON(c *gin.Context, dst interface{}) bool` — bindJSON decodes and validates a body; failures become VALIDATION_ERROR with field details.
- **func** `snake(s string) string`
- **func** `actorFrom(c *gin.Context) (services.Actor, bool)` — actorFrom builds the caller from Module 0 middleware context only (NFR-SEC002).
- **func** `pathID(c *gin.Context, name string) (uuid.UUID, bool)` — pathID parses a :param UUID; malformed ids are 400.
- **func** `page(c *gin.Context) dto.Page`

#### `internal/hr/leave/dto/requests.go` (62 lines)
_Package:_ Package dto holds Module 4 request/response shapes (specification.md §5); never GORM models.
- **type** `CreateLeaveTypeRequest struct` — CreateLeaveTypeRequest is L1. Day amounts are JSON numbers with ≤ 2 decimals; ranges checked in validators.
- **type** `UpdateLeaveTypeRequest struct` — UpdateLeaveTypeRequest is L4: pointer fields, code immutable; max_consecutive_days 0 clears the limit.
- **type** `CreateHolidayRequest struct` — CreateHolidayRequest is H1.
- **type** `AdjustBalanceRequest struct` — AdjustBalanceRequest is B3 (LV-017); days is signed and non-zero.
- **type** `ApplyLeaveRequest struct` — ApplyLeaveRequest is R1/R2. No employee field exists: the employee is the caller (NFR-SEC002).
- **type** `ReviewRequest struct` — ReviewRequest is R7/R8; reject requires a comment (LV-010, enforced in service).

#### `internal/hr/leave/dto/responses.go` (139 lines)
- **type** `LeaveTypeResponse struct` — LeaveTypeResponse serializes a leave type.
- **type** `HolidayResponse struct` — HolidayResponse serializes a holiday.
- **type** `BalanceResponse struct` — BalanceResponse serializes one balance projection.
- **type** `LedgerEntryResponse struct` — LedgerEntryResponse serializes one ledger row.
- **type** `LeaveRequestResponse struct` — LeaveRequestResponse serializes a leave request.
- **type** `PreviewResponse struct` — PreviewResponse is R1: the price of a request without saving it.
- **type** `PageMeta struct` — PageMeta is the list envelope meta.
- **type** `Page struct` — Page is a clamped page request (G15: bad input never errors).
- **func** `ParsePage(pageStr, perPageStr string) Page` — ParsePage clamps raw query values to valid bounds (spec §5, EC-18).
- **func** `(p Page) Offset() int { return (p.Page - 1) * p.PerPage }` — Offset is the SQL offset for this page.
- **func** `(p Page) Meta(total int64) PageMeta` — Meta builds list metadata for a total count.

#### `internal/hr/leave/events/events.go` (75 lines)
_Package:_ Package events defines Module 4 event types and the shared envelope (FR-EV001).
- **type** `Envelope struct` — Envelope is the platform event envelope (same shape as Module 3).
- **type** `Source struct` — Source identifies who and when an event came from.
- **func** `New(routingKey string, src Source, payload interface{}) Envelope` — New wraps a payload in the envelope.
- **type** `RequestPayload struct` — RequestPayload is published for leave.applied|approved|rejected|cancelled; no reason/comment (FR-EV003).
- **type** `AccruedPayload struct` — AccruedPayload is published for leave.accrued.

#### `internal/hr/leave/models/models.go` (182 lines)
_Package:_ Package models holds Module 4 (Leave) persistence models; migrations 000032–000037 are the SQL twin.
- **type** `LeaveType struct` — LeaveType is a tenant leave policy (FR-LT001). Integer fields carry no GORM default so explicit 0 is stored.
- **func** `(LeaveType) TableName() string { return "leave_types" }` — TableName maps to leave_types (migration 000032).
- **func** `(l *LeaveType) BeforeCreate(tx *gorm.DB) error { return assignID(&l.ID) }` — BeforeCreate assigns a UUID v4 when unset.
- **type** `Holiday struct` — Holiday is one tenant calendar date excluded from day counting unless optional (FR-HD001, D4-05).
- **func** `(Holiday) TableName() string { return "leave_holidays" }` — TableName maps to leave_holidays (migration 000033).
- **func** `(h *Holiday) BeforeCreate(tx *gorm.DB) error { return assignID(&h.ID) }` — BeforeCreate assigns a UUID v4 when unset.
- **type** `Balance struct` — Balance is the per (employee, type, year) projection of the ledger (D4-02, LV-013).
- **func** `(Balance) TableName() string { return "leave_balances" }` — TableName maps to leave_balances (migration 000034).
- **func** `(b *Balance) BeforeCreate(tx *gorm.DB) error { return assignID(&b.ID) }` — BeforeCreate assigns a UUID v4 when unset.
- **func** `(b *Balance) Available() calc.Days` — Available is opening + accrued + adjusted − used − reserved.
- **type** `Request struct` — Request is a leave application; TotalDays is a snapshot at apply time (LV-012).
- **func** `(Request) TableName() string { return "leave_requests" }` — TableName maps to leave_requests (migration 000035).
- **func** `(r *Request) BeforeCreate(tx *gorm.DB) error { return assignID(&r.ID) }` — BeforeCreate assigns a UUID v4 when unset.
- **type** `LedgerEntry struct` — LedgerEntry is one append-only balance movement (LV-013, LV-016).
- **func** `(LedgerEntry) TableName() string { return "leave_ledger" }` — TableName maps to leave_ledger (migration 000036).
- **func** `(e *LedgerEntry) BeforeCreate(tx *gorm.DB) error { return assignID(&e.ID) }` — BeforeCreate assigns a UUID v4 when unset.
- **type** `OutboxEvent struct` — OutboxEvent is a transactional-outbox row written with the business change (FR-EV002).
- **func** `(OutboxEvent) TableName() string { return "leave_events_outbox" }` — TableName maps to leave_events_outbox (migration 000037).
- **func** `(o *OutboxEvent) BeforeCreate(tx *gorm.DB) error { return assignID(&o.ID) }` — BeforeCreate assigns a UUID v4 when unset.
- **func** `assignID(id *uuid.UUID) error`

#### `internal/hr/leave/repositories/balance_repository.go` (65 lines)
- **type** `balanceRepository struct{}`
- **func** `NewBalanceRepository() BalanceRepository { return &balanceRepository{} }` — NewBalanceRepository builds a BalanceRepository.
- **func** `(r *balanceRepository) LockEmployee(ctx context.Context, tx *gorm.DB, tenantID, employeeID uuid.UUID) error` — LockEmployee takes a tx-scoped advisory lock namespaced away from Module 3's attendance lock (LV-014).
- **func** `(r *balanceRepository) FindOrCreate(ctx context.Context, tx *gorm.DB, key BalanceKey) (*models.Balance, bool, error)` — FindOrCreate inserts a zero row unless the key exists; RowsAffected tells whether this call created it.
- **func** `(r *balanceRepository) Find(ctx context.Context, db *gorm.DB, key BalanceKey) (*models.Balance, error)`
- **func** `(r *balanceRepository) Update(ctx context.Context, tx *gorm.DB, b *models.Balance) error`
- **type** `ledgerRepository struct{}`
- **func** `NewLedgerRepository() LedgerRepository { return &ledgerRepository{} }` — NewLedgerRepository builds a LedgerRepository.
- **func** `(r *ledgerRepository) Create(ctx context.Context, tx *gorm.DB, e *models.LedgerEntry) error`
- **func** `(r *ledgerRepository) List(ctx context.Context, db *gorm.DB, tenantID uuid.UUID, f LedgerFilter, page dto.Page) ([]models.LedgerEntry, int64, error)`

#### `internal/hr/leave/repositories/interfaces.go` (113 lines)
_Package:_ Package repositories is Module 4 data access: tenant_id on every scoped query, `db`/`tx` passed per call.
- **type** `TypeRepository interface` — TypeRepository persists leave types (FR-LT).
- **method** `TypeRepository.Create(ctx context.Context, tx *gorm.DB, t *models.LeaveType) error`
- **method** `TypeRepository.Update(ctx context.Context, tx *gorm.DB, t *models.LeaveType) error`
- **method** `TypeRepository.FindByID(ctx context.Context, db *gorm.DB, tenantID, id uuid.UUID) (*models.LeaveType, error)`
- **method** `TypeRepository.FindByName(ctx context.Context, db *gorm.DB, tenantID uuid.UUID, name string) (*models.LeaveType, error)`
- **method** `TypeRepository.FindByCode(ctx context.Context, db *gorm.DB, tenantID uuid.UUID, code string) (*models.LeaveType, error)`
- **method** `TypeRepository.List(ctx context.Context, db *gorm.DB, tenantID uuid.UUID, status string, page dto.Page) ([]models.LeaveType, int64, error)`
- **type** `HolidayRepository interface` — HolidayRepository persists the tenant holiday calendar (FR-HD).
- **method** `HolidayRepository.Create(ctx context.Context, tx *gorm.DB, h *models.Holiday) error`
- **method** `HolidayRepository.Delete(ctx context.Context, tx *gorm.DB, h *models.Holiday) error`
- **method** `HolidayRepository.FindByID(ctx context.Context, db *gorm.DB, tenantID, id uuid.UUID) (*models.Holiday, error)`
- **method** `HolidayRepository.FindByDate(ctx context.Context, db *gorm.DB, tenantID uuid.UUID, date time.Time) (*models.Holiday, error)`
- **method** `HolidayRepository.ListRange(ctx context.Context, db *gorm.DB, tenantID uuid.UUID, from, to time.Time) ([]models.Holiday, error)` — ListRange returns holidays in [from, to], optional ones included, ordered by date.
- **type** `BalanceKey struct` — BalanceKey identifies one balance row.
- **type** `BalanceRepository interface` — BalanceRepository persists balance projections (D4-02).
- **method** `BalanceRepository.LockEmployee(ctx context.Context, tx *gorm.DB, tenantID, employeeID uuid.UUID) error` — LockEmployee serializes leave writes for one employee until the tx ends (LV-014).
- **method** `BalanceRepository.FindOrCreate(ctx context.Context, tx *gorm.DB, key BalanceKey) (b *models.Balance, created bool, err error)` — FindOrCreate returns the row for key, inserting a zero row if absent; created reports the insert.
- **method** `BalanceRepository.Find(ctx context.Context, db *gorm.DB, key BalanceKey) (*models.Balance, error)`
- **method** `BalanceRepository.Update(ctx context.Context, tx *gorm.DB, b *models.Balance) error`
- **type** `RequestFilter struct` — RequestFilter narrows request lists (R3/R5).
- **type** `RequestRepository interface` — RequestRepository persists leave requests (FR-LR).
- **method** `RequestRepository.Create(ctx context.Context, tx *gorm.DB, r *models.Request) error`
- **method** `RequestRepository.Update(ctx context.Context, tx *gorm.DB, r *models.Request) error`
- **method** `RequestRepository.FindByID(ctx context.Context, db *gorm.DB, tenantID, id uuid.UUID) (*models.Request, error)`
- **method** `RequestRepository.Overlapping(ctx context.Context, db *gorm.DB, tenantID, employeeID uuid.UUID, from, to time.Time) ([]models.Request, error)` — Overlapping returns the employee's pending/approved requests intersecting [from, to] (LV-008).
- **method** `RequestRepository.List(ctx context.Context, db *gorm.DB, tenantID uuid.UUID, f RequestFilter, page dto.Page) ([]models.Request, int64, error)`
- **type** `LedgerFilter struct` — LedgerFilter narrows ledger lists (B4).
- **type** `LedgerRepository interface` — LedgerRepository appends ledger rows (LV-013, LV-016) — there is no update or delete.
- **method** `LedgerRepository.Create(ctx context.Context, tx *gorm.DB, e *models.LedgerEntry) error`
- **method** `LedgerRepository.List(ctx context.Context, db *gorm.DB, tenantID uuid.UUID, f LedgerFilter, page dto.Page) ([]models.LedgerEntry, int64, error)`
- **type** `OutboxRepository interface` — OutboxRepository persists outbox rows. Relay methods are cross-tenant by design (system worker).
- **method** `OutboxRepository.Create(ctx context.Context, tx *gorm.DB, e *models.OutboxEvent) error`
- **method** `OutboxRepository.MarkPublished(ctx context.Context, db *gorm.DB, id uuid.UUID, at time.Time) error`
- **method** `OutboxRepository.RecordFailure(ctx context.Context, db *gorm.DB, id uuid.UUID, reason string) error`
- **method** `OutboxRepository.FetchUnpublished(ctx context.Context, db *gorm.DB, limit, maxAttempts int) ([]models.OutboxEvent, error)`
- **func** `day(t time.Time) string { return t.Format("2006-01-02") }` — day renders a calendar date for `?::date` comparisons (session-timezone independent).
- **func** `first[T any](q *gorm.DB) (*T, error)` — first runs a single-row query and maps "not found" to (nil, nil).
- **func** `paged[T any](q *gorm.DB, order string, page dto.Page) ([]T, int64, error)` — paged counts then fetches one page of q.

#### `internal/hr/leave/repositories/request_repository.go` (80 lines)
- **type** `requestRepository struct{}`
- **func** `NewRequestRepository() RequestRepository { return &requestRepository{} }` — NewRequestRepository builds a RequestRepository.
- **func** `(r *requestRepository) Create(ctx context.Context, tx *gorm.DB, req *models.Request) error`
- **func** `(r *requestRepository) Update(ctx context.Context, tx *gorm.DB, req *models.Request) error`
- **func** `(r *requestRepository) FindByID(ctx context.Context, db *gorm.DB, tenantID, id uuid.UUID) (*models.Request, error)`
- **func** `(r *requestRepository) Overlapping(ctx context.Context, db *gorm.DB, tenantID, employeeID uuid.UUID, from, to time.Time) ([]models.Request, error)`
- **func** `(r *requestRepository) List(ctx context.Context, db *gorm.DB, tenantID uuid.UUID, f RequestFilter, page dto.Page) ([]models.Request, int64, error)`
- **type** `outboxRepository struct{}`
- **func** `NewOutboxRepository() OutboxRepository { return &outboxRepository{} }` — NewOutboxRepository builds an OutboxRepository.
- **func** `(r *outboxRepository) Create(ctx context.Context, tx *gorm.DB, e *models.OutboxEvent) error`
- **func** `(r *outboxRepository) MarkPublished(ctx context.Context, db *gorm.DB, id uuid.UUID, at time.Time) error`
- **func** `(r *outboxRepository) RecordFailure(ctx context.Context, db *gorm.DB, id uuid.UUID, reason string) error`
- **func** `(r *outboxRepository) FetchUnpublished(ctx context.Context, db *gorm.DB, limit, maxAttempts int) ([]models.OutboxEvent, error)`

#### `internal/hr/leave/repositories/type_repository.go` (75 lines)
- **type** `typeRepository struct{}`
- **func** `NewTypeRepository() TypeRepository { return &typeRepository{} }` — NewTypeRepository builds a TypeRepository.
- **func** `(r *typeRepository) Create(ctx context.Context, tx *gorm.DB, t *models.LeaveType) error`
- **func** `(r *typeRepository) Update(ctx context.Context, tx *gorm.DB, t *models.LeaveType) error`
- **func** `(r *typeRepository) FindByID(ctx context.Context, db *gorm.DB, tenantID, id uuid.UUID) (*models.LeaveType, error)`
- **func** `(r *typeRepository) FindByName(ctx context.Context, db *gorm.DB, tenantID uuid.UUID, name string) (*models.LeaveType, error)`
- **func** `(r *typeRepository) FindByCode(ctx context.Context, db *gorm.DB, tenantID uuid.UUID, code string) (*models.LeaveType, error)`
- **func** `(r *typeRepository) List(ctx context.Context, db *gorm.DB, tenantID uuid.UUID, status string, page dto.Page) ([]models.LeaveType, int64, error)`
- **type** `holidayRepository struct{}`
- **func** `NewHolidayRepository() HolidayRepository { return &holidayRepository{} }` — NewHolidayRepository builds a HolidayRepository.
- **func** `(r *holidayRepository) Create(ctx context.Context, tx *gorm.DB, h *models.Holiday) error`
- **func** `(r *holidayRepository) Delete(ctx context.Context, tx *gorm.DB, h *models.Holiday) error` — Delete soft-deletes (FR-HD003); existing requests keep their snapshot (LV-012).
- **func** `(r *holidayRepository) FindByID(ctx context.Context, db *gorm.DB, tenantID, id uuid.UUID) (*models.Holiday, error)`
- **func** `(r *holidayRepository) FindByDate(ctx context.Context, db *gorm.DB, tenantID uuid.UUID, date time.Time) (*models.Holiday, error)`
- **func** `(r *holidayRepository) ListRange(ctx context.Context, db *gorm.DB, tenantID uuid.UUID, from, to time.Time) ([]models.Holiday, error)`

#### `internal/hr/leave/routes/routes.go` (59 lines)
_Package:_ Package routes mounts Module 4 endpoints (specification.md §5) behind Module 0 middleware.
- **type** `Middleware struct` — Middleware is the Module 0 middleware Module 4 depends on (connections C4).
- **type** `Controllers struct` — Controllers groups Module 4 handlers.
- **func** `RegisterRoutes(rg *gin.RouterGroup, mw Middleware, ctl Controllers)` — RegisterRoutes mounts all 20 Module 4 operations; self endpoints and type/holiday reads need authentication only.

#### `internal/hr/leave/services/balance_service.go` (137 lines)
- **type** `LedgerQuery struct` — LedgerQuery is B4's filter; employee_id is required.
- **type** `BalanceService interface` — BalanceService serves balances, HR adjustments and the ledger (FR-BL001..BL005).
- **method** `BalanceService.Mine(ctx context.Context, a Actor) ([]dto.BalanceResponse, error)`
- **method** `BalanceService.ForEmployee(ctx context.Context, a Actor, employeeID string) ([]dto.BalanceResponse, error)`
- **method** `BalanceService.Adjust(ctx context.Context, a Actor, req dto.AdjustBalanceRequest) (*dto.BalanceResponse, error)`
- **method** `BalanceService.Ledger(ctx context.Context, tenantID uuid.UUID, q LedgerQuery, page dto.Page) ([]dto.LedgerEntryResponse, dto.PageMeta, error)`
- **type** `balanceService struct{ base }`
- **func** `NewBalanceService(d Deps) BalanceService { return &balanceService{base{d}} }` — NewBalanceService builds a BalanceService.
- **func** `(s *balanceService) Mine(ctx context.Context, a Actor) ([]dto.BalanceResponse, error)`
- **func** `(s *balanceService) ForEmployee(ctx context.Context, a Actor, employeeID string) ([]dto.BalanceResponse, error)`
- **func** `(s *balanceService) balancesFor(ctx context.Context, a Actor, emp *employeeDTO.EmployeeResponse) ([]dto.BalanceResponse, error)` — balancesFor materializes every active type's current-year balance under the employee lock (FR-BL001).
- **func** `(s *balanceService) Adjust(ctx context.Context, a Actor, req dto.AdjustBalanceRequest) (*dto.BalanceResponse, error)` — Adjust applies an HR correction as an `adjustment` ledger row (FR-BL004, LV-017).
- **func** `(s *balanceService) Ledger(ctx context.Context, tenantID uuid.UUID, q LedgerQuery, page dto.Page) ([]dto.LedgerEntryResponse, dto.PageMeta, error)`

#### `internal/hr/leave/services/errors.go` (40 lines)
- **func** `appErr(status int, code, msg string) *sharedErrors.AppError`
- **func** `invalid(field, msg string) *sharedErrors.AppError` — invalid builds a 400 VALIDATION_ERROR naming the offending field.

#### `internal/hr/leave/services/helpers.go` (120 lines)
- **type** `base struct{ Deps }` — base carries Deps plus helpers shared by all services.
- **func** `(b base) today() time.Time { return calc.DateOf(b.Now()) }`
- **func** `(b base) employeeFor(ctx context.Context, a Actor, requireWorking bool) (*employeeDTO.EmployeeResponse, error)` — employeeFor resolves the caller's profile (LV-001); requireWorking enforces status.
- **func** `(b base) employeeByID(ctx context.Context, tenantID uuid.UUID, raw string) (*employeeDTO.EmployeeResponse, error)` — employeeByID resolves a profile by id in the caller's tenant (B2/B3/B4).
- **func** `checkEmployee(emp *employeeDTO.EmployeeResponse, err error, requireWorking bool) (*employeeDTO.EmployeeResponse, error)`
- **func** `joiningOf(emp *employeeDTO.EmployeeResponse) *time.Time` — joiningOf returns the employee's joining date, nil when unknown (A4-02).
- **func** `(b base) typeByID(ctx context.Context, db *gorm.DB, tenantID uuid.UUID, raw string) (*models.LeaveType, error)` — typeByID loads a leave type in the caller's tenant; unknown → 404.
- **func** `(b base) writeOutbox(ctx context.Context, tx *gorm.DB, a Actor, routingKey string, payload interface{}) (*models.OutboxEvent, error)` — writeOutbox stores the event inside the business transaction (FR-EV002).
- **type** `auditEntry struct` — auditEntry is one audit row: action, resource type, resource id, metadata (positional order).
- **func** `(b base) afterCommit(ctx context.Context, a Actor, e auditEntry, rows []*models.OutboxEvent)` — afterCommit audits and publishes best-effort; failures never surface (FR-EV002).
- **func** `parseUUIDField(field, raw string) (uuid.UUID, error)`
- **func** `keyFor(tenantID, employeeID uuid.UUID, typ *models.LeaveType, year int) repositories.BalanceKey` — keyFor builds the balance key for an employee/type/year.

#### `internal/hr/leave/services/holiday_service.go` (87 lines)
- **type** `HolidayService interface` — HolidayService manages the tenant holiday calendar (FR-HD001..HD003).
- **method** `HolidayService.Create(ctx context.Context, a Actor, req dto.CreateHolidayRequest) (*dto.HolidayResponse, error)`
- **method** `HolidayService.List(ctx context.Context, tenantID uuid.UUID, year string) ([]dto.HolidayResponse, error)`
- **method** `HolidayService.Delete(ctx context.Context, a Actor, id uuid.UUID) (*dto.HolidayResponse, error)`
- **type** `holidayService struct{ base }`
- **func** `NewHolidayService(d Deps) HolidayService { return &holidayService{base{d}} }` — NewHolidayService builds a HolidayService.
- **func** `(s *holidayService) Create(ctx context.Context, a Actor, req dto.CreateHolidayRequest) (*dto.HolidayResponse, error)`
- **func** `(s *holidayService) List(ctx context.Context, tenantID uuid.UUID, year string) ([]dto.HolidayResponse, error)` — List returns one year's holidays (default: current year).
- **func** `(s *holidayService) Delete(ctx context.Context, a Actor, id uuid.UUID) (*dto.HolidayResponse, error)` — Delete soft-deletes a holiday; existing requests keep their snapshot (LV-012).

#### `internal/hr/leave/services/ledger.go` (88 lines)
- **type** `movement struct` — movement is one balance change; it always becomes exactly one ledger row (LV-013).
- **func** `(b base) move(ctx context.Context, tx *gorm.DB, bal *models.Balance, m movement) error` — move applies a movement to the projection column for its kind and appends the ledger row (architecture §4).
- **type** `entitlement struct` — entitlement is the employee-side input to balance materialization.
- **func** `(b base) ensure(ctx context.Context, tx *gorm.DB, a Actor, typ *models.LeaveType, e entitlement) (*models.Balance, error)` — ensure materializes the balance lazily (FR-BL001..003, D4-06): carry-forward once on creation, then accrual top-up to the policy target as of today. Caller must hold the employee lock (LV-014).
- **func** `(b base) carryForward(ctx context.Context, tx *gorm.DB, bal *models.Balance, typ *models.LeaveType) error`

#### `internal/hr/leave/services/mappers.go` (53 lines)
- **func** `mapType(t *models.LeaveType) dto.LeaveTypeResponse`
- **func** `mapHoliday(h *models.Holiday) dto.HolidayResponse`
- **func** `mapBalance(b *models.Balance, t *models.LeaveType) dto.BalanceResponse`
- **func** `mapLedger(e *models.LedgerEntry) dto.LedgerEntryResponse`
- **func** `mapRequest(r *models.Request, code string) dto.LeaveRequestResponse`
- **func** `requestPayload(r *models.Request, t *models.LeaveType) events.RequestPayload` — requestPayload whitelists event fields (FR-EV003: no reason/comment).
- **func** `requestMeta(r *models.Request) map[string]string` — requestMeta is the audit metadata for request actions: ids, dates and days only (LS-T6).

#### `internal/hr/leave/services/outbox_relay.go` (59 lines)
- **type** `OutboxRelay struct` — OutboxRelay republishes outbox rows the request path could not publish (FR-EV002).
- **func** `NewOutboxRelay(d Deps) *OutboxRelay { return &OutboxRelay{base: base{d}, interval: relayInterval} }` — NewOutboxRelay builds a relay over the module dependencies.
- **func** `(r *OutboxRelay) RelayOnce(ctx context.Context) (int, error)` — RelayOnce publishes one batch and returns how many rows were published.
- **func** `(r *OutboxRelay) Run(ctx context.Context)` — Run ticks RelayOnce until ctx is cancelled (started by cmd/main.go, stopped on shutdown).

#### `internal/hr/leave/services/ports.go` (80 lines)
_Package:_ Package services holds Module 4 business rules (specification.md LV-001..LV-017).
- **type** `EmployeeDirectory interface` — EmployeeDirectory is Module 4's view of Module 2 (connections C2).
- **method** `EmployeeDirectory.GetByUserID(ctx context.Context, tenantID, userID uuid.UUID) (*employeeDTO.EmployeeResponse, error)`
- **method** `EmployeeDirectory.GetByID(ctx context.Context, tenantID, id uuid.UUID) (*employeeDTO.EmployeeResponse, error)`
- **type** `AuditLogger interface` — AuditLogger is Module 4's view of the Module 0 AuditService (connections C3).
- **method** `AuditLogger.Log(ctx context.Context, tx *gorm.DB, tenantID, userID string, action, resource, resourceID string, metadata interface{}, ipAddress, userAgent string) error`
- **type** `AttendanceSync interface` — AttendanceSync is Module 4's view of Module 3 services.LeaveSync (connections C8, D4-08).
- **method** `AttendanceSync.MarkLeave(ctx context.Context, tx *gorm.DB, tenantID, employeeID uuid.UUID, dates []time.Time) error`
- **method** `AttendanceSync.ClearLeave(ctx context.Context, tx *gorm.DB, tenantID, employeeID uuid.UUID, dates []time.Time) error`
- **type** `TxRunner interface` — TxRunner opens database transactions; controllers never see *gorm.DB.
- **method** `TxRunner.InTx(ctx context.Context, fn func(tx *gorm.DB) error) error`
- **method** `TxRunner.DB() *gorm.DB`
- **type** `Clock func() time.Time` — Clock returns the current instant; injected so goldens run on fixed dates.
- **type** `Actor struct` — Actor is the authenticated caller, from Module 0 middleware context only (NFR-SEC002).
- **type** `Repos struct` — Repos bundles the module repositories.
- **type** `Deps struct` — Deps is everything a Module 4 service needs; built once in module.go.
- **type** `gormRunner struct{ db *gorm.DB }`
- **func** `NewTxRunner(db *gorm.DB) TxRunner { return gormRunner{db: db} }` — NewTxRunner wraps a GORM handle as a TxRunner.
- **func** `(g gormRunner) InTx(ctx context.Context, fn func(tx *gorm.DB) error) error`
- **func** `(g gormRunner) DB() *gorm.DB { return g.db }`

#### `internal/hr/leave/services/request_query.go` (97 lines)
- **func** `(s *requestService) ListMine(ctx context.Context, a Actor, status string, page dto.Page) ([]dto.LeaveRequestResponse, dto.PageMeta, error)` — ListMine lists the caller's own requests (R3); the employee comes from the JWT only (NFR-SEC002).
- **func** `(s *requestService) List(ctx context.Context, tenantID uuid.UUID, q RequestQuery, page dto.Page) ([]dto.LeaveRequestResponse, dto.PageMeta, error)` — List lists tenant requests with filters (R5).
- **func** `(s *requestService) Get(ctx context.Context, tenantID, id uuid.UUID) (*dto.LeaveRequestResponse, error)` — Get returns one request (R6); foreign ids → 404.
- **func** `(s *requestService) list(ctx context.Context, tenantID uuid.UUID, f repositories.RequestFilter, page dto.Page) ([]dto.LeaveRequestResponse, dto.PageMeta, error)`
- **func** `(s *requestService) typeCodes(ctx context.Context, tenantID uuid.UUID) (map[uuid.UUID]string, error)` — typeCodes maps type id → code in one query (no N+1, NFR-P002).
- **func** `requestFilter(q RequestQuery) (repositories.RequestFilter, error)`

#### `internal/hr/leave/services/request_review.go` (186 lines)
- **type** `transition struct` — transition is one locked state change of a request; apply mutates balance/attendance and the request.
- **func** `(s *requestService) run(ctx context.Context, a Actor, tr transition) (*dto.LeaveRequestResponse, error)` — run loads, locks and re-reads the request, applies the transition and writes the outbox row (LV-014).
- **func** `(s *requestService) lockedRequest(ctx context.Context, tx *gorm.DB, a Actor, tr transition) (*models.Request, error)`
- **func** `orNotFound(err error) error`
- **func** `(s *requestService) Approve(ctx context.Context, a Actor, id uuid.UUID, req dto.ReviewRequest) (*dto.LeaveRequestResponse, error)` — Approve moves reserved days to used and marks attendance (FR-LR003, LV-015).
- **func** `(s *requestService) Reject(ctx context.Context, a Actor, id uuid.UUID, req dto.ReviewRequest) (*dto.LeaveRequestResponse, error)` — Reject releases the reservation; a comment is required (FR-LR004, LV-010).
- **func** `(s *requestService) Cancel(ctx context.Context, a Actor, id uuid.UUID) (*dto.LeaveRequestResponse, error)` — Cancel is the owner's withdrawal (FR-LR005, LV-011).
- **func** `(s *requestService) settle(ctx context.Context, tx *gorm.DB, a Actor, r *models.Request, bal *models.Balance) error` — settle turns the reservation into usage: release −d, consume +d.
- **func** `(s *requestService) unwind(ctx context.Context, tx *gorm.DB, a Actor, r *models.Request, bal *models.Balance) error` — unwind reverses whatever the request holds: a reservation, or future approved usage + attendance marks.
- **func** `(s *requestService) leaveDates(ctx context.Context, tx *gorm.DB, r *models.Request) ([]time.Time, error)` — leaveDates are the full working days of a request for attendance (LV-015); half days mark nothing (D4-09).
- **func** `(s *requestService) review(r *models.Request, a Actor, status string, comment *string)`

#### `internal/hr/leave/services/request_service.go` (223 lines)
- **type** `RequestQuery struct` — RequestQuery is R5's filter.
- **type** `RequestService interface` — RequestService runs the leave request lifecycle (FR-LR001..LR006).
- **method** `RequestService.Preview(ctx context.Context, a Actor, req dto.ApplyLeaveRequest) (*dto.PreviewResponse, error)`
- **method** `RequestService.Apply(ctx context.Context, a Actor, req dto.ApplyLeaveRequest) (*dto.LeaveRequestResponse, error)`
- **method** `RequestService.ListMine(ctx context.Context, a Actor, status string, page dto.Page) ([]dto.LeaveRequestResponse, dto.PageMeta, error)`
- **method** `RequestService.List(ctx context.Context, tenantID uuid.UUID, q RequestQuery, page dto.Page) ([]dto.LeaveRequestResponse, dto.PageMeta, error)`
- **method** `RequestService.Get(ctx context.Context, tenantID, id uuid.UUID) (*dto.LeaveRequestResponse, error)`
- **method** `RequestService.Cancel(ctx context.Context, a Actor, id uuid.UUID) (*dto.LeaveRequestResponse, error)`
- **method** `RequestService.Approve(ctx context.Context, a Actor, id uuid.UUID, req dto.ReviewRequest) (*dto.LeaveRequestResponse, error)`
- **method** `RequestService.Reject(ctx context.Context, a Actor, id uuid.UUID, req dto.ReviewRequest) (*dto.LeaveRequestResponse, error)`
- **type** `requestService struct{ base }`
- **func** `NewRequestService(d Deps) RequestService { return &requestService{base{d}} }` — NewRequestService builds a RequestService.
- **type** `quote struct` — quote is a fully validated, priced request plus its locked balance.
- **func** `(s *requestService) Preview(ctx context.Context, a Actor, req dto.ApplyLeaveRequest) (*dto.PreviewResponse, error)`
- **func** `(s *requestService) Apply(ctx context.Context, a Actor, req dto.ApplyLeaveRequest) (*dto.LeaveRequestResponse, error)`
- **type** `quoteInput struct`
- **func** `(s *requestService) quote(ctx context.Context, tx *gorm.DB, a Actor, in quoteInput) (*quote, error)` — quote validates LV-002..LV-009 under the employee lock (LV-014) and prices the range.
- **func** `(s *requestService) applicableType(ctx context.Context, tx *gorm.DB, tenantID uuid.UUID, in quoteInput) (*models.LeaveType, error)`
- **func** `(s *requestService) parseRange(q *quote, req dto.ApplyLeaveRequest) error` — parseRange applies LV-003, LV-004 and LV-005.
- **func** `(s *requestService) price(ctx context.Context, tx *gorm.DB, tenantID uuid.UUID, q *quote) (calc.Count, error)` — price counts days with the tenant calendar (LV-006, LV-007).
- **func** `(b base) blockingHolidays(ctx context.Context, tx *gorm.DB, tenantID uuid.UUID, from, to time.Time) ([]time.Time, error)` — blockingHolidays returns non-optional holiday dates in range (optional ones are working days, D4-05).
- **func** `(s *requestService) checkOverlap(ctx context.Context, tx *gorm.DB, tenantID, employeeID uuid.UUID, q *quote) error` — checkOverlap applies LV-008: opposite halves of the same single day may coexist.

#### `internal/hr/leave/services/type_service.go` (215 lines)
- **type** `TypeService interface` — TypeService manages leave types (FR-LT001..LT004).
- **method** `TypeService.Create(ctx context.Context, a Actor, req dto.CreateLeaveTypeRequest) (*dto.LeaveTypeResponse, error)`
- **method** `TypeService.Get(ctx context.Context, tenantID, id uuid.UUID) (*dto.LeaveTypeResponse, error)`
- **method** `TypeService.List(ctx context.Context, tenantID uuid.UUID, status string, page dto.Page) ([]dto.LeaveTypeResponse, dto.PageMeta, error)`
- **method** `TypeService.Update(ctx context.Context, a Actor, id uuid.UUID, req dto.UpdateLeaveTypeRequest) (*dto.LeaveTypeResponse, error)`
- **method** `TypeService.Deactivate(ctx context.Context, a Actor, id uuid.UUID) (*dto.LeaveTypeResponse, error)`
- **type** `typeService struct{ base }`
- **func** `NewTypeService(d Deps) TypeService { return &typeService{base{d}} }` — NewTypeService builds a TypeService.
- **func** `(s *typeService) Create(ctx context.Context, a Actor, req dto.CreateLeaveTypeRequest) (*dto.LeaveTypeResponse, error)`
- **func** `(s *typeService) Get(ctx context.Context, tenantID, id uuid.UUID) (*dto.LeaveTypeResponse, error)`
- **func** `(s *typeService) List(ctx context.Context, tenantID uuid.UUID, status string, page dto.Page) ([]dto.LeaveTypeResponse, dto.PageMeta, error)`
- **func** `(s *typeService) Update(ctx context.Context, a Actor, id uuid.UUID, req dto.UpdateLeaveTypeRequest) (*dto.LeaveTypeResponse, error)`
- **func** `(s *typeService) Deactivate(ctx context.Context, a Actor, id uuid.UUID) (*dto.LeaveTypeResponse, error)`
- **func** `(s *typeService) find(ctx context.Context, db *gorm.DB, tenantID, id uuid.UUID) (*models.LeaveType, error)`
- **func** `(s *typeService) checkUnique(ctx context.Context, db *gorm.DB, t *models.LeaveType) error` — checkUnique enforces FR-LT004 name/code uniqueness (DB unique indexes are the backstop).
- **func** `applyTypeUpdate(t *models.LeaveType, req dto.UpdateLeaveTypeRequest) error` — applyTypeUpdate copies set fields (FR-LT003; code is immutable). max_consecutive_days 0 clears the limit.
- **func** `checkAmounts(allowance, carry calc.Days) error`
- **func** `boolOr(p *bool, def bool) bool`
- **func** `intOr(p *int, def int) int`
- **func** `strOr(s, def string) string`
- **func** `setStr(dst *string, p *string)`
- **func** `setBool(dst *bool, p *bool)`

#### `internal/hr/leave/services/unpaid_leave.go` (76 lines)
- **type** `UnpaidLeave interface` — UnpaidLeave is Module 4's read-only contract for Module 5 payroll (connections C3 of Module 5, D5-03): approved leave of unpaid types (is_paid = false), clipped to [from, to].
- **method** `UnpaidLeave.UnpaidDays(ctx context.Context, tenantID, employeeID uuid.UUID, from, to time.Time) (calc.Days, error)`
- **type** `unpaidLeave struct{ base }`
- **func** `NewUnpaidLeave(d Deps) UnpaidLeave { return &unpaidLeave{base{d}} }` — NewUnpaidLeave builds the UnpaidLeave port.
- **func** `(u *unpaidLeave) UnpaidDays(ctx context.Context, tenantID, employeeID uuid.UUID, from, to time.Time) (calc.Days, error)` — UnpaidDays counts approved unpaid leave days inside the range with the request's day-counting rules (holidays, sandwich, half day = 0.50).
- **type** `period struct{ from, to time.Time }`
- **func** `(u *unpaidLeave) typeFlags(ctx context.Context, tenantID, typeID uuid.UUID) (bool, bool, error)` — typeFlags returns (is_paid, sandwich_rule) for a type.
- **func** `(u *unpaidLeave) clippedDays(ctx context.Context, r models.Request, p period, sandwich bool) (calc.Days, error)`

#### `internal/hr/leave/validators/validators.go` (95 lines)
_Package:_ Package validators holds Module 4 field rules that struct tags cannot express (FR-LT001, LV-003..LV-005, LV-017).
- **func** `NormalizeTypeCode(code string) (string, error)` — NormalizeTypeCode upper-cases and checks a leave type code (FR-LT001).
- **func** `ValidateAllowance(d calc.Days) error` — ValidateAllowance checks an allowance/carry-forward amount in [0, 365].
- **func** `ValidateAdjustment(d calc.Days) error` — ValidateAdjustment checks an HR adjustment (LV-017).
- **func** `ParseDate(s string) (time.Time, error)` — ParseDate parses a YYYY-MM-DD calendar date as UTC midnight.
- **func** `ParseYear(s string, fallback int) (int, error)` — ParseYear parses an optional ?year= query; empty means fallback.
- **func** `GenderApplies(applicable, employeeGender string) bool` — GenderApplies reports whether a type's applicable_gender admits the employee (LV-002, A4-01).
- **func** `NoticeOK(start, today time.Time, minNotice int) bool` — NoticeOK applies LV-005: future notice when minNotice > 0, else a 30-day backdating window.

### M1 HR / Payroll


#### `internal/hr/payroll/catalogue.go` (19 lines)
- **func** `Catalogue() []access.PermissionDef` — Catalogue is HR/Payroll's permission catalogue with the scopes predefined roles receive by default (docs/ARCHITECTURE.md §17). The app registers it with M0 at boot.

#### `internal/hr/payroll/module.go` (71 lines)
_Package:_ Package payroll wires Module 5 (Payroll & Statutory Compliance) into the application.
- **type** `Module struct` — Module is the Module 5 DI container.
- **type** `Ports struct` — Ports are the cross-module collaborators Module 5 consumes (connections C2–C4).
- **func** `NewModule(db *gorm.DB, p Ports) *Module` — NewModule wires repositories, services and controllers.
- **func** `(m *Module) RegisterModels() []interface{}` — RegisterModels lists Module 5 models for legacy AutoMigrate (SQL 000038–000044 is the authority).
- **type** `RouteDeps struct` — RouteDeps is the Module 0 infrastructure the routes need.
- **func** `(m *Module) RegisterRoutes(rg *gin.RouterGroup, d RouteDeps)` — RegisterRoutes mounts Module 5 under rg.
- **func** `(m *Module) Relay() *services.OutboxRelay { return m.relay }` — Relay exposes the outbox relay.

#### `internal/hr/payroll/calc/compute.go` (95 lines)
- **type** `Input struct` — Input is everything needed to compute one payslip; PayableHundredths is payable days × 100 (half days allowed).
- **type** `Result struct` — Result is a computed payslip; Capped reports that deductions were trimmed to gross (PY-009).
- **func** `Compute(in Input) (Result, error)` — Compute runs the PY-001..PY-009 pipeline: breakdown → proration → statutory → net with cap.
- **type** `statutoryBase struct{ earnedBasic, fullGross, earnedGross, fullTaxable Money }`
- **func** `statutory(s StructureSpec, b statutoryBase) []Line` — statutory returns the enabled, non-zero statutory deduction lines in fixed order PF, ESI, PT, TDS.
- **func** `(r *Result) capDeductions()` — capDeductions trims deduction lines from the last one backwards so deductions never exceed gross (PY-009).

#### `internal/hr/payroll/calc/money.go` (20 lines)
_Package:_ Package calc is HR/Payroll's pure payroll arithmetic: structure breakdown, proration and Indian statutory deductions. No I/O; every rule is golden-tested. The Money amount type lives in the HR kernel; the aliases keep this package's API unchanged.
- **type** `Money = kernel.Money` — Money is an exact 2-decimal amount in hundredths (kernel.Money).
- **func** `Rupees(n int64) Money { return kernel.Rupees(n) }` — Rupees converts whole rupees to Money.
- **func** `ParseMoney(s string) (Money, error) { return kernel.ParseMoney(s) }` — ParseMoney parses a decimal amount with at most two decimals.
- **func** `Min(a, b Money) Money { return kernel.Min(a, b) }` — Min returns the smaller amount.
- **func** `divRound(n, d int64) Money { return kernel.DivRound(n, d) }`

#### `internal/hr/payroll/calc/statutory.go` (77 lines)
- **type** `slab struct` — slab is an upper bound (exclusive of the next) and its rate in hundredths of a percent.
- **func** `PF(earnedBasic Money) Money { return Min(earnedBasic, pfWageCeiling).Percent(pfRate).RoundRupee() }` — PF is the employee contribution: 12% of earned basic capped at the wage ceiling, to the rupee (PY-005).
- **func** `ESI(fullGross, earnedGross Money) Money` — ESI is 0.75% of earned gross, rounded up to the rupee, when full-month gross is within the ceiling (PY-006). Computed exactly (paise × 75 / 10000, ceiling to rupee) so round-then-ceil never under-collects.
- **func** `PT(earnedGross Money) Money` — PT is the default professional-tax slab (PY-007, D5-06).
- **func** `AnnualTax(taxable Money) Money` — AnnualTax is new-regime income tax on annual taxable income with the 87A rebate and 4% cess, to the rupee (PY-008).
- **func** `MonthlyTDS(fullMonthTaxable Money) Money` — MonthlyTDS projects full-month taxable earnings over 12 months (PY-008).

#### `internal/hr/payroll/calc/structure.go` (155 lines)
- **type** `ComponentSpec struct` — ComponentSpec is one structure line; Value is rupees (fixed) or a percentage in hundredths (percent_*).
- **type** `StructureSpec struct` — StructureSpec is a structure plus its statutory switches.
- **type** `Line struct` — Line is a computed payslip line.
- **func** `DaysInMonth(year, month int) int` — DaysInMonth returns the calendar days of a month.
- **func** `ValidateStructure(s StructureSpec) error` — ValidateStructure enforces FR-ST002; every failure wraps ErrInvalidStructure.
- **func** `validateComponent(c ComponentSpec) error`
- **func** `invalid(format string, args ...interface{}) error`
- **func** `Breakdown(s StructureSpec, monthlyCTC Money) ([]Line, error)` — Breakdown computes full-month amounts in component order (PY-001); balance takes CTC minus other earnings.
- **func** `componentAmount(c ComponentSpec, monthlyCTC, basic Money) Money`

#### `internal/hr/payroll/controllers/respond.go` (132 lines)
_Package:_ Package controllers is Module 5's thin HTTP edge: parse → validate → service → envelope.
- **type** `errorBody struct`
- **func** `ok(c *gin.Context, status int, data interface{})`
- **func** `okList(c *gin.Context, data interface{}, meta dto.PageMeta)`
- **func** `fail(c *gin.Context, err error)` — fail maps AppErrors to their status after shared normalization (unique violation → 409, not found → 404); anything else is an opaque 500 (no internals leaked).
- **func** `bindJSON(c *gin.Context, dst interface{}) bool` — bindJSON decodes and validates a body; failures become VALIDATION_ERROR with field details.
- **func** `snake(s string) string`
- **func** `actorFrom(c *gin.Context) (services.Actor, bool)` — actorFrom builds the caller from Module 0 middleware context only (NFR-SEC002).
- **func** `pathID(c *gin.Context, name string) (uuid.UUID, bool)` — pathID parses a :param UUID; malformed ids are 400.
- **func** `page(c *gin.Context) dto.Page`
- **func** `respond(c *gin.Context, status int, data interface{}, err error)` — respond writes data or the mapped error.
- **func** `respondList(c *gin.Context, data interface{}, meta dto.PageMeta, err error)` — respondList writes a paginated list or the mapped error.
- **func** `pathIDIf(c *gin.Context, actorOK bool) (uuid.UUID, bool)` — pathIDIf parses :id only when the actor resolved (the actor failure already wrote 401).

#### `internal/hr/payroll/controllers/run_controller.go` (135 lines)
- **type** `RunController struct` — RunController serves payroll runs and payslips (P10–P20).
- **func** `NewRunController(r services.RunService, p services.PayslipService) *RunController` — NewRunController builds a RunController.
- **func** `(ctl *RunController) CreateRun(c *gin.Context)` — CreateRun handles POST /payroll/runs (P10).
- **func** `(ctl *RunController) ListRuns(c *gin.Context)` — ListRuns handles GET /payroll/runs (P11).
- **func** `(ctl *RunController) GetRun(c *gin.Context)` — GetRun handles GET /payroll/runs/:id (P12).
- **type** `runAction func(ctx context.Context, a services.Actor, id uuid.UUID) (*dto.RunResponse, error)`
- **func** `(ctl *RunController) act(c *gin.Context, fn runAction)`
- **func** `(ctl *RunController) Calculate(c *gin.Context) { ctl.act(c, ctl.runs.Calculate) }` — Calculate handles POST /payroll/runs/:id/calculate (P13).
- **func** `(ctl *RunController) Approve(c *gin.Context) { ctl.act(c, ctl.runs.Approve) }` — Approve handles POST /payroll/runs/:id/approve (P14).
- **func** `(ctl *RunController) Finalize(c *gin.Context) { ctl.act(c, ctl.runs.Finalize) }` — Finalize handles POST /payroll/runs/:id/finalize (P15).
- **func** `(ctl *RunController) RunPayslips(c *gin.Context)` — RunPayslips handles GET /payroll/runs/:id/payslips (P16).
- **func** `(ctl *RunController) PayoutCSV(c *gin.Context)` — PayoutCSV handles GET /payroll/runs/:id/payout.csv (P17).
- **func** `(ctl *RunController) GetPayslip(c *gin.Context)` — GetPayslip handles GET /payroll/payslips/:id (P18).
- **func** `(ctl *RunController) MyPayslips(c *gin.Context)` — MyPayslips handles GET /payroll/payslips/me (P19); finalized only.
- **func** `(ctl *RunController) MyPayslip(c *gin.Context)` — MyPayslip handles GET /payroll/payslips/me/:id (P20).

#### `internal/hr/payroll/controllers/setup_controller.go` (117 lines)
- **type** `SetupController struct` — SetupController serves salary structures and CTC assignments (P1–P9).
- **func** `NewSetupController(s services.StructureService, a services.AssignmentService) *SetupController` — NewSetupController builds a SetupController.
- **func** `(ctl *SetupController) CreateStructure(c *gin.Context)` — CreateStructure handles POST /payroll/structures (P1).
- **func** `(ctl *SetupController) ListStructures(c *gin.Context)` — ListStructures handles GET /payroll/structures (P2).
- **func** `(ctl *SetupController) GetStructure(c *gin.Context)` — GetStructure handles GET /payroll/structures/:id (P3).
- **func** `(ctl *SetupController) UpdateStructure(c *gin.Context)` — UpdateStructure handles PATCH /payroll/structures/:id (P4).
- **func** `(ctl *SetupController) DeactivateStructure(c *gin.Context)` — DeactivateStructure handles POST /payroll/structures/:id/deactivate (P5).
- **func** `(ctl *SetupController) Assign(c *gin.Context)` — Assign handles POST /payroll/assignments (P6).
- **func** `(ctl *SetupController) ListAssignments(c *gin.Context)` — ListAssignments handles GET /payroll/assignments?employee_id= (P7).
- **func** `(ctl *SetupController) MyAssignment(c *gin.Context)` — MyAssignment handles GET /payroll/assignments/me (P8); the employee comes from the JWT only.
- **func** `(ctl *SetupController) Preview(c *gin.Context)` — Preview handles POST /payroll/assignments/preview (P9).

#### `internal/hr/payroll/dto/dto.go` (207 lines)
_Package:_ Package dto holds Module 5 request/response shapes (specification.md §5); never GORM models.
- **type** `ComponentRequest struct` — ComponentRequest is one structure line (FR-ST001); Value is rupees (fixed) or a percent (percent_*).
- **type** `CreateStructureRequest struct` — CreateStructureRequest is P1; statutory flags default to true.
- **type** `UpdateStructureRequest struct` — UpdateStructureRequest is P4: components are immutable (PY-015).
- **type** `AssignRequest struct` — AssignRequest is P6.
- **type** `PreviewRequest struct` — PreviewRequest is P9.
- **type** `CreateRunRequest struct` — CreateRunRequest is P10.
- **type** `ComponentResponse struct` — ComponentResponse serializes a component.
- **type** `StructureResponse struct` — StructureResponse serializes a structure with its components.
- **type** `AssignmentResponse struct` — AssignmentResponse serializes an assignment.
- **type** `LineResponse struct` — LineResponse is one breakdown or payslip line.
- **type** `BreakdownResponse struct` — BreakdownResponse is a full-month salary breakdown with statutory deductions (P8/P9).
- **type** `MyAssignmentResponse struct` — MyAssignmentResponse is P8.
- **type** `RunResponse struct` — RunResponse serializes a payroll run.
- **type** `PayslipResponse struct` — PayslipResponse serializes a payslip.
- **type** `PageMeta struct` — PageMeta is the list envelope meta.
- **type** `Page struct` — Page is a clamped page request (G16).
- **func** `ParsePage(pageStr, perPageStr string) Page` — ParsePage clamps raw query values (spec §5).
- **func** `(p Page) Offset() int { return (p.Page - 1) * p.PerPage }` — Offset is the SQL offset.
- **func** `(p Page) Meta(total int64) PageMeta` — Meta builds list metadata.

#### `internal/hr/payroll/events/events.go` (60 lines)
_Package:_ Package events defines Module 5 event types and the shared envelope (FR-EV001).
- **type** `Envelope struct` — Envelope is the platform event envelope (same shape as Modules 3/4).
- **type** `Source struct` — Source identifies who and when an event came from.
- **func** `New(routingKey string, src Source, payload interface{}) Envelope` — New wraps a payload in the envelope.
- **type** `RunPayload struct` — RunPayload carries run identity, period, counts and totals only — never per-employee amounts (FR-EV002).

#### `internal/hr/payroll/models/models.go` (191 lines)
_Package:_ Package models holds Module 5 (Payroll) persistence models; migrations 000038–000044 are the SQL twin.
- **type** `Structure struct` — Structure is a salary template with statutory switches (FR-ST001).
- **func** `(Structure) TableName() string { return "payroll_structures" }` — TableName maps to payroll_structures (migration 000038).
- **func** `(s *Structure) BeforeCreate(tx *gorm.DB) error { return assignID(&s.ID) }` — BeforeCreate assigns a UUID v4 when unset.
- **type** `Component struct` — Component is one immutable line of a structure (FR-ST001, PY-015). Value is rupees (fixed) or percent (percent_*).
- **func** `(Component) TableName() string { return "payroll_components" }` — TableName maps to payroll_components (migration 000039).
- **func** `(c *Component) BeforeCreate(tx *gorm.DB) error { return assignID(&c.ID) }` — BeforeCreate assigns a UUID v4 when unset.
- **type** `Assignment struct` — Assignment binds an employee to a structure and annual CTC from a date (FR-AS001, PY-014).
- **func** `(Assignment) TableName() string { return "payroll_assignments" }` — TableName maps to payroll_assignments (migration 000040).
- **func** `(a *Assignment) BeforeCreate(tx *gorm.DB) error { return assignID(&a.ID) }` — BeforeCreate assigns a UUID v4 when unset.
- **type** `Run struct` — Run is one tenant payroll period (FR-RN, PY-010). Warnings is a JSON array of strings.
- **func** `(Run) TableName() string { return "payroll_runs" }` — TableName maps to payroll_runs (migration 000041).
- **func** `(r *Run) BeforeCreate(tx *gorm.DB) error { return assignID(&r.ID) }` — BeforeCreate assigns a UUID v4 when unset.
- **type** `Payslip struct` — Payslip is one employee's result in a run, with an employee snapshot (FR-PS001, D5-09).
- **func** `(Payslip) TableName() string { return "payroll_payslips" }` — TableName maps to payroll_payslips (migration 000042).
- **func** `(p *Payslip) BeforeCreate(tx *gorm.DB) error { return assignID(&p.ID) }` — BeforeCreate assigns a UUID v4 when unset.
- **type** `PayslipLine struct` — PayslipLine is one earning or deduction on a payslip, statutory ones included (code PF, ESI, PT, TDS).
- **func** `(PayslipLine) TableName() string { return "payroll_payslip_lines" }` — TableName maps to payroll_payslip_lines (migration 000043).
- **func** `(l *PayslipLine) BeforeCreate(tx *gorm.DB) error { return assignID(&l.ID) }` — BeforeCreate assigns a UUID v4 when unset.
- **type** `OutboxEvent struct` — OutboxEvent is a transactional-outbox row (FR-EV001).
- **func** `(OutboxEvent) TableName() string { return "payroll_events_outbox" }` — TableName maps to payroll_events_outbox (migration 000044).
- **func** `(o *OutboxEvent) BeforeCreate(tx *gorm.DB) error { return assignID(&o.ID) }` — BeforeCreate assigns a UUID v4 when unset.
- **func** `assignID(id *uuid.UUID) error`

#### `internal/hr/payroll/repositories/interfaces.go` (92 lines)
_Package:_ Package repositories is Module 5 data access: tenant_id on every scoped query, `db`/`tx` passed per call.
- **type** `StructureRepository interface` — StructureRepository persists structures and their immutable components (FR-ST).
- **method** `StructureRepository.Create(ctx context.Context, tx *gorm.DB, s *models.Structure, comps []models.Component) error`
- **method** `StructureRepository.Update(ctx context.Context, tx *gorm.DB, s *models.Structure) error`
- **method** `StructureRepository.FindByID(ctx context.Context, db *gorm.DB, tenantID, id uuid.UUID) (*models.Structure, error)`
- **method** `StructureRepository.FindByName(ctx context.Context, db *gorm.DB, tenantID uuid.UUID, name string) (*models.Structure, error)`
- **method** `StructureRepository.Components(ctx context.Context, db *gorm.DB, tenantID, structureID uuid.UUID) ([]models.Component, error)`
- **method** `StructureRepository.List(ctx context.Context, db *gorm.DB, tenantID uuid.UUID, status string, page dto.Page) ([]models.Structure, int64, error)`
- **type** `Period struct{ From, To time.Time }` — Period is an inclusive date range.
- **type** `AssignmentRepository interface` — AssignmentRepository persists CTC assignments (FR-AS, PY-014).
- **method** `AssignmentRepository.LockEmployee(ctx context.Context, tx *gorm.DB, tenantID, employeeID uuid.UUID) error`
- **method** `AssignmentRepository.Create(ctx context.Context, tx *gorm.DB, a *models.Assignment) error`
- **method** `AssignmentRepository.Update(ctx context.Context, tx *gorm.DB, a *models.Assignment) error`
- **method** `AssignmentRepository.Current(ctx context.Context, db *gorm.DB, tenantID, employeeID uuid.UUID) (*models.Assignment, error)` — Current returns the open (effective_to IS NULL) assignment, or nil.
- **method** `AssignmentRepository.ListByEmployee(ctx context.Context, db *gorm.DB, tenantID, employeeID uuid.UUID, page dto.Page) ([]models.Assignment, int64, error)`
- **method** `AssignmentRepository.EligibleFor(ctx context.Context, db *gorm.DB, tenantID uuid.UUID, p Period) ([]models.Assignment, error)` — EligibleFor returns, per employee, the latest assignment overlapping the period (D5-10).
- **type** `RunRepository interface` — RunRepository persists payroll runs (FR-RN).
- **method** `RunRepository.Create(ctx context.Context, tx *gorm.DB, r *models.Run) error`
- **method** `RunRepository.Update(ctx context.Context, tx *gorm.DB, r *models.Run) error`
- **method** `RunRepository.FindByID(ctx context.Context, db *gorm.DB, tenantID, id uuid.UUID) (*models.Run, error)`
- **method** `RunRepository.LockByID(ctx context.Context, tx *gorm.DB, tenantID, id uuid.UUID) (*models.Run, error)` — LockByID loads the run with SELECT … FOR UPDATE (transitions are serialized).
- **method** `RunRepository.FindByPeriod(ctx context.Context, db *gorm.DB, tenantID uuid.UUID, year, month int) (*models.Run, error)`
- **method** `RunRepository.List(ctx context.Context, db *gorm.DB, tenantID uuid.UUID, status string, page dto.Page) ([]models.Run, int64, error)`
- **type** `PayslipRepository interface` — PayslipRepository persists payslips and their lines (FR-PS, PY-011).
- **method** `PayslipRepository.DeleteByRun(ctx context.Context, tx *gorm.DB, tenantID, runID uuid.UUID) error`
- **method** `PayslipRepository.CreateBatch(ctx context.Context, tx *gorm.DB, slips []models.Payslip) error`
- **method** `PayslipRepository.FindByID(ctx context.Context, db *gorm.DB, tenantID, id uuid.UUID) (*models.Payslip, error)`
- **method** `PayslipRepository.ListByRun(ctx context.Context, db *gorm.DB, tenantID, runID uuid.UUID, page dto.Page) ([]models.Payslip, int64, error)`
- **method** `PayslipRepository.AllByRun(ctx context.Context, db *gorm.DB, tenantID, runID uuid.UUID) ([]models.Payslip, error)`
- **method** `PayslipRepository.ListFinalizedForEmployee(ctx context.Context, db *gorm.DB, tenantID, employeeID uuid.UUID, page dto.Page) ([]models.Payslip, int64, error)` — ListFinalizedForEmployee returns the employee's payslips of finalized runs only (PY-013).
- **type** `OutboxRepository interface` — OutboxRepository persists outbox rows; relay methods are cross-tenant by design.
- **method** `OutboxRepository.Create(ctx context.Context, tx *gorm.DB, e *models.OutboxEvent) error`
- **method** `OutboxRepository.MarkPublished(ctx context.Context, db *gorm.DB, id uuid.UUID, at time.Time) error`
- **method** `OutboxRepository.RecordFailure(ctx context.Context, db *gorm.DB, id uuid.UUID, reason string) error`
- **method** `OutboxRepository.FetchUnpublished(ctx context.Context, db *gorm.DB, limit, maxAttempts int) ([]models.OutboxEvent, error)`
- **func** `day(t time.Time) string { return t.Format("2006-01-02") }`
- **func** `first[T any](q *gorm.DB) (*T, error)`
- **func** `paged[T any](q *gorm.DB, order string, page dto.Page) ([]T, int64, error)`

#### `internal/hr/payroll/repositories/run_repository.go` (113 lines)
- **type** `runRepository struct{}`
- **func** `NewRunRepository() RunRepository { return &runRepository{} }` — NewRunRepository builds a RunRepository.
- **func** `(r *runRepository) Create(ctx context.Context, tx *gorm.DB, run *models.Run) error`
- **func** `(r *runRepository) Update(ctx context.Context, tx *gorm.DB, run *models.Run) error`
- **func** `(r *runRepository) FindByID(ctx context.Context, db *gorm.DB, tenantID, id uuid.UUID) (*models.Run, error)`
- **func** `(r *runRepository) LockByID(ctx context.Context, tx *gorm.DB, tenantID, id uuid.UUID) (*models.Run, error)`
- **func** `(r *runRepository) FindByPeriod(ctx context.Context, db *gorm.DB, tenantID uuid.UUID, year, month int) (*models.Run, error)`
- **func** `(r *runRepository) List(ctx context.Context, db *gorm.DB, tenantID uuid.UUID, status string, page dto.Page) ([]models.Run, int64, error)`
- **type** `payslipRepository struct{}`
- **func** `NewPayslipRepository() PayslipRepository { return &payslipRepository{} }` — NewPayslipRepository builds a PayslipRepository.
- **func** `(r *payslipRepository) DeleteByRun(ctx context.Context, tx *gorm.DB, tenantID, runID uuid.UUID) error` — DeleteByRun removes a run's payslips; lines cascade (PY-011 recalculation).
- **func** `(r *payslipRepository) CreateBatch(ctx context.Context, tx *gorm.DB, slips []models.Payslip) error` — CreateBatch inserts payslips and their lines.
- **func** `(r *payslipRepository) withLines(db *gorm.DB) *gorm.DB`
- **func** `(r *payslipRepository) FindByID(ctx context.Context, db *gorm.DB, tenantID, id uuid.UUID) (*models.Payslip, error)`
- **func** `(r *payslipRepository) ListByRun(ctx context.Context, db *gorm.DB, tenantID, runID uuid.UUID, page dto.Page) ([]models.Payslip, int64, error)`
- **func** `(r *payslipRepository) AllByRun(ctx context.Context, db *gorm.DB, tenantID, runID uuid.UUID) ([]models.Payslip, error)`
- **func** `(r *payslipRepository) ListFinalizedForEmployee(ctx context.Context, db *gorm.DB, tenantID, employeeID uuid.UUID, page dto.Page) ([]models.Payslip, int64, error)`
- **type** `outboxRepository struct{}`
- **func** `NewOutboxRepository() OutboxRepository { return &outboxRepository{} }` — NewOutboxRepository builds an OutboxRepository.
- **func** `(r *outboxRepository) Create(ctx context.Context, tx *gorm.DB, e *models.OutboxEvent) error`
- **func** `(r *outboxRepository) MarkPublished(ctx context.Context, db *gorm.DB, id uuid.UUID, at time.Time) error`
- **func** `(r *outboxRepository) RecordFailure(ctx context.Context, db *gorm.DB, id uuid.UUID, reason string) error`
- **func** `(r *outboxRepository) FetchUnpublished(ctx context.Context, db *gorm.DB, limit, maxAttempts int) ([]models.OutboxEvent, error)`

#### `internal/hr/payroll/repositories/structure_repository.go` (89 lines)
- **type** `structureRepository struct{}`
- **func** `NewStructureRepository() StructureRepository { return &structureRepository{} }` — NewStructureRepository builds a StructureRepository.
- **func** `(r *structureRepository) Create(ctx context.Context, tx *gorm.DB, s *models.Structure, comps []models.Component) error`
- **func** `(r *structureRepository) Update(ctx context.Context, tx *gorm.DB, s *models.Structure) error`
- **func** `(r *structureRepository) FindByID(ctx context.Context, db *gorm.DB, tenantID, id uuid.UUID) (*models.Structure, error)`
- **func** `(r *structureRepository) FindByName(ctx context.Context, db *gorm.DB, tenantID uuid.UUID, name string) (*models.Structure, error)`
- **func** `(r *structureRepository) Components(ctx context.Context, db *gorm.DB, tenantID, structureID uuid.UUID) ([]models.Component, error)`
- **func** `(r *structureRepository) List(ctx context.Context, db *gorm.DB, tenantID uuid.UUID, status string, page dto.Page) ([]models.Structure, int64, error)`
- **type** `assignmentRepository struct{}`
- **func** `NewAssignmentRepository() AssignmentRepository { return &assignmentRepository{} }` — NewAssignmentRepository builds an AssignmentRepository.
- **func** `(r *assignmentRepository) LockEmployee(ctx context.Context, tx *gorm.DB, tenantID, employeeID uuid.UUID) error` — LockEmployee takes a tx-scoped advisory lock namespaced for payroll.
- **func** `(r *assignmentRepository) Create(ctx context.Context, tx *gorm.DB, a *models.Assignment) error`
- **func** `(r *assignmentRepository) Update(ctx context.Context, tx *gorm.DB, a *models.Assignment) error`
- **func** `(r *assignmentRepository) Current(ctx context.Context, db *gorm.DB, tenantID, employeeID uuid.UUID) (*models.Assignment, error)`
- **func** `(r *assignmentRepository) ListByEmployee(ctx context.Context, db *gorm.DB, tenantID, employeeID uuid.UUID, page dto.Page) ([]models.Assignment, int64, error)`
- **func** `(r *assignmentRepository) EligibleFor(ctx context.Context, db *gorm.DB, tenantID uuid.UUID, p Period) ([]models.Assignment, error)`

#### `internal/hr/payroll/routes/routes.go` (56 lines)
_Package:_ Package routes mounts Module 5 endpoints (specification.md §5) behind Module 0 middleware.
- **type** `Middleware struct` — Middleware is the Module 0 middleware Module 5 depends on.
- **type** `Controllers struct` — Controllers groups Module 5 handlers.
- **func** `RegisterRoutes(rg *gin.RouterGroup, mw Middleware, ctl Controllers)` — RegisterRoutes mounts all 20 Module 5 operations; self endpoints need authentication only (PY-013).

#### `internal/hr/payroll/services/assignment_service.go` (148 lines)
- **type** `AssignmentService interface` — AssignmentService manages CTC assignments (FR-AS001..AS003).
- **method** `AssignmentService.Assign(ctx context.Context, a Actor, req dto.AssignRequest) (*dto.AssignmentResponse, error)`
- **method** `AssignmentService.List(ctx context.Context, tenantID uuid.UUID, employeeID string, page dto.Page) ([]dto.AssignmentResponse, dto.PageMeta, error)`
- **method** `AssignmentService.Mine(ctx context.Context, a Actor) (*dto.MyAssignmentResponse, error)`
- **method** `AssignmentService.Preview(ctx context.Context, tenantID uuid.UUID, req dto.PreviewRequest) (*dto.BreakdownResponse, error)`
- **type** `assignmentService struct{ base }`
- **func** `NewAssignmentService(d Deps) AssignmentService { return &assignmentService{base{d}} }` — NewAssignmentService builds an AssignmentService.
- **func** `(s *assignmentService) Assign(ctx context.Context, a Actor, req dto.AssignRequest) (*dto.AssignmentResponse, error)`
- **func** `(s *assignmentService) assignTx(ctx context.Context, tx *gorm.DB, next *models.Assignment) error` — assignTx checks the structure, closes the open assignment and inserts the new one (PY-014).
- **func** `(s *assignmentService) List(ctx context.Context, tenantID uuid.UUID, employeeID string, page dto.Page) ([]dto.AssignmentResponse, dto.PageMeta, error)`
- **func** `(s *assignmentService) Mine(ctx context.Context, a Actor) (*dto.MyAssignmentResponse, error)` — Mine returns the caller's current assignment with a full-month breakdown (P8).
- **func** `(s *assignmentService) Preview(ctx context.Context, tenantID uuid.UUID, req dto.PreviewRequest) (*dto.BreakdownResponse, error)` — Preview prices a CTC on a structure without saving (P9).
- **func** `(s *assignmentService) breakdown(ctx context.Context, tenantID, structureID uuid.UUID, ctc calc.Money) (*dto.BreakdownResponse, error)`

#### `internal/hr/payroll/services/calculate.go` (160 lines)
- **type** `monthPeriod struct` — period is the run month as dates.
- **func** `periodOf(run *models.Run) monthPeriod`
- **func** `(s *runService) calculate(ctx context.Context, tx *gorm.DB, a Actor, run *models.Run) error` — calculate replaces the run's payslips (PY-011) and stores totals and warnings.
- **func** `(s *runService) totals(run *models.Run, slips []models.Payslip, warnings []string, calculator uuid.UUID)`
- **func** `(s *runService) payslipFor(ctx context.Context, tx *gorm.DB, run *models.Run, p monthPeriod, as models.Assignment, specs map[uuid.UUID]calc.StructureSpec) (*models.Payslip, string, error)` — payslipFor computes one employee (PY-001..PY-009); a nil payslip with a warning means "skipped".
- **func** `(s *runService) specCached(ctx context.Context, tx *gorm.DB, tenantID, id uuid.UUID, cache map[uuid.UUID]calc.StructureSpec) (calc.StructureSpec, error)`
- **func** `window(p monthPeriod, emp *employeeDTO.EmployeeResponse) (time.Time, time.Time, bool)` — window clips the month to the employment dates (PY-003); ok=false means no payslip.
- **func** `dateOnly(t time.Time) time.Time`
- **type** `slipFigures struct`
- **func** `buildPayslip(run *models.Run, as models.Assignment, emp *employeeDTO.EmployeeResponse, f slipFigures) *models.Payslip`

#### `internal/hr/payroll/services/errors.go` (31 lines)
- **func** `appErr(status int, code, msg string) *sharedErrors.AppError`
- **func** `invalid(field, msg string) *sharedErrors.AppError` — invalid builds a 400 VALIDATION_ERROR naming the offending field.

#### `internal/hr/payroll/services/helpers.go` (107 lines)
- **type** `base struct{ Deps }` — base carries Deps plus helpers shared by all services.
- **func** `(b base) today() time.Time`
- **func** `(b base) employeeFor(ctx context.Context, a Actor) (*employeeDTO.EmployeeResponse, error)` — employeeFor resolves the caller's own profile (self endpoints, NFR-SEC002).
- **func** `(b base) employeeByID(ctx context.Context, tenantID uuid.UUID, id uuid.UUID) (*employeeDTO.EmployeeResponse, error)` — employeeByID resolves a profile id in the caller's tenant.
- **func** `checkEmployee(emp *employeeDTO.EmployeeResponse, err error) (*employeeDTO.EmployeeResponse, error)`
- **func** `(b base) loadSpec(ctx context.Context, db *gorm.DB, tenantID, id uuid.UUID) (*models.Structure, calc.StructureSpec, error)` — loadSpec loads a structure and its components as calc input.
- **func** `specFor(s *models.Structure, comps []models.Component) calc.StructureSpec`
- **func** `(b base) writeOutbox(ctx context.Context, tx *gorm.DB, a Actor, key string, payload interface{}) (*models.OutboxEvent, error)` — writeOutbox stores the event inside the business transaction (FR-EV001).
- **type** `auditEntry struct` — auditEntry is one audit row: action, resource type, resource id, metadata (no salary amounts, NFR-SEC003).
- **func** `(b base) afterCommit(ctx context.Context, a Actor, e auditEntry, row *models.OutboxEvent)` — afterCommit audits and publishes best-effort.
- **func** `parseUUIDField(field, raw string) (uuid.UUID, error)`
- **func** `orNotFound(err error) error`

#### `internal/hr/payroll/services/mappers.go` (65 lines)
- **func** `mapStructure(s *models.Structure, comps []models.Component) dto.StructureResponse`
- **func** `mapAssignment(a *models.Assignment) dto.AssignmentResponse`
- **func** `mapBreakdown(monthlyCTC calc.Money, r calc.Result) dto.BreakdownResponse`
- **func** `mapRun(r *models.Run) dto.RunResponse`
- **func** `mapPayslip(p *models.Payslip, run *models.Run) dto.PayslipResponse`
- **func** `runPayload(r *models.Run) events.RunPayload`

#### `internal/hr/payroll/services/outbox_relay.go` (59 lines)
- **type** `OutboxRelay struct` — OutboxRelay republishes outbox rows the request path could not publish (FR-EV002).
- **func** `NewOutboxRelay(d Deps) *OutboxRelay { return &OutboxRelay{base: base{d}, interval: relayInterval} }` — NewOutboxRelay builds a relay over the module dependencies.
- **func** `(r *OutboxRelay) RelayOnce(ctx context.Context) (int, error)` — RelayOnce publishes one batch and returns how many rows were published.
- **func** `(r *OutboxRelay) Run(ctx context.Context)` — Run ticks RelayOnce until ctx is cancelled (started by cmd/main.go, stopped on shutdown).

#### `internal/hr/payroll/services/payslip_service.go` (129 lines)
- **type** `PayslipService interface` — PayslipService serves payslips and the payout export (FR-PS, FR-RN005, PY-013).
- **method** `PayslipService.ByRun(ctx context.Context, tenantID, runID uuid.UUID, page dto.Page) ([]dto.PayslipResponse, dto.PageMeta, error)`
- **method** `PayslipService.Get(ctx context.Context, tenantID, id uuid.UUID) (*dto.PayslipResponse, error)`
- **method** `PayslipService.Mine(ctx context.Context, a Actor, page dto.Page) ([]dto.PayslipResponse, dto.PageMeta, error)`
- **method** `PayslipService.MineOne(ctx context.Context, a Actor, id uuid.UUID) (*dto.PayslipResponse, error)`
- **method** `PayslipService.PayoutCSV(ctx context.Context, tenantID, runID uuid.UUID) ([]byte, error)`
- **type** `payslipService struct{ base }`
- **func** `NewPayslipService(d Deps) PayslipService { return &payslipService{base{d}} }` — NewPayslipService builds a PayslipService.
- **func** `(s *payslipService) run(ctx context.Context, tenantID, id uuid.UUID) (*models.Run, error)`
- **func** `(s *payslipService) ByRun(ctx context.Context, tenantID, runID uuid.UUID, page dto.Page) ([]dto.PayslipResponse, dto.PageMeta, error)`
- **func** `(s *payslipService) Get(ctx context.Context, tenantID, id uuid.UUID) (*dto.PayslipResponse, error)`
- **func** `(s *payslipService) Mine(ctx context.Context, a Actor, page dto.Page) ([]dto.PayslipResponse, dto.PageMeta, error)` — Mine lists the caller's payslips of finalized runs (PY-013).
- **func** `(s *payslipService) MineOne(ctx context.Context, a Actor, id uuid.UUID) (*dto.PayslipResponse, error)` — MineOne returns one of the caller's finalized payslips; anything else is 404 (PS-T1, PS-T2).
- **func** `(s *payslipService) PayoutCSV(ctx context.Context, tenantID, runID uuid.UUID) ([]byte, error)` — PayoutCSV exports employee_code, employee_name, net_pay for a finalized run (FR-RN005, PS-T8).

#### `internal/hr/payroll/services/ports.go` (76 lines)
_Package:_ Package services holds Module 5 business rules (specification.md PY-001..PY-015).
- **type** `EmployeeDirectory interface` — EmployeeDirectory is Module 5's view of Module 2 (connections C2).
- **method** `EmployeeDirectory.GetByUserID(ctx context.Context, tenantID, userID uuid.UUID) (*employeeDTO.EmployeeResponse, error)`
- **method** `EmployeeDirectory.GetByID(ctx context.Context, tenantID, id uuid.UUID) (*employeeDTO.EmployeeResponse, error)`
- **type** `LeaveLOP interface` — LeaveLOP is Module 5's view of Module 4 `services.UnpaidLeave` (connections C3, D5-03).
- **method** `LeaveLOP.UnpaidDays(ctx context.Context, tenantID, employeeID uuid.UUID, from, to time.Time) (kernel.Days, error)`
- **type** `AuditLogger interface` — AuditLogger is Module 5's view of the Module 0 AuditService.
- **method** `AuditLogger.Log(ctx context.Context, tx *gorm.DB, tenantID, userID string, action, resource, resourceID string, metadata interface{}, ipAddress, userAgent string) error`
- **type** `TxRunner interface` — TxRunner opens database transactions.
- **method** `TxRunner.InTx(ctx context.Context, fn func(tx *gorm.DB) error) error`
- **method** `TxRunner.DB() *gorm.DB`
- **type** `Clock func() time.Time` — Clock returns the current instant.
- **type** `Actor struct` — Actor is the authenticated caller from Module 0 middleware context only.
- **type** `Repos struct` — Repos bundles the module repositories.
- **type** `Deps struct` — Deps is everything a Module 5 service needs.
- **type** `gormRunner struct{ db *gorm.DB }`
- **func** `NewTxRunner(db *gorm.DB) TxRunner { return gormRunner{db: db} }` — NewTxRunner wraps a GORM handle as a TxRunner.
- **func** `(g gormRunner) InTx(ctx context.Context, fn func(tx *gorm.DB) error) error`
- **func** `(g gormRunner) DB() *gorm.DB { return g.db }`

#### `internal/hr/payroll/services/run_service.go` (162 lines)
- **type** `RunService interface` — RunService runs the monthly payroll lifecycle (FR-RN001..RN006, PY-010..PY-012).
- **method** `RunService.Create(ctx context.Context, a Actor, req dto.CreateRunRequest) (*dto.RunResponse, error)`
- **method** `RunService.Get(ctx context.Context, tenantID, id uuid.UUID) (*dto.RunResponse, error)`
- **method** `RunService.List(ctx context.Context, tenantID uuid.UUID, status string, page dto.Page) ([]dto.RunResponse, dto.PageMeta, error)`
- **method** `RunService.Calculate(ctx context.Context, a Actor, id uuid.UUID) (*dto.RunResponse, error)`
- **method** `RunService.Approve(ctx context.Context, a Actor, id uuid.UUID) (*dto.RunResponse, error)`
- **method** `RunService.Finalize(ctx context.Context, a Actor, id uuid.UUID) (*dto.RunResponse, error)`
- **type** `runService struct{ base }`
- **func** `NewRunService(d Deps) RunService { return &runService{base{d}} }` — NewRunService builds a RunService.
- **func** `(s *runService) Create(ctx context.Context, a Actor, req dto.CreateRunRequest) (*dto.RunResponse, error)`
- **func** `(s *runService) Get(ctx context.Context, tenantID, id uuid.UUID) (*dto.RunResponse, error)`
- **func** `(s *runService) List(ctx context.Context, tenantID uuid.UUID, status string, page dto.Page) ([]dto.RunResponse, dto.PageMeta, error)`
- **func** `(s *runService) Calculate(ctx context.Context, a Actor, id uuid.UUID) (*dto.RunResponse, error)` — Calculate (re)builds every payslip of a draft or calculated run (FR-RN002, PY-011).
- **func** `(s *runService) Approve(ctx context.Context, a Actor, id uuid.UUID) (*dto.RunResponse, error)` — Approve applies maker-checker (PY-012).
- **func** `(s *runService) Finalize(ctx context.Context, a Actor, id uuid.UUID) (*dto.RunResponse, error)` — Finalize locks the run and releases payslips to employees (FR-RN004).
- **type** `transition struct`
- **func** `(s *runService) transition(ctx context.Context, a Actor, id uuid.UUID, tr transition) (*dto.RunResponse, error)` — transition locks the run row, checks the source state, applies, saves and emits the event (PY-010).
- **func** `periodMeta(r *models.Run) map[string]string`
- **func** `contains(list []string, v string) bool`

#### `internal/hr/payroll/services/structure_service.go` (166 lines)
- **type** `StructureService interface` — StructureService manages salary structures (FR-ST001..ST003).
- **method** `StructureService.Create(ctx context.Context, a Actor, req dto.CreateStructureRequest) (*dto.StructureResponse, error)`
- **method** `StructureService.Get(ctx context.Context, tenantID, id uuid.UUID) (*dto.StructureResponse, error)`
- **method** `StructureService.List(ctx context.Context, tenantID uuid.UUID, status string, page dto.Page) ([]dto.StructureResponse, dto.PageMeta, error)`
- **method** `StructureService.Update(ctx context.Context, a Actor, id uuid.UUID, req dto.UpdateStructureRequest) (*dto.StructureResponse, error)`
- **method** `StructureService.Deactivate(ctx context.Context, a Actor, id uuid.UUID) (*dto.StructureResponse, error)`
- **type** `structureService struct{ base }`
- **func** `NewStructureService(d Deps) StructureService { return &structureService{base{d}} }` — NewStructureService builds a StructureService.
- **func** `(s *structureService) Create(ctx context.Context, a Actor, req dto.CreateStructureRequest) (*dto.StructureResponse, error)`
- **func** `(s *structureService) Get(ctx context.Context, tenantID, id uuid.UUID) (*dto.StructureResponse, error)`
- **func** `(s *structureService) List(ctx context.Context, tenantID uuid.UUID, status string, page dto.Page) ([]dto.StructureResponse, dto.PageMeta, error)`
- **func** `(s *structureService) Update(ctx context.Context, a Actor, id uuid.UUID, req dto.UpdateStructureRequest) (*dto.StructureResponse, error)`
- **func** `(s *structureService) Deactivate(ctx context.Context, a Actor, id uuid.UUID) (*dto.StructureResponse, error)`
- **func** `(s *structureService) mutate(ctx context.Context, a Actor, id uuid.UUID, action string, fn func(*gorm.DB, *models.Structure) error) (*dto.StructureResponse, error)` — mutate loads, applies and saves a structure inside one transaction, then audits.
- **func** `(s *structureService) checkName(ctx context.Context, db *gorm.DB, st *models.Structure) error`
- **func** `boolOr(p *bool, def bool) bool`
- **func** `setBool(dst *bool, p *bool)`
- **func** `isOverflow(err error) error` — isOverflow maps the calc overflow to the API error.

#### `internal/hr/payroll/validators/validators.go` (57 lines)
_Package:_ Package validators holds Module 5 field rules that struct tags cannot express.
- **func** `ParseDate(s string) (time.Time, error)` — ParseDate parses a YYYY-MM-DD date as UTC midnight.
- **func** `ValidateCTC(ctc calc.Money) error` — ValidateCTC enforces the CTC range.
- **func** `ValidatePeriod(year, month int, today time.Time) error` — ValidatePeriod rejects periods after the current month (FR-RN001).
- **func** `CSVSafe(s string) string` — CSVSafe neutralizes spreadsheet formula injection in exported cells (PS-T8) and strips line breaks.

### M1 HR / Recruitment


#### `internal/hr/recruitment/catalogue.go` (19 lines)
- **func** `Catalogue() []access.PermissionDef` — Catalogue is HR/Recruitment's permission catalogue with the scopes predefined roles receive by default (docs/ARCHITECTURE.md §17). The app registers it with M0 at boot.

#### `internal/hr/recruitment/module.go` (113 lines)
_Package:_ Package recruitment wires Module 6 (Recruitment & Applicant Tracking) into the application.
- **type** `Module struct` — Module is the Module 6 DI container.
- **type** `DepartmentReader interface` — DepartmentReader is the Module 1 department lookup Module 6 needs (D6-03).
- **method** `DepartmentReader.GetDepartment(ctx context.Context, db *gorm.DB, tenantID, id uuid.UUID) (*orgDTO.DepartmentResponse, error)`
- **type** `DesignationReader interface` — DesignationReader is the Module 1 designation lookup Module 6 needs (D6-03).
- **method** `DesignationReader.GetDesignation(ctx context.Context, db *gorm.DB, tenantID, id uuid.UUID) (*orgDTO.DesignationResponse, error)`
- **type** `Ports struct` — Ports are the cross-module collaborators Module 6 consumes (connections C2–C5).
- **func** `NewModule(db *gorm.DB, p Ports) *Module` — NewModule wires repositories, services and the controller.
- **type** `orgDirectory struct` — orgDirectory adapts Module 1 services to services.OrgDirectory: not found → false, other errors propagate.
- **func** `(o orgDirectory) DepartmentExists(ctx context.Context, tenantID, id uuid.UUID) (bool, error)`
- **func** `(o orgDirectory) DesignationExists(ctx context.Context, tenantID, id uuid.UUID) (bool, error)`
- **func** `exists(err error) (bool, error)`
- **func** `(m *Module) RegisterModels() []interface{}` — RegisterModels lists Module 6 models for legacy AutoMigrate (SQL 000045–000050 is the authority).
- **type** `RouteDeps struct` — RouteDeps is the Module 0 infrastructure the routes need.
- **func** `(m *Module) RegisterRoutes(rg *gin.RouterGroup, d RouteDeps)` — RegisterRoutes mounts Module 6 under rg.
- **func** `(m *Module) Relay() *services.OutboxRelay { return m.relay }` — Relay exposes the outbox relay.

#### `internal/hr/recruitment/controllers/controllers.go` (163 lines)
- **type** `JobService interface` — JobService is the R1–R5 surface the edge needs.
- **method** `JobService.Create(ctx context.Context, a services.Actor, req dto.CreateJobRequest) (*dto.JobResponse, error)`
- **method** `JobService.Get(ctx context.Context, tenantID, id uuid.UUID) (*dto.JobResponse, error)`
- **method** `JobService.List(ctx context.Context, tenantID uuid.UUID, f repositories.JobFilter, page dto.Page) ([]dto.JobResponse, dto.PageMeta, error)`
- **method** `JobService.Update(ctx context.Context, a services.Actor, id uuid.UUID, req dto.UpdateJobRequest) (*dto.JobResponse, error)`
- **method** `JobService.SetStatus(ctx context.Context, a services.Actor, id uuid.UUID, to string) (*dto.JobResponse, error)`
- **type** `CandidateService interface` — CandidateService is the R6–R10 surface.
- **method** `CandidateService.Create(ctx context.Context, a services.Actor, req dto.CreateCandidateRequest) (*dto.CandidateResponse, error)`
- **method** `CandidateService.Get(ctx context.Context, tenantID, id uuid.UUID) (*dto.CandidateResponse, error)`
- **method** `CandidateService.List(ctx context.Context, tenantID uuid.UUID, f repositories.CandidateFilter, page dto.Page) ([]dto.CandidateResponse, dto.PageMeta, error)`
- **method** `CandidateService.Update(ctx context.Context, a services.Actor, id uuid.UUID, req dto.UpdateCandidateRequest) (*dto.CandidateResponse, error)`
- **method** `CandidateService.MoveStage(ctx context.Context, a services.Actor, id uuid.UUID, req dto.StageRequest) (*dto.CandidateResponse, error)`
- **type** `HireService interface` — HireService is R11.
- **method** `HireService.Hire(ctx context.Context, a services.Actor, candidateID uuid.UUID, req dto.HireRequest) (*dto.HireResponse, error)`
- **type** `InterviewService interface` — InterviewService is R12–R16.
- **method** `InterviewService.Schedule(ctx context.Context, a services.Actor, req dto.ScheduleInterviewRequest) (*dto.InterviewResponse, error)`
- **method** `InterviewService.ListByCandidate(ctx context.Context, tenantID, candidateID uuid.UUID, page dto.Page) ([]dto.InterviewResponse, dto.PageMeta, error)`
- **method** `InterviewService.ListMine(ctx context.Context, a services.Actor, status string, page dto.Page) ([]dto.InterviewResponse, dto.PageMeta, error)`
- **method** `InterviewService.Feedback(ctx context.Context, a services.Actor, id uuid.UUID, req dto.FeedbackRequest) (*dto.InterviewResponse, error)`
- **method** `InterviewService.Cancel(ctx context.Context, a services.Actor, id uuid.UUID) (*dto.InterviewResponse, error)`
- **type** `OfferService interface` — OfferService is R17–R20.
- **method** `OfferService.Create(ctx context.Context, a services.Actor, req dto.CreateOfferRequest) (*dto.OfferResponse, error)`
- **method** `OfferService.List(ctx context.Context, tenantID, candidateID uuid.UUID, page dto.Page) ([]dto.OfferResponse, dto.PageMeta, error)`
- **method** `OfferService.Decide(ctx context.Context, a services.Actor, id uuid.UUID, decision string) (*dto.OfferResponse, error)`
- **method** `OfferService.Withdraw(ctx context.Context, a services.Actor, id uuid.UUID) (*dto.OfferResponse, error)`
- **type** `Controller struct` — Controller serves all 20 Module 6 operations.
- **type** `Services struct` — Services bundles the controller's collaborators.
- **func** `New(s Services) *Controller` — New builds the Controller.
- **func** `queryUUID(c *gin.Context, name string) (*uuid.UUID, bool)` — queryUUID parses an optional UUID query parameter; malformed → 400.
- **func** `requiredQueryUUID(c *gin.Context, name string) (uuid.UUID, bool)` — requiredQueryUUID is queryUUID for mandatory filters (R13, R18).
- **func** `(ctl *Controller) CreateJob(c *gin.Context)` — CreateJob handles POST /recruitment/jobs (R1).
- **func** `(ctl *Controller) ListJobs(c *gin.Context)` — ListJobs handles GET /recruitment/jobs (R2).
- **func** `(ctl *Controller) GetJob(c *gin.Context)` — GetJob handles GET /recruitment/jobs/:id (R3).
- **func** `(ctl *Controller) UpdateJob(c *gin.Context)` — UpdateJob handles PATCH /recruitment/jobs/:id (R4).
- **func** `(ctl *Controller) SetJobStatus(c *gin.Context)` — SetJobStatus handles POST /recruitment/jobs/:id/status (R5).

#### `internal/hr/recruitment/controllers/pipeline_controller.go` (187 lines)
- **func** `(ctl *Controller) CreateCandidate(c *gin.Context)` — CreateCandidate handles POST /recruitment/candidates (R6).
- **func** `(ctl *Controller) ListCandidates(c *gin.Context)` — ListCandidates handles GET /recruitment/candidates (R7).
- **func** `(ctl *Controller) GetCandidate(c *gin.Context)` — GetCandidate handles GET /recruitment/candidates/:id (R8).
- **func** `(ctl *Controller) UpdateCandidate(c *gin.Context)` — UpdateCandidate handles PATCH /recruitment/candidates/:id (R9).
- **func** `(ctl *Controller) MoveStage(c *gin.Context)` — MoveStage handles POST /recruitment/candidates/:id/stage (R10).
- **func** `(ctl *Controller) Hire(c *gin.Context)` — Hire handles POST /recruitment/candidates/:id/hire (R11).
- **func** `(ctl *Controller) ScheduleInterview(c *gin.Context)` — ScheduleInterview handles POST /recruitment/interviews (R12).
- **func** `(ctl *Controller) ListInterviews(c *gin.Context)` — ListInterviews handles GET /recruitment/interviews?candidate_id (R13).
- **func** `(ctl *Controller) MyInterviews(c *gin.Context)` — MyInterviews handles GET /recruitment/interviews/me (R14).
- **func** `(ctl *Controller) Feedback(c *gin.Context)` — Feedback handles POST /recruitment/interviews/:id/feedback (R15).
- **func** `(ctl *Controller) CancelInterview(c *gin.Context)` — CancelInterview handles POST /recruitment/interviews/:id/cancel (R16).
- **func** `(ctl *Controller) CreateOffer(c *gin.Context)` — CreateOffer handles POST /recruitment/offers (R17).
- **func** `(ctl *Controller) ListOffers(c *gin.Context)` — ListOffers handles GET /recruitment/offers?candidate_id (R18).
- **func** `(ctl *Controller) DecideOffer(c *gin.Context)` — DecideOffer handles POST /recruitment/offers/:id/decision (R19).
- **func** `(ctl *Controller) WithdrawOffer(c *gin.Context)` — WithdrawOffer handles POST /recruitment/offers/:id/withdraw (R20).

#### `internal/hr/recruitment/controllers/respond.go` (132 lines)
_Package:_ Package controllers is Module 6's thin HTTP edge: parse → validate → service → envelope.
- **type** `errorBody struct`
- **func** `ok(c *gin.Context, status int, data interface{})`
- **func** `okList(c *gin.Context, data interface{}, meta dto.PageMeta)`
- **func** `fail(c *gin.Context, err error)` — fail maps AppErrors to their status after shared normalization (unique violation → 409, not found → 404); anything else is an opaque 500 (no internals leaked).
- **func** `bindJSON(c *gin.Context, dst interface{}) bool` — bindJSON decodes and validates a body; failures become VALIDATION_ERROR with field details.
- **func** `snake(s string) string`
- **func** `actorFrom(c *gin.Context) (services.Actor, bool)` — actorFrom builds the caller from Module 0 middleware context only (NFR-SEC002).
- **func** `pathID(c *gin.Context, name string) (uuid.UUID, bool)` — pathID parses a :param UUID; malformed ids are 400.
- **func** `page(c *gin.Context) dto.Page`
- **func** `respond(c *gin.Context, status int, data interface{}, err error)` — respond writes data or the mapped error.
- **func** `respondList(c *gin.Context, data interface{}, meta dto.PageMeta, err error)` — respondList writes a paginated list or the mapped error.
- **func** `pathIDIf(c *gin.Context, actorOK bool) (uuid.UUID, bool)` — pathIDIf parses :id only when the actor resolved (the actor failure already wrote 401).

#### `internal/hr/recruitment/dto/dto.go` (224 lines)
_Package:_ Package dto holds Module 6 request/response shapes (specification.md §5); never GORM models.
- **type** `CreateJobRequest struct` — CreateJobRequest is R1.
- **type** `UpdateJobRequest struct` — UpdateJobRequest is R4 (pointer fields).
- **type** `JobStatusRequest struct` — JobStatusRequest is R5.
- **type** `CreateCandidateRequest struct` — CreateCandidateRequest is R6.
- **type** `UpdateCandidateRequest struct` — UpdateCandidateRequest is R9 (contact fields only; stage changes go through R10/R11/R17).
- **type** `StageRequest struct` — StageRequest is R10.
- **type** `HireRequest struct` — HireRequest is R11.
- **type** `ScheduleInterviewRequest struct` — ScheduleInterviewRequest is R12.
- **type** `FeedbackRequest struct` — FeedbackRequest is R15.
- **type** `CreateOfferRequest struct` — CreateOfferRequest is R17.
- **type** `OfferDecisionRequest struct` — OfferDecisionRequest is R19.
- **type** `JobResponse struct` — JobResponse serializes a job.
- **type** `StageEventResponse struct` — StageEventResponse serializes one pipeline change.
- **type** `CandidateResponse struct` — CandidateResponse serializes a candidate; History is set on R8 only.
- **type** `InterviewResponse struct` — InterviewResponse serializes an interview.
- **type** `OfferResponse struct` — OfferResponse serializes an offer.
- **type** `HireResponse struct` — HireResponse is R11.
- **type** `PageMeta struct` — PageMeta is the list envelope meta.
- **type** `Page struct{ Page, PerPage int }` — Page is a clamped page request (G14).
- **func** `ParsePage(pageStr, perPageStr string) Page` — ParsePage clamps raw query values (page ≥ 1 default 1, per_page 1–100 default 20).
- **func** `(p Page) Offset() int { return (p.Page - 1) * p.PerPage }` — Offset is the SQL offset.
- **func** `(p Page) Meta(total int64) PageMeta` — Meta builds list metadata.

#### `internal/hr/recruitment/events/events.go` (57 lines)
_Package:_ Package events defines Module 6 event types and the shared envelope (FR-EV001).
- **type** `Envelope struct` — Envelope is the platform event envelope (same shape as Modules 3–5).
- **type** `Source struct` — Source identifies who and when an event came from.
- **func** `New(routingKey string, src Source, payload interface{}) Envelope` — New wraps a payload in the envelope.
- **type** `Payload struct` — Payload carries ids and stages only — never candidate PII, CTC or feedback (FR-EV002, RC-011).

#### `internal/hr/recruitment/models/models.go` (167 lines)
_Package:_ Package models holds Module 6 (Recruitment) persistence models; migrations 000045–000050 are the SQL twin.
- **type** `Job struct` — Job is a job opening (FR-JB001).
- **func** `(Job) TableName() string { return "recruitment_jobs" }` — TableName maps to recruitment_jobs (migration 000045).
- **func** `(j *Job) BeforeCreate(tx *gorm.DB) error { return assignID(&j.ID) }` — BeforeCreate assigns a UUID v4 when unset.
- **type** `Candidate struct` — Candidate is one applicant for one job (FR-CD001).
- **func** `(Candidate) TableName() string { return "recruitment_candidates" }` — TableName maps to recruitment_candidates (migration 000046).
- **func** `(c *Candidate) BeforeCreate(tx *gorm.DB) error { return assignID(&c.ID) }` — BeforeCreate assigns a UUID v4 when unset.
- **type** `StageEvent struct` — StageEvent is one append-only pipeline change (RC-003).
- **func** `(StageEvent) TableName() string { return "recruitment_stage_events" }` — TableName maps to recruitment_stage_events (migration 000047).
- **func** `(e *StageEvent) BeforeCreate(tx *gorm.DB) error { return assignID(&e.ID) }` — BeforeCreate assigns a UUID v4 when unset.
- **type** `Interview struct` — Interview is one scheduled round with optional feedback (FR-IV).
- **func** `(Interview) TableName() string { return "recruitment_interviews" }` — TableName maps to recruitment_interviews (migration 000048).
- **func** `(i *Interview) BeforeCreate(tx *gorm.DB) error { return assignID(&i.ID) }` — BeforeCreate assigns a UUID v4 when unset.
- **type** `Offer struct` — Offer is an offer to a candidate (FR-OF); at most one `offered` per candidate (RC-006).
- **func** `(Offer) TableName() string { return "recruitment_offers" }` — TableName maps to recruitment_offers (migration 000049).
- **func** `(o *Offer) BeforeCreate(tx *gorm.DB) error { return assignID(&o.ID) }` — BeforeCreate assigns a UUID v4 when unset.
- **type** `OutboxEvent struct` — OutboxEvent is a transactional-outbox row (FR-EV001).
- **func** `(OutboxEvent) TableName() string { return "recruitment_events_outbox" }` — TableName maps to recruitment_events_outbox (migration 000050).
- **func** `(o *OutboxEvent) BeforeCreate(tx *gorm.DB) error { return assignID(&o.ID) }` — BeforeCreate assigns a UUID v4 when unset.
- **func** `assignID(id *uuid.UUID) error`

#### `internal/hr/recruitment/pipeline/pipeline.go` (63 lines)
_Package:_ Package pipeline is Module 6's pure state machines: candidate stages (RC-002) and job statuses (FR-JB003).
- **func** `CanMove(from, to string) bool { return contains(manualMoves[from], to) }` — CanMove reports whether a recruiter may move a candidate from → to.
- **func** `CanTransitionJob(from, to string) bool { return contains(jobMoves[from], to) }` — CanTransitionJob reports whether a job may change status from → to.
- **func** `IsTerminal(stage string) bool { return stage == Hired || stage == Rejected || stage == Withdrawn }` — IsTerminal reports a final candidate stage.
- **func** `ValidStage(stage string) bool` — ValidStage reports a known candidate stage.
- **func** `Editable(status string) bool { return status != JobClosed && status != JobFilled }` — Editable reports whether a job's details may still change (FR-JB002).
- **func** `contains(list []string, v string) bool`

#### `internal/hr/recruitment/repositories/interfaces.go` (95 lines)
_Package:_ Package repositories is Module 6 data access: tenant_id on every scoped query, `db`/`tx` passed per call.
- **type** `JobFilter struct` — JobFilter narrows job lists (R2).
- **type** `JobRepository interface` — JobRepository persists job openings (FR-JB).
- **method** `JobRepository.Create(ctx context.Context, tx *gorm.DB, j *models.Job) error`
- **method** `JobRepository.Update(ctx context.Context, tx *gorm.DB, j *models.Job) error`
- **method** `JobRepository.FindByID(ctx context.Context, db *gorm.DB, tenantID, id uuid.UUID) (*models.Job, error)`
- **method** `JobRepository.LockByID(ctx context.Context, tx *gorm.DB, tenantID, id uuid.UUID) (*models.Job, error)`
- **method** `JobRepository.List(ctx context.Context, db *gorm.DB, tenantID uuid.UUID, f JobFilter, page dto.Page) ([]models.Job, int64, error)`
- **type** `CandidateFilter struct` — CandidateFilter narrows candidate lists (R7).
- **type** `CandidateRepository interface` — CandidateRepository persists candidates and their append-only stage history (FR-CD, RC-003).
- **method** `CandidateRepository.Create(ctx context.Context, tx *gorm.DB, c *models.Candidate) error`
- **method** `CandidateRepository.Update(ctx context.Context, tx *gorm.DB, c *models.Candidate) error`
- **method** `CandidateRepository.FindByID(ctx context.Context, db *gorm.DB, tenantID, id uuid.UUID) (*models.Candidate, error)`
- **method** `CandidateRepository.LockByID(ctx context.Context, tx *gorm.DB, tenantID, id uuid.UUID) (*models.Candidate, error)`
- **method** `CandidateRepository.FindByJobEmail(ctx context.Context, db *gorm.DB, jobID uuid.UUID, email string) (*models.Candidate, error)`
- **method** `CandidateRepository.List(ctx context.Context, db *gorm.DB, tenantID uuid.UUID, f CandidateFilter, page dto.Page) ([]models.Candidate, int64, error)`
- **method** `CandidateRepository.AddEvent(ctx context.Context, tx *gorm.DB, e *models.StageEvent) error`
- **method** `CandidateRepository.Events(ctx context.Context, db *gorm.DB, tenantID, candidateID uuid.UUID) ([]models.StageEvent, error)`
- **type** `InterviewRepository interface` — InterviewRepository persists interviews (FR-IV).
- **method** `InterviewRepository.Create(ctx context.Context, tx *gorm.DB, i *models.Interview) error`
- **method** `InterviewRepository.Update(ctx context.Context, tx *gorm.DB, i *models.Interview) error`
- **method** `InterviewRepository.FindByID(ctx context.Context, db *gorm.DB, tenantID, id uuid.UUID) (*models.Interview, error)`
- **method** `InterviewRepository.ListByCandidate(ctx context.Context, db *gorm.DB, tenantID, candidateID uuid.UUID, page dto.Page) ([]models.Interview, int64, error)`
- **method** `InterviewRepository.ListByInterviewer(ctx context.Context, db *gorm.DB, tenantID, employeeID uuid.UUID, status string, page dto.Page) ([]models.Interview, int64, error)`
- **type** `OfferRepository interface` — OfferRepository persists offers (FR-OF).
- **method** `OfferRepository.Create(ctx context.Context, tx *gorm.DB, o *models.Offer) error`
- **method** `OfferRepository.Update(ctx context.Context, tx *gorm.DB, o *models.Offer) error`
- **method** `OfferRepository.FindByID(ctx context.Context, db *gorm.DB, tenantID, id uuid.UUID) (*models.Offer, error)`
- **method** `OfferRepository.LatestByStatus(ctx context.Context, db *gorm.DB, tenantID, candidateID uuid.UUID, status string) (*models.Offer, error)` — LatestByStatus returns the candidate's most recent offer in a status, or nil.
- **method** `OfferRepository.ListByCandidate(ctx context.Context, db *gorm.DB, tenantID, candidateID uuid.UUID, page dto.Page) ([]models.Offer, int64, error)`
- **type** `OutboxRepository interface` — OutboxRepository persists outbox rows; relay methods are cross-tenant by design.
- **method** `OutboxRepository.Create(ctx context.Context, tx *gorm.DB, e *models.OutboxEvent) error`
- **method** `OutboxRepository.MarkPublished(ctx context.Context, db *gorm.DB, id uuid.UUID, at time.Time) error`
- **method** `OutboxRepository.RecordFailure(ctx context.Context, db *gorm.DB, id uuid.UUID, reason string) error`
- **method** `OutboxRepository.FetchUnpublished(ctx context.Context, db *gorm.DB, limit, maxAttempts int) ([]models.OutboxEvent, error)`
- **func** `first[T any](q *gorm.DB) (*T, error)`
- **func** `paged[T any](q *gorm.DB, order string, page dto.Page) ([]T, int64, error)`

#### `internal/hr/recruitment/repositories/repositories.go` (149 lines)
- **type** `jobRepository struct{}`
- **func** `NewJobRepository() JobRepository { return &jobRepository{} }` — NewJobRepository builds a JobRepository.
- **func** `(r *jobRepository) Create(ctx context.Context, tx *gorm.DB, j *models.Job) error`
- **func** `(r *jobRepository) Update(ctx context.Context, tx *gorm.DB, j *models.Job) error`
- **func** `(r *jobRepository) FindByID(ctx context.Context, db *gorm.DB, tenantID, id uuid.UUID) (*models.Job, error)`
- **func** `(r *jobRepository) LockByID(ctx context.Context, tx *gorm.DB, tenantID, id uuid.UUID) (*models.Job, error)`
- **func** `(r *jobRepository) List(ctx context.Context, db *gorm.DB, tenantID uuid.UUID, f JobFilter, page dto.Page) ([]models.Job, int64, error)`
- **type** `candidateRepository struct{}`
- **func** `NewCandidateRepository() CandidateRepository { return &candidateRepository{} }` — NewCandidateRepository builds a CandidateRepository.
- **func** `(r *candidateRepository) Create(ctx context.Context, tx *gorm.DB, c *models.Candidate) error`
- **func** `(r *candidateRepository) Update(ctx context.Context, tx *gorm.DB, c *models.Candidate) error`
- **func** `(r *candidateRepository) FindByID(ctx context.Context, db *gorm.DB, tenantID, id uuid.UUID) (*models.Candidate, error)`
- **func** `(r *candidateRepository) LockByID(ctx context.Context, tx *gorm.DB, tenantID, id uuid.UUID) (*models.Candidate, error)`
- **func** `(r *candidateRepository) FindByJobEmail(ctx context.Context, db *gorm.DB, jobID uuid.UUID, email string) (*models.Candidate, error)`
- **func** `(r *candidateRepository) List(ctx context.Context, db *gorm.DB, tenantID uuid.UUID, f CandidateFilter, page dto.Page) ([]models.Candidate, int64, error)`
- **func** `(r *candidateRepository) AddEvent(ctx context.Context, tx *gorm.DB, e *models.StageEvent) error`
- **func** `(r *candidateRepository) Events(ctx context.Context, db *gorm.DB, tenantID, candidateID uuid.UUID) ([]models.StageEvent, error)`
- **type** `interviewRepository struct{}`
- **func** `NewInterviewRepository() InterviewRepository { return &interviewRepository{} }` — NewInterviewRepository builds an InterviewRepository.
- **func** `(r *interviewRepository) Create(ctx context.Context, tx *gorm.DB, i *models.Interview) error`
- **func** `(r *interviewRepository) Update(ctx context.Context, tx *gorm.DB, i *models.Interview) error`
- **func** `(r *interviewRepository) FindByID(ctx context.Context, db *gorm.DB, tenantID, id uuid.UUID) (*models.Interview, error)`
- **func** `(r *interviewRepository) ListByCandidate(ctx context.Context, db *gorm.DB, tenantID, candidateID uuid.UUID, page dto.Page) ([]models.Interview, int64, error)`
- **func** `(r *interviewRepository) ListByInterviewer(ctx context.Context, db *gorm.DB, tenantID, employeeID uuid.UUID, status string, page dto.Page) ([]models.Interview, int64, error)`
- **type** `offerRepository struct{}`
- **func** `NewOfferRepository() OfferRepository { return &offerRepository{} }` — NewOfferRepository builds an OfferRepository.
- **func** `(r *offerRepository) Create(ctx context.Context, tx *gorm.DB, o *models.Offer) error`
- **func** `(r *offerRepository) Update(ctx context.Context, tx *gorm.DB, o *models.Offer) error`
- **func** `(r *offerRepository) FindByID(ctx context.Context, db *gorm.DB, tenantID, id uuid.UUID) (*models.Offer, error)`
- **func** `(r *offerRepository) LatestByStatus(ctx context.Context, db *gorm.DB, tenantID, candidateID uuid.UUID, status string) (*models.Offer, error)`
- **func** `(r *offerRepository) ListByCandidate(ctx context.Context, db *gorm.DB, tenantID, candidateID uuid.UUID, page dto.Page) ([]models.Offer, int64, error)`
- **type** `outboxRepository struct{}`
- **func** `NewOutboxRepository() OutboxRepository { return &outboxRepository{} }` — NewOutboxRepository builds an OutboxRepository.
- **func** `(r *outboxRepository) Create(ctx context.Context, tx *gorm.DB, e *models.OutboxEvent) error`
- **func** `(r *outboxRepository) MarkPublished(ctx context.Context, db *gorm.DB, id uuid.UUID, at time.Time) error`
- **func** `(r *outboxRepository) RecordFailure(ctx context.Context, db *gorm.DB, id uuid.UUID, reason string) error`
- **func** `(r *outboxRepository) FetchUnpublished(ctx context.Context, db *gorm.DB, limit, maxAttempts int) ([]models.OutboxEvent, error)`

#### `internal/hr/recruitment/routes/routes.go` (50 lines)
_Package:_ Package routes mounts Module 6 endpoints (specification.md §5) behind Module 0 middleware.
- **type** `Middleware struct` — Middleware is the Module 0 middleware Module 6 depends on.
- **func** `RegisterRoutes(rg *gin.RouterGroup, mw Middleware, ctl *controllers.Controller)` — RegisterRoutes mounts all 20 Module 6 operations; interviewer self endpoints need authentication only (NFR-SEC002).

#### `internal/hr/recruitment/services/candidate_service.go` (190 lines)
- **type** `CandidateService struct{ base }` — CandidateService owns candidates and their pipeline (FR-CD).
- **func** `NewCandidateService(d Deps) *CandidateService { return &CandidateService{base{d}} }` — NewCandidateService builds a CandidateService.
- **func** `(s *CandidateService) Create(ctx context.Context, a Actor, req dto.CreateCandidateRequest) (*dto.CandidateResponse, error)` — Create is R6: a candidate on an open job, stage applied + first stage event (RC-001, RC-003).
- **func** `(s *CandidateService) apply(ctx context.Context, tx *gorm.DB, a Actor, c *models.Candidate) (*models.OutboxEvent, error)`
- **func** `(s *CandidateService) ensureUniqueEmail(ctx context.Context, tx *gorm.DB, jobID uuid.UUID, email string, self uuid.UUID) error` — ensureUniqueEmail enforces FR-CD001 (case-insensitive per job); self is excluded on update.
- **func** `(s *CandidateService) Get(ctx context.Context, tenantID, id uuid.UUID) (*dto.CandidateResponse, error)` — Get is R8 with stage history.
- **func** `(s *CandidateService) List(ctx context.Context, tenantID uuid.UUID, f repositories.CandidateFilter, page dto.Page) ([]dto.CandidateResponse, dto.PageMeta, error)` — List is R7.
- **func** `(s *CandidateService) Update(ctx context.Context, a Actor, id uuid.UUID, req dto.UpdateCandidateRequest) (*dto.CandidateResponse, error)` — Update is R9: contact fields only, never for a hired candidate.
- **func** `(s *CandidateService) MoveStage(ctx context.Context, a Actor, id uuid.UUID, req dto.StageRequest) (*dto.CandidateResponse, error)` — MoveStage is R10 along RC-002 only.
- **func** `(s *CandidateService) closeOpenOffer(ctx context.Context, tx *gorm.DB, c *models.Candidate) error` — closeOpenOffer withdraws an open offer when a candidate leaves the pipeline from `offer` (RC-006 frees the slot).
- **func** `applyCandidatePatch(c *models.Candidate, req dto.UpdateCandidateRequest)`

#### `internal/hr/recruitment/services/errors.go` (31 lines)
- **func** `appErr(status int, code, msg string) *sharedErrors.AppError`
- **func** `invalid(field, msg string) *sharedErrors.AppError` — invalid builds a 400 VALIDATION_ERROR naming the offending field.

#### `internal/hr/recruitment/services/helpers.go` (112 lines)
- **type** `base struct{ Deps }` — base carries Deps plus helpers shared by all services.
- **func** `(b base) today() time.Time`
- **func** `(b base) employeeFor(ctx context.Context, a Actor) (*employeeDTO.EmployeeResponse, error)` — employeeFor resolves the caller's own employee profile (interviewer self endpoints, NFR-SEC002).
- **type** `stageChange struct` — stageChange records RC-003: candidate stage update + append-only event in the caller's transaction.
- **func** `(b base) moveStage(ctx context.Context, tx *gorm.DB, a Actor, c *models.Candidate, ch stageChange) error`
- **func** `(b base) lockCandidate(ctx context.Context, tx *gorm.DB, tenantID uuid.UUID, id uuid.UUID) (*models.Candidate, error)` — lockCandidate loads the candidate FOR UPDATE in the caller's tenant; unknown → 404.
- **func** `(b base) writeOutbox(ctx context.Context, tx *gorm.DB, a Actor, key string, payload events.Payload) (*models.OutboxEvent, error)` — writeOutbox stores the event inside the business transaction (FR-EV001).
- **type** `auditEntry struct` — auditEntry is one audit row; metadata holds ids/stages only (RC-011).
- **func** `(b base) afterCommit(ctx context.Context, a Actor, e auditEntry, row *models.OutboxEvent)`
- **func** `parseUUIDField(field, raw string) (uuid.UUID, error)`
- **func** `parseOptionalUUID(field string, raw *string) (*uuid.UUID, error)`
- **func** `orNotFound(err error) error`
- **func** `strPtr(s string) *string { return &s }`

#### `internal/hr/recruitment/services/hire_service.go` (145 lines)
- **type** `HireService struct{ base }` — HireService runs the three-step idempotent hire saga (FR-HR001, D6-04, RC-007/008/010).
- **func** `NewHireService(d Deps) *HireService { return &HireService{base{d}} }` — NewHireService builds a HireService.
- **type** `hirePlan struct` — hirePlan is what step 1 resolves for steps 2 and 3.
- **func** `(s *HireService) Hire(ctx context.Context, a Actor, candidateID uuid.UUID, req dto.HireRequest) (*dto.HireResponse, error)` — Hire is R11.
- **func** `(s *HireService) reserveUser(ctx context.Context, a Actor, candidateID uuid.UUID) (*hirePlan, error)` — reserveUser is step 1: validate hireability and invite the user once, storing its id on the candidate (RC-010).
- **func** `(s *HireService) loadHireable(ctx context.Context, tx *gorm.DB, tenantID, candidateID uuid.UUID, plan *hirePlan) error` — loadHireable locks candidate and job and checks RC-007/RC-008.
- **func** `(s *HireService) ensureEmployee(ctx context.Context, tenantID uuid.UUID, plan *hirePlan, req dto.HireRequest) (*employeeDTO.EmployeeResponse, error)` — ensureEmployee is step 2: reuse the user's profile if a previous attempt created it, else create one in Module 2.
- **func** `(s *HireService) complete(ctx context.Context, a Actor, candidateID, employeeID uuid.UUID) (*models.Job, *models.OutboxEvent, error)` — complete is step 3: candidate hired, job count (+ filled), stage event and outbox in one transaction.

#### `internal/hr/recruitment/services/interview_service.go` (147 lines)
- **type** `InterviewService struct{ base }` — InterviewService owns interviews and interviewer self-service (FR-IV).
- **func** `NewInterviewService(d Deps) *InterviewService { return &InterviewService{base{d}} }` — NewInterviewService builds an InterviewService.
- **func** `(s *InterviewService) Schedule(ctx context.Context, a Actor, req dto.ScheduleInterviewRequest) (*dto.InterviewResponse, error)` — Schedule is R12 (RC-004; interviewer must be a working employee, A6-01).
- **func** `(s *InterviewService) checkInterviewer(ctx context.Context, tenantID, id uuid.UUID) error`
- **func** `(s *InterviewService) ListByCandidate(ctx context.Context, tenantID, candidateID uuid.UUID, page dto.Page) ([]dto.InterviewResponse, dto.PageMeta, error)` — ListByCandidate is R13.
- **func** `(s *InterviewService) ListMine(ctx context.Context, a Actor, status string, page dto.Page) ([]dto.InterviewResponse, dto.PageMeta, error)` — ListMine is R14: the caller's own interviews (NFR-SEC002).
- **func** `(s *InterviewService) Feedback(ctx context.Context, a Actor, id uuid.UUID, req dto.FeedbackRequest) (*dto.InterviewResponse, error)` — Feedback is R15: only the assigned interviewer, only on scheduled interviews (RC-005).
- **func** `(s *InterviewService) Cancel(ctx context.Context, a Actor, id uuid.UUID) (*dto.InterviewResponse, error)` — Cancel is R16.
- **func** `(s *InterviewService) transition(ctx context.Context, tenantID, id uuid.UUID, owner *uuid.UUID, fn func(*models.Interview)) (*models.Interview, error)` — transition loads a scheduled interview (owned by owner when set; others 404), applies fn and saves it in one transaction.

#### `internal/hr/recruitment/services/job_service.go` (184 lines)
- **type** `JobService struct{ base }` — JobService owns job openings (FR-JB).
- **func** `NewJobService(d Deps) *JobService { return &JobService{base{d}} }` — NewJobService builds a JobService.
- **func** `(s *JobService) Create(ctx context.Context, a Actor, req dto.CreateJobRequest) (*dto.JobResponse, error)` — Create is R1: a draft job with Module 1 references validated.
- **func** `(s *JobService) Get(ctx context.Context, tenantID, id uuid.UUID) (*dto.JobResponse, error)` — Get is R3.
- **func** `(s *JobService) List(ctx context.Context, tenantID uuid.UUID, f repositories.JobFilter, page dto.Page) ([]dto.JobResponse, dto.PageMeta, error)` — List is R2.
- **func** `(s *JobService) Update(ctx context.Context, a Actor, id uuid.UUID, req dto.UpdateJobRequest) (*dto.JobResponse, error)` — Update is R4: any field while the job is not closed/filled (FR-JB002).
- **func** `(s *JobService) SetStatus(ctx context.Context, a Actor, id uuid.UUID, to string) (*dto.JobResponse, error)` — SetStatus is R5 (FR-JB003); the first open publishes recruitment.job_published.
- **func** `(s *JobService) applyStatus(ctx context.Context, tx *gorm.DB, a Actor, j *models.Job, to string) (*models.OutboxEvent, error)`
- **func** `(s *JobService) lockJob(ctx context.Context, tx *gorm.DB, tenantID, id uuid.UUID) (*models.Job, error)`
- **func** `(s *JobService) orgRefs(ctx context.Context, tenantID uuid.UUID, deptRaw, desigRaw *string) (*uuid.UUID, *uuid.UUID, error)` — orgRefs parses and validates optional department/designation ids against Module 1 (D6-03).
- **func** `(s *JobService) orgRef(ctx context.Context, tenantID uuid.UUID, field string, raw *string) (*uuid.UUID, error)`
- **func** `applyJobPatch(j *models.Job, req dto.UpdateJobRequest, dept, desig *uuid.UUID)`
- **func** `setIf[T any](dst *T, src *T)`

#### `internal/hr/recruitment/services/mappers.go` (47 lines)
- **func** `mapJob(j *models.Job) dto.JobResponse`
- **func** `mapCandidate(c *models.Candidate) dto.CandidateResponse`
- **func** `mapEvent(e *models.StageEvent) dto.StageEventResponse`
- **func** `mapInterview(i *models.Interview) dto.InterviewResponse`
- **func** `mapOffer(o *models.Offer) dto.OfferResponse`
- **func** `mapList[M any, R any](rows []M, fn func(*M) R) []R`

#### `internal/hr/recruitment/services/offer_service.go` (138 lines)
- **type** `OfferService struct{ base }` — OfferService owns offers (FR-OF).
- **func** `NewOfferService(d Deps) *OfferService { return &OfferService{base{d}} }` — NewOfferService builds an OfferService.
- **func** `(s *OfferService) Create(ctx context.Context, a Actor, req dto.CreateOfferRequest) (*dto.OfferResponse, error)` — Create is R17: one open offer per candidate (RC-006); an interview-stage candidate moves to offer.
- **func** `(s *OfferService) newOffer(a Actor, candID uuid.UUID, req dto.CreateOfferRequest) (*models.Offer, error)` — newOffer validates the request fields (FR-OF001) into an unsaved offer.
- **func** `(s *OfferService) ensureNoLiveOffer(ctx context.Context, tx *gorm.DB, c *models.Candidate) error` — ensureNoLiveOffer rejects a new offer while one is open or already accepted.
- **func** `(s *OfferService) List(ctx context.Context, tenantID, candidateID uuid.UUID, page dto.Page) ([]dto.OfferResponse, dto.PageMeta, error)` — List is R18.
- **func** `(s *OfferService) Decide(ctx context.Context, a Actor, id uuid.UUID, decision string) (*dto.OfferResponse, error)` — Decide is R19 (D6-05): accepted|declined on an open, unexpired offer.
- **func** `(s *OfferService) Withdraw(ctx context.Context, a Actor, id uuid.UUID) (*dto.OfferResponse, error)` — Withdraw is R20.
- **func** `(s *OfferService) close(ctx context.Context, a Actor, id uuid.UUID, to string) (*dto.OfferResponse, error)`

#### `internal/hr/recruitment/services/outbox_relay.go` (59 lines)
- **type** `OutboxRelay struct` — OutboxRelay republishes outbox rows the request path could not publish.
- **func** `NewOutboxRelay(d Deps) *OutboxRelay { return &OutboxRelay{base: base{d}, interval: relayInterval} }` — NewOutboxRelay builds a relay over the module dependencies.
- **func** `(r *OutboxRelay) RelayOnce(ctx context.Context) (int, error)` — RelayOnce publishes one batch and returns how many rows were published.
- **func** `(r *OutboxRelay) Run(ctx context.Context)` — Run ticks RelayOnce until ctx is cancelled.

#### `internal/hr/recruitment/services/ports.go` (84 lines)
_Package:_ Package services holds Module 6 business rules (specification.md RC-001..RC-011).
- **type** `EmployeeDirectory interface` — EmployeeDirectory is Module 6's view of Module 2 (connections C5).
- **method** `EmployeeDirectory.GetByID(ctx context.Context, tenantID, id uuid.UUID) (*employeeDTO.EmployeeResponse, error)`
- **method** `EmployeeDirectory.GetByUserID(ctx context.Context, tenantID, userID uuid.UUID) (*employeeDTO.EmployeeResponse, error)`
- **method** `EmployeeDirectory.CreateEmployee(ctx context.Context, tenantID uuid.UUID, req employeeDTO.CreateEmployeeRequest) (*employeeDTO.EmployeeResponse, error)`
- **type** `UserInviter interface` — UserInviter is HR/Recruitment's view of M0 user invitation (connections C2, D6-02); identityport.Users satisfies it.
- **method** `UserInviter.InviteUser(ctx context.Context, tx *gorm.DB, tenantID uuid.UUID, in identityport.Invite, correlationID uuid.UUID) (*identityport.User, error)`
- **type** `OrgDirectory interface` — OrgDirectory answers whether Module 1 references exist (connections C4, D6-03).
- **method** `OrgDirectory.DepartmentExists(ctx context.Context, tenantID, id uuid.UUID) (bool, error)`
- **method** `OrgDirectory.DesignationExists(ctx context.Context, tenantID, id uuid.UUID) (bool, error)`
- **type** `AuditLogger interface` — AuditLogger is Module 6's view of the Module 0 AuditService.
- **method** `AuditLogger.Log(ctx context.Context, tx *gorm.DB, tenantID, userID string, action, resource, resourceID string, metadata interface{}, ipAddress, userAgent string) error`
- **type** `TxRunner interface` — TxRunner opens database transactions.
- **method** `TxRunner.InTx(ctx context.Context, fn func(tx *gorm.DB) error) error`
- **method** `TxRunner.DB() *gorm.DB`
- **type** `Clock func() time.Time` — Clock returns the current instant.
- **type** `Actor struct` — Actor is the authenticated caller from Module 0 middleware context only.
- **type** `Repos struct` — Repos bundles the module repositories.
- **type** `Deps struct` — Deps is everything a Module 6 service needs.
- **type** `gormRunner struct{ db *gorm.DB }`
- **func** `NewTxRunner(db *gorm.DB) TxRunner { return gormRunner{db: db} }` — NewTxRunner wraps a GORM handle as a TxRunner.
- **func** `(g gormRunner) InTx(ctx context.Context, fn func(tx *gorm.DB) error) error`
- **func** `(g gormRunner) DB() *gorm.DB { return g.db }`

#### `internal/hr/recruitment/validators/validators.go` (63 lines)
_Package:_ Package validators holds Module 6 field rules that struct tags cannot express.
- **func** `ParseDate(s string) (time.Time, error)` — ParseDate parses a YYYY-MM-DD date as UTC midnight.
- **func** `ParseFutureDate(s string, today time.Time) (time.Time, error)` — ParseFutureDate parses a date that must be today or later (FR-OF001 joining date).
- **func** `FutureInstant(at, now time.Time) error` — FutureInstant enforces RC-004 (scheduled_at after now).
- **func** `ValidateCTC(m kernel.Money) error` — ValidateCTC checks an offered/expected CTC.
- **func** `NormalizeEmail(s string) string { return strings.ToLower(strings.TrimSpace(s)) }` — NormalizeEmail lower-cases and trims an email for the per-job uniqueness check (FR-CD001).

### M1 HR / Performance & OKRs


#### `internal/hr/performance/catalogue.go` (19 lines)
- **func** `Catalogue() []access.PermissionDef` — Catalogue is HR/Performance's permission catalogue with the scopes predefined roles receive by default (docs/ARCHITECTURE.md §17). Self, manager and reviewer access needs no permission (PM-008).

#### `internal/hr/performance/module.go` (68 lines)
_Package:_ Package performance wires HR/Performance & OKRs (docs/modules/m1-hr-management/contexts/performance) into M1.
- **type** `Module struct` — Module is the HR/Performance DI container.
- **type** `Ports struct` — Ports are the collaborators HR/Performance consumes (design.md connections C2–C4).
- **func** `NewModule(db *gorm.DB, p Ports) *Module` — NewModule wires repositories, services and the controller.
- **func** `(m *Module) RegisterModels() []interface{}` — RegisterModels lists HR/Performance models for legacy AutoMigrate (the SQL migrations are the authority).
- **type** `RouteDeps struct` — RouteDeps is the M0 infrastructure the routes need.
- **func** `(m *Module) RegisterRoutes(rg *gin.RouterGroup, d RouteDeps)` — RegisterRoutes mounts HR/Performance under rg.
- **func** `(m *Module) Relay() *services.OutboxRelay { return m.relay }` — Relay exposes the outbox relay.

#### `internal/hr/performance/controllers/controllers.go` (339 lines)
- **type** `CycleService interface` — CycleService is the C1–C8 surface the edge needs.
- **method** `CycleService.Create(ctx context.Context, a services.Actor, req dto.CreateCycleRequest) (*dto.CycleResponse, error)`
- **method** `CycleService.Get(ctx context.Context, tenantID, id uuid.UUID) (*dto.CycleResponse, error)`
- **method** `CycleService.List(ctx context.Context, tenantID uuid.UUID, status string, page dto.Page) ([]dto.CycleResponse, dto.PageMeta, error)`
- **method** `CycleService.Update(ctx context.Context, a services.Actor, id uuid.UUID, req dto.UpdateCycleRequest) (*dto.CycleResponse, error)`
- **method** `CycleService.Activate(ctx context.Context, a services.Actor, id uuid.UUID) (*dto.CycleResponse, error)`
- **method** `CycleService.LaunchReviews(ctx context.Context, a services.Actor, id uuid.UUID) (*dto.LaunchResponse, error)`
- **method** `CycleService.Complete(ctx context.Context, a services.Actor, id uuid.UUID) (*dto.CycleResponse, error)`
- **method** `CycleService.NineBox(ctx context.Context, tenantID, id uuid.UUID) (*dto.NineBoxResponse, error)`
- **type** `GoalService interface` — GoalService is the G1–G8 surface.
- **method** `GoalService.Create(ctx context.Context, a services.Actor, req dto.CreateGoalRequest) (*dto.GoalResponse, error)`
- **method** `GoalService.Get(ctx context.Context, a services.Actor, id uuid.UUID) (*dto.GoalResponse, error)`
- **method** `GoalService.List(ctx context.Context, a services.Actor, f repositories.GoalFilter, page dto.Page) ([]dto.GoalResponse, dto.PageMeta, error)`
- **method** `GoalService.ListMine(ctx context.Context, a services.Actor, cycleID *uuid.UUID, page dto.Page) ([]dto.GoalResponse, dto.PageMeta, error)`
- **method** `GoalService.Update(ctx context.Context, a services.Actor, id uuid.UUID, req dto.UpdateGoalRequest) (*dto.GoalResponse, error)`
- **method** `GoalService.SetStatus(ctx context.Context, a services.Actor, id uuid.UUID, to string) (*dto.GoalResponse, error)`
- **method** `GoalService.AddKeyResult(ctx context.Context, a services.Actor, id uuid.UUID, req dto.KeyResultRequest) (*dto.GoalResponse, error)`
- **method** `GoalService.CheckIn(ctx context.Context, a services.Actor, id uuid.UUID, req dto.CheckInRequest) (*dto.GoalResponse, error)`
- **type** `ReviewService interface` — ReviewService is the V1–V6 surface.
- **method** `ReviewService.Assign(ctx context.Context, a services.Actor, req dto.AssignReviewerRequest) (*dto.ReviewResponse, error)`
- **method** `ReviewService.List(ctx context.Context, tenantID uuid.UUID, f repositories.ReviewFilter, page dto.Page) ([]dto.ReviewResponse, dto.PageMeta, error)`
- **method** `ReviewService.ListAssigned(ctx context.Context, a services.Actor, status string, page dto.Page) ([]dto.ReviewResponse, dto.PageMeta, error)`
- **method** `ReviewService.ListMine(ctx context.Context, a services.Actor, cycleID *uuid.UUID) ([]dto.ReviewResponse, error)`
- **method** `ReviewService.Get(ctx context.Context, a services.Actor, id uuid.UUID) (*dto.ReviewResponse, error)`
- **method** `ReviewService.Submit(ctx context.Context, a services.Actor, id uuid.UUID, req dto.SubmitReviewRequest) (*dto.ReviewResponse, error)`
- **type** `Controller struct` — Controller serves all 33 HR/Performance operations.
- **type** `Services struct` — Services bundles the controller's collaborators.
- **func** `New(s Services) *Controller` — New builds the Controller.
- **func** `invalidQuery(msg string) error` — invalidQuery is a 400 for a bad or missing query parameter.
- **func** `queryUUID(c *gin.Context, name string) (*uuid.UUID, bool)` — queryUUID parses an optional UUID query parameter; malformed → 400.
- **func** `(ctl *Controller) CreateCycle(c *gin.Context)`
- **func** `(ctl *Controller) ListCycles(c *gin.Context)`
- **func** `(ctl *Controller) GetCycle(c *gin.Context)`
- **func** `(ctl *Controller) UpdateCycle(c *gin.Context)`
- **func** `(ctl *Controller) ActivateCycle(c *gin.Context)`
- **func** `(ctl *Controller) LaunchReviews(c *gin.Context)`
- **func** `(ctl *Controller) CompleteCycle(c *gin.Context)`
- **func** `(ctl *Controller) NineBox(c *gin.Context)`
- **func** `(ctl *Controller) CreateGoal(c *gin.Context)`
- **func** `(ctl *Controller) ListGoals(c *gin.Context)`
- **func** `(ctl *Controller) MyGoals(c *gin.Context)`
- **func** `(ctl *Controller) GetGoal(c *gin.Context)`
- **func** `(ctl *Controller) UpdateGoal(c *gin.Context)`
- **func** `(ctl *Controller) SetGoalStatus(c *gin.Context)`
- **func** `(ctl *Controller) AddKeyResult(c *gin.Context)`
- **func** `(ctl *Controller) CheckIn(c *gin.Context)`
- **func** `(ctl *Controller) AssignReviewer(c *gin.Context)`
- **func** `(ctl *Controller) ListReviews(c *gin.Context)`
- **func** `(ctl *Controller) AssignedReviews(c *gin.Context)`
- **func** `(ctl *Controller) MyReviews(c *gin.Context)`
- **func** `(ctl *Controller) GetReview(c *gin.Context)`
- **func** `(ctl *Controller) SubmitReview(c *gin.Context)`

#### `internal/hr/performance/controllers/parity_controllers.go` (151 lines)
- **type** `CalibrationService interface` — CalibrationService is the calibration surface (FR-CA001, FR-CA002).
- **method** `CalibrationService.Calibrate(ctx context.Context, a services.Actor, cycleID uuid.UUID, req dto.CalibrateRequest) (*dto.CalibrationResponse, error)`
- **method** `CalibrationService.List(ctx context.Context, tenantID, cycleID uuid.UUID, employeeID *uuid.UUID, page dto.Page) ([]dto.CalibrationResponse, dto.PageMeta, error)`
- **type** `FeedbackService interface` — FeedbackService is the continuous-feedback surface (F1–F9).
- **method** `FeedbackService.Give(ctx context.Context, a services.Actor, req dto.GiveFeedbackRequest) (*dto.FeedbackResponse, error)`
- **method** `FeedbackService.ListReceived(ctx context.Context, a services.Actor, page dto.Page) ([]dto.FeedbackResponse, dto.PageMeta, error)`
- **method** `FeedbackService.ListGiven(ctx context.Context, a services.Actor, page dto.Page) ([]dto.FeedbackResponse, dto.PageMeta, error)`
- **method** `FeedbackService.ListFor(ctx context.Context, a services.Actor, employeeID uuid.UUID, page dto.Page) ([]dto.FeedbackResponse, dto.PageMeta, error)`
- **method** `FeedbackService.ListPublic(ctx context.Context, tenantID uuid.UUID, page dto.Page) ([]dto.FeedbackResponse, dto.PageMeta, error)`
- **method** `FeedbackService.Request(ctx context.Context, a services.Actor, req dto.RequestFeedbackRequest) ([]dto.FeedbackRequestResponse, error)`
- **method** `FeedbackService.ListAssigned(ctx context.Context, a services.Actor, status string, page dto.Page) ([]dto.FeedbackRequestResponse, dto.PageMeta, error)`
- **method** `FeedbackService.ListMine(ctx context.Context, a services.Actor, status string, page dto.Page) ([]dto.FeedbackRequestResponse, dto.PageMeta, error)`
- **method** `FeedbackService.Decline(ctx context.Context, a services.Actor, id uuid.UUID) (*dto.FeedbackRequestResponse, error)`
- **func** `(ctl *Controller) Calibrate(c *gin.Context)`
- **func** `(ctl *Controller) ListCalibrations(c *gin.Context)`
- **func** `(ctl *Controller) GiveFeedback(c *gin.Context)`
- **func** `(ctl *Controller) FeedbackReceived(c *gin.Context)`
- **func** `(ctl *Controller) FeedbackGiven(c *gin.Context)`
- **func** `(ctl *Controller) FeedbackFor(c *gin.Context)`
- **func** `(ctl *Controller) PublicFeedback(c *gin.Context)`
- **func** `(ctl *Controller) RequestFeedback(c *gin.Context)`
- **func** `(ctl *Controller) AssignedFeedbackRequests(c *gin.Context)`
- **func** `(ctl *Controller) MyFeedbackRequests(c *gin.Context)`
- **func** `(ctl *Controller) DeclineFeedbackRequest(c *gin.Context)`

#### `internal/hr/performance/controllers/respond.go` (139 lines)
_Package:_ Package controllers is HR/Performance's thin HTTP edge: parse → validate → service → envelope.
- **type** `errorBody struct`
- **func** `ok(c *gin.Context, status int, data interface{})`
- **func** `okList(c *gin.Context, data interface{}, meta dto.PageMeta)`
- **func** `fail(c *gin.Context, err error)` — fail maps AppErrors to their status after shared normalization (unique violation → 409, not found → 404); anything else is an opaque 500 (no internals leaked).
- **func** `bindJSON(c *gin.Context, dst interface{}) bool` — bindJSON decodes and validates a body; failures become VALIDATION_ERROR with field details.
- **func** `snake(s string) string`
- **func** `actorFrom(c *gin.Context) (services.Actor, bool)` — actorFrom builds the caller from M0 middleware context only (NFR-SEC002).
- **func** `pathID(c *gin.Context, name string) (uuid.UUID, bool)` — pathID parses a :param UUID; malformed ids are 400.
- **func** `page(c *gin.Context) dto.Page`
- **func** `respond(c *gin.Context, status int, data interface{}, err error)` — respond writes data or the mapped error.
- **func** `respondList(c *gin.Context, data interface{}, meta dto.PageMeta, err error)` — respondList writes a paginated list or the mapped error.
- **func** `pathIDIf(c *gin.Context, actorOK bool) (uuid.UUID, bool)` — pathIDIf parses :id only when the actor resolved (the actor failure already wrote 401).

#### `internal/hr/performance/dto/dto.go` (286 lines)
_Package:_ Package dto holds HR/Performance request and response shapes (specification.md §5).
- **type** `CreateCycleRequest struct` — CreateCycleRequest is C1 (FR-CY001); criteria default when omitted.
- **type** `UpdateCycleRequest struct` — UpdateCycleRequest is C4 (FR-CY002, draft only).
- **type** `CycleResponse struct` — CycleResponse is a cycle.
- **type** `LaunchResponse struct` — LaunchResponse is C6 (PM-010): the cycle plus how many reviews were created.
- **type** `NineBoxEntry struct` — NineBoxEntry is one employee on the 9-box (PM-015, PM-020); Box is "<performance>/<potential>" or "unrated".
- **type** `CalibrateRequest struct` — CalibrateRequest is FR-CA001: at least one of overall_score / potential, always a reason.
- **type** `CalibrationResponse struct` — CalibrationResponse is one calibration row (PM-019).
- **type** `FinalRating struct` — FinalRating is PM-020: the latest calibration, else the latest submitted manager review.
- **type** `GiveFeedbackRequest struct` — GiveFeedbackRequest is F1 (FB-001, FB-005).
- **type** `FeedbackResponse struct` — FeedbackResponse is one piece of feedback.
- **type** `RequestFeedbackRequest struct` — RequestFeedbackRequest is F6 (FB-004).
- **type** `FeedbackRequestResponse struct` — FeedbackRequestResponse is one feedback request.
- **type** `NineBoxResponse struct` — NineBoxResponse is C8.
- **type** `CreateGoalRequest struct` — CreateGoalRequest is G1 (FR-GL001); employee_id defaults to the caller's profile.
- **type** `UpdateGoalRequest struct` — UpdateGoalRequest is G5 (FR-GL003).
- **type** `GoalStatusRequest struct` — GoalStatusRequest is G6 (FR-GL004).
- **type** `KeyResultRequest struct` — KeyResultRequest is G7 (FR-GL005).
- **type** `CheckInRequest struct` — CheckInRequest is G8 (FR-GL006): progress for a goal without key results, key_result_id + value otherwise.
- **type** `KeyResultResponse struct` — KeyResultResponse is a key result with its derived progress (PM-006).
- **type** `CheckInResponse struct` — CheckInResponse is one progress update (PM-007).
- **type** `GoalResponse struct` — GoalResponse is a goal; detail reads add key results and the newest check-ins.
- **type** `AssignReviewerRequest struct` — AssignReviewerRequest is V1 (FR-RV001).
- **type** `SubmitReviewRequest struct` — SubmitReviewRequest is V6 (FR-RV003).
- **type** `ReviewResponse struct` — ReviewResponse is a review; reviewer_employee_id is omitted for peer reviews shown to the subject (PM-014).
- **type** `PageMeta struct` — PageMeta is list metadata.
- **type** `Page struct{ Page, PerPage int }` — Page is a clamped page request (G14).
- **func** `ParsePage(pageStr, perPageStr string) Page` — ParsePage clamps raw query values (page ≥ 1 default 1, per_page 1–100 default 20).
- **func** `(p Page) Offset() int { return (p.Page - 1) * p.PerPage }` — Offset is the SQL offset.
- **func** `(p Page) Meta(total int64) PageMeta` — Meta builds list metadata.

#### `internal/hr/performance/events/events.go` (73 lines)
_Package:_ Package events defines HR/Performance event types and the shared envelope (FR-EV001).
- **type** `Envelope struct` — Envelope is the platform event envelope (same shape as the other HR contexts).
- **type** `Source struct` — Source identifies who and when an event came from.
- **func** `New(routingKey string, src Source, payload interface{}) Envelope` — New wraps a payload in the envelope.
- **type** `Payload struct` — Payload carries ids, statuses and progress only — never ratings, scores, potential or review text (FR-EV002, PM-017).
- **type** `FeedbackPayload struct` — FeedbackPayload carries ids, kind and visibility only — never the message or the question (FB-006).

#### `internal/hr/performance/lifecycle/lifecycle.go` (51 lines)
_Package:_ Package lifecycle is Module 7's pure state machines: appraisal cycles (PM-002) and goals (FR-GL004).
- **func** `CanTransitionCycle(from, to string) bool { n, ok := cycleNext[from]; return ok && n == to }` — CanTransitionCycle reports the only forward step allowed from → to.
- **func** `CanTransitionGoal(from, to string) bool` — CanTransitionGoal reports a legal goal status change.
- **func** `ValidCycleStatus(s string) bool` — ValidCycleStatus reports a known cycle status (list filters).
- **func** `CycleEditable(s string) bool { return s == CycleDraft }` — CycleEditable: title, dates and criteria change only in draft (PM-001).
- **func** `GoalsOpen(s string) bool { return s == CycleDraft || s == CycleActive }` — GoalsOpen: goals are created, edited, re-weighted and status-changed in draft or active (PM-003).
- **func** `CheckInsOpen(s string) bool { return s == CycleActive || s == CycleReview }` — CheckInsOpen: progress check-ins in active or review (PM-007).
- **func** `ReviewsOpen(s string) bool { return s == CycleReview }` — ReviewsOpen: reviewers are assigned and reviews submitted only in review (PM-011, PM-012).

#### `internal/hr/performance/models/models.go` (283 lines)
_Package:_ Package models holds HR/Performance persistence models; migrations/hr 000038–000046 are the SQL twin.
- **type** `Criteria []string` — Criteria is a cycle's ordered rating criteria, stored as a JSON array (PM-001).
- **func** `(c Criteria) Value() (driver.Value, error) { return marshalJSON(c) }` — Value encodes the criteria as JSON.
- **func** `(c *Criteria) Scan(src interface{}) error { return unmarshalJSON(src, c) }` — Scan decodes the criteria from JSON.
- **type** `Rating struct` — Rating is one criterion score in a review (PM-012).
- **type** `Ratings []Rating` — Ratings is a review's scores, stored as a JSON array.
- **func** `(r Ratings) Value() (driver.Value, error) { return marshalJSON(r) }` — Value encodes the ratings as JSON.
- **func** `(r *Ratings) Scan(src interface{}) error { return unmarshalJSON(src, r) }` — Scan decodes the ratings from JSON.
- **func** `marshalJSON(v interface{}) (driver.Value, error)`
- **func** `unmarshalJSON(src interface{}, dst interface{}) error`
- **type** `Cycle struct` — Cycle is an appraisal cycle (FR-CY).
- **func** `(Cycle) TableName() string { return "performance_cycles" }` — TableName maps to performance_cycles (migration 000051).
- **func** `(c *Cycle) BeforeCreate(tx *gorm.DB) error { return assignID(&c.ID) }` — BeforeCreate assigns a UUID v4 when unset.
- **type** `Goal struct` — Goal is one employee objective in a cycle (FR-GL).
- **func** `(Goal) TableName() string { return "performance_goals" }` — TableName maps to performance_goals (migration 000052).
- **func** `(g *Goal) BeforeCreate(tx *gorm.DB) error { return assignID(&g.ID) }` — BeforeCreate assigns a UUID v4 when unset.
- **type** `KeyResult struct` — KeyResult is a measurable target of a goal (FR-GL005).
- **func** `(KeyResult) TableName() string { return "performance_key_results" }` — TableName maps to performance_key_results (migration 000053).
- **func** `(k *KeyResult) BeforeCreate(tx *gorm.DB) error { return assignID(&k.ID) }` — BeforeCreate assigns a UUID v4 when unset.
- **type** `CheckIn struct` — CheckIn is one append-only progress update (PM-007).
- **func** `(CheckIn) TableName() string { return "performance_checkins" }` — TableName maps to performance_checkins (migration 000054).
- **func** `(c *CheckIn) BeforeCreate(tx *gorm.DB) error { return assignID(&c.ID) }` — BeforeCreate assigns a UUID v4 when unset.
- **type** `Review struct` — Review is one reviewer's assessment of one employee in one cycle (FR-RV).
- **func** `(Review) TableName() string { return "performance_reviews" }` — TableName maps to performance_reviews (migration 000055).
- **func** `(r *Review) BeforeCreate(tx *gorm.DB) error { return assignID(&r.ID) }` — BeforeCreate assigns a UUID v4 when unset.
- **type** `Calibration struct` — Calibration is one append-only rating override next to the manager review (PM-018..PM-022).
- **func** `(Calibration) TableName() string { return "performance_calibrations" }` — TableName maps to performance_calibrations (migration hr/000044).
- **func** `(c *Calibration) BeforeCreate(tx *gorm.DB) error { return assignID(&c.ID) }` — BeforeCreate assigns a UUID v4 when unset.
- **type** `Feedback struct` — Feedback is one immutable piece of continuous feedback (FB-001..FB-003).
- **func** `(Feedback) TableName() string { return "performance_feedback" }` — TableName maps to performance_feedback (migration hr/000046).
- **func** `(f *Feedback) BeforeCreate(tx *gorm.DB) error { return assignID(&f.ID) }` — BeforeCreate assigns a UUID v4 when unset.
- **type** `FeedbackRequest struct` — FeedbackRequest asks a colleague for feedback about the requester or a direct report (FB-004, FB-005).
- **func** `(FeedbackRequest) TableName() string { return "performance_feedback_requests" }` — TableName maps to performance_feedback_requests (migration hr/000045).
- **func** `(r *FeedbackRequest) BeforeCreate(tx *gorm.DB) error { return assignID(&r.ID) }` — BeforeCreate assigns a UUID v4 when unset.
- **type** `OutboxEvent struct` — OutboxEvent is a transactional-outbox row (FR-EV001).
- **func** `(OutboxEvent) TableName() string { return "performance_events_outbox" }` — TableName maps to performance_events_outbox (migration 000056).
- **func** `(o *OutboxEvent) BeforeCreate(tx *gorm.DB) error { return assignID(&o.ID) }` — BeforeCreate assigns a UUID v4 when unset.
- **func** `assignID(id *uuid.UUID) error`

#### `internal/hr/performance/repositories/interfaces.go` (138 lines)
_Package:_ Package repositories is HR/Performance data access: tenant_id on every scoped query, `db`/`tx` passed per call.
- **type** `CycleRepository interface` — CycleRepository persists appraisal cycles (FR-CY).
- **method** `CycleRepository.Create(ctx context.Context, tx *gorm.DB, c *models.Cycle) error`
- **method** `CycleRepository.Update(ctx context.Context, tx *gorm.DB, c *models.Cycle) error`
- **method** `CycleRepository.FindByID(ctx context.Context, db *gorm.DB, tenantID, id uuid.UUID) (*models.Cycle, error)`
- **method** `CycleRepository.LockByID(ctx context.Context, tx *gorm.DB, tenantID, id uuid.UUID) (*models.Cycle, error)`
- **method** `CycleRepository.List(ctx context.Context, db *gorm.DB, tenantID uuid.UUID, status string, page dto.Page) ([]models.Cycle, int64, error)`
- **type** `GoalFilter struct` — GoalFilter narrows goal lists (G2); EmployeeID nil means any owner.
- **type** `GoalRepository interface` — GoalRepository persists goals, their key results and append-only check-ins (FR-GL, PM-007).
- **method** `GoalRepository.Create(ctx context.Context, tx *gorm.DB, g *models.Goal) error`
- **method** `GoalRepository.Update(ctx context.Context, tx *gorm.DB, g *models.Goal) error`
- **method** `GoalRepository.FindByID(ctx context.Context, db *gorm.DB, tenantID, id uuid.UUID) (*models.Goal, error)`
- **method** `GoalRepository.LockByID(ctx context.Context, tx *gorm.DB, tenantID, id uuid.UUID) (*models.Goal, error)`
- **method** `GoalRepository.List(ctx context.Context, db *gorm.DB, tenantID uuid.UUID, f GoalFilter, page dto.Page) ([]models.Goal, int64, error)`
- **method** `GoalRepository.LockOwner(ctx context.Context, tx *gorm.DB, cycleID, employeeID uuid.UUID) error` — LockOwner takes the transaction-scoped advisory lock for (cycle, owner) that serializes the weight cap (PM-004).
- **method** `GoalRepository.WeightSum(ctx context.Context, tx *gorm.DB, tenantID, cycleID, employeeID, exclude uuid.UUID) (score.Fixed, error)` — WeightSum sums the owner's non-cancelled goal weights in the cycle, excluding one goal (uuid.Nil: none).
- **method** `GoalRepository.Open(ctx context.Context, db *gorm.DB, tenantID, cycleID uuid.UUID) ([]models.Goal, error)` — Open lists the cycle's non-cancelled goals (launch reviews, 9-box goal scores).
- **method** `GoalRepository.AddKeyResult(ctx context.Context, tx *gorm.DB, k *models.KeyResult) error`
- **method** `GoalRepository.UpdateKeyResult(ctx context.Context, tx *gorm.DB, k *models.KeyResult) error`
- **method** `GoalRepository.KeyResults(ctx context.Context, db *gorm.DB, tenantID, goalID uuid.UUID) ([]models.KeyResult, error)`
- **method** `GoalRepository.AddCheckIn(ctx context.Context, tx *gorm.DB, c *models.CheckIn) error`
- **method** `GoalRepository.CheckIns(ctx context.Context, db *gorm.DB, tenantID, goalID uuid.UUID, limit int) ([]models.CheckIn, error)` — CheckIns returns the goal's newest check-ins first, at most limit.
- **type** `ReviewFilter struct` — ReviewFilter narrows review lists (V2–V4).
- **type** `ReviewRepository interface` — ReviewRepository persists reviews (FR-RV).
- **method** `ReviewRepository.Create(ctx context.Context, tx *gorm.DB, r *models.Review) error`
- **method** `ReviewRepository.Update(ctx context.Context, tx *gorm.DB, r *models.Review) error`
- **method** `ReviewRepository.FindByID(ctx context.Context, db *gorm.DB, tenantID, id uuid.UUID) (*models.Review, error)`
- **method** `ReviewRepository.LockByID(ctx context.Context, tx *gorm.DB, tenantID, id uuid.UUID) (*models.Review, error)`
- **method** `ReviewRepository.Exists(ctx context.Context, tx *gorm.DB, tenantID, cycleID, employeeID, reviewerID uuid.UUID, typ string) (bool, error)` — Exists reports whether the (cycle, employee, reviewer, type) assignment exists (PM-010, PM-011).
- **method** `ReviewRepository.List(ctx context.Context, db *gorm.DB, tenantID uuid.UUID, f ReviewFilter, page dto.Page) ([]models.Review, int64, error)`
- **method** `ReviewRepository.ExpirePending(ctx context.Context, tx *gorm.DB, tenantID, cycleID uuid.UUID, at time.Time) (int64, error)` — ExpirePending marks the cycle's pending reviews expired (PM-013).
- **type** `CalibrationRepository interface` — CalibrationRepository persists append-only calibrations (PM-019).
- **method** `CalibrationRepository.Create(ctx context.Context, tx *gorm.DB, c *models.Calibration) error`
- **method** `CalibrationRepository.Latest(ctx context.Context, db *gorm.DB, tenantID, cycleID, employeeID uuid.UUID) (*models.Calibration, error)` — Latest returns the employee's newest calibration in the cycle, or nil.
- **method** `CalibrationRepository.LatestPerEmployee(ctx context.Context, db *gorm.DB, tenantID, cycleID uuid.UUID) ([]models.Calibration, error)` — LatestPerEmployee returns each employee's newest calibration in the cycle.
- **method** `CalibrationRepository.List(ctx context.Context, db *gorm.DB, tenantID, cycleID uuid.UUID, employeeID *uuid.UUID, page dto.Page) ([]models.Calibration, int64, error)`
- **type** `FeedbackFilter struct` — FeedbackFilter narrows feedback lists (F2–F5).
- **type** `RequestFilter struct` — RequestFilter narrows feedback-request lists (F7, F8).
- **type** `FeedbackRepository interface` — FeedbackRepository persists continuous feedback and feedback requests (FB-001..FB-005).
- **method** `FeedbackRepository.Create(ctx context.Context, tx *gorm.DB, f *models.Feedback) error`
- **method** `FeedbackRepository.List(ctx context.Context, db *gorm.DB, tenantID uuid.UUID, f FeedbackFilter, page dto.Page) ([]models.Feedback, int64, error)`
- **method** `FeedbackRepository.CreateRequest(ctx context.Context, tx *gorm.DB, r *models.FeedbackRequest) error`
- **method** `FeedbackRepository.UpdateRequest(ctx context.Context, tx *gorm.DB, r *models.FeedbackRequest) error`
- **method** `FeedbackRepository.LockRequest(ctx context.Context, tx *gorm.DB, tenantID, id uuid.UUID) (*models.FeedbackRequest, error)`
- **method** `FeedbackRepository.PendingExists(ctx context.Context, tx *gorm.DB, tenantID, requester, responder, about uuid.UUID) (bool, error)` — PendingExists reports a pending request for (requester, responder, about) (FB-004).
- **method** `FeedbackRepository.ListRequests(ctx context.Context, db *gorm.DB, tenantID uuid.UUID, f RequestFilter, page dto.Page) ([]models.FeedbackRequest, int64, error)`
- **type** `OutboxRepository interface` — OutboxRepository persists outbox rows; relay methods are cross-tenant by design.
- **method** `OutboxRepository.Create(ctx context.Context, tx *gorm.DB, e *models.OutboxEvent) error`
- **method** `OutboxRepository.MarkPublished(ctx context.Context, db *gorm.DB, id uuid.UUID, at time.Time) error`
- **method** `OutboxRepository.RecordFailure(ctx context.Context, db *gorm.DB, id uuid.UUID, reason string) error`
- **method** `OutboxRepository.FetchUnpublished(ctx context.Context, db *gorm.DB, limit, maxAttempts int) ([]models.OutboxEvent, error)`
- **func** `first[T any](q *gorm.DB) (*T, error)`
- **func** `paged[T any](q *gorm.DB, order string, page dto.Page) ([]T, int64, error)`

#### `internal/hr/performance/repositories/repositories.go` (253 lines)
- **type** `cycleRepository struct{}`
- **func** `NewCycleRepository() CycleRepository { return &cycleRepository{} }` — NewCycleRepository builds a CycleRepository.
- **func** `(r *cycleRepository) Create(ctx context.Context, tx *gorm.DB, c *models.Cycle) error`
- **func** `(r *cycleRepository) Update(ctx context.Context, tx *gorm.DB, c *models.Cycle) error`
- **func** `(r *cycleRepository) FindByID(ctx context.Context, db *gorm.DB, tenantID, id uuid.UUID) (*models.Cycle, error)`
- **func** `(r *cycleRepository) LockByID(ctx context.Context, tx *gorm.DB, tenantID, id uuid.UUID) (*models.Cycle, error)`
- **func** `(r *cycleRepository) List(ctx context.Context, db *gorm.DB, tenantID uuid.UUID, status string, page dto.Page) ([]models.Cycle, int64, error)`
- **type** `goalRepository struct{}`
- **func** `NewGoalRepository() GoalRepository { return &goalRepository{} }` — NewGoalRepository builds a GoalRepository.
- **func** `(r *goalRepository) Create(ctx context.Context, tx *gorm.DB, g *models.Goal) error`
- **func** `(r *goalRepository) Update(ctx context.Context, tx *gorm.DB, g *models.Goal) error`
- **func** `(r *goalRepository) FindByID(ctx context.Context, db *gorm.DB, tenantID, id uuid.UUID) (*models.Goal, error)`
- **func** `(r *goalRepository) LockByID(ctx context.Context, tx *gorm.DB, tenantID, id uuid.UUID) (*models.Goal, error)`
- **func** `(r *goalRepository) List(ctx context.Context, db *gorm.DB, tenantID uuid.UUID, f GoalFilter, page dto.Page) ([]models.Goal, int64, error)`
- **func** `(r *goalRepository) LockOwner(ctx context.Context, tx *gorm.DB, cycleID, employeeID uuid.UUID) error`
- **func** `(r *goalRepository) WeightSum(ctx context.Context, tx *gorm.DB, tenantID, cycleID, employeeID, exclude uuid.UUID) (score.Fixed, error)`
- **func** `(r *goalRepository) Open(ctx context.Context, db *gorm.DB, tenantID, cycleID uuid.UUID) ([]models.Goal, error)`
- **func** `(r *goalRepository) AddKeyResult(ctx context.Context, tx *gorm.DB, k *models.KeyResult) error`
- **func** `(r *goalRepository) UpdateKeyResult(ctx context.Context, tx *gorm.DB, k *models.KeyResult) error`
- **func** `(r *goalRepository) KeyResults(ctx context.Context, db *gorm.DB, tenantID, goalID uuid.UUID) ([]models.KeyResult, error)`
- **func** `(r *goalRepository) AddCheckIn(ctx context.Context, tx *gorm.DB, c *models.CheckIn) error`
- **func** `(r *goalRepository) CheckIns(ctx context.Context, db *gorm.DB, tenantID, goalID uuid.UUID, limit int) ([]models.CheckIn, error)`
- **type** `reviewRepository struct{}`
- **func** `NewReviewRepository() ReviewRepository { return &reviewRepository{} }` — NewReviewRepository builds a ReviewRepository.
- **func** `(r *reviewRepository) Create(ctx context.Context, tx *gorm.DB, rv *models.Review) error`
- **func** `(r *reviewRepository) Update(ctx context.Context, tx *gorm.DB, rv *models.Review) error`
- **func** `(r *reviewRepository) FindByID(ctx context.Context, db *gorm.DB, tenantID, id uuid.UUID) (*models.Review, error)`
- **func** `(r *reviewRepository) LockByID(ctx context.Context, tx *gorm.DB, tenantID, id uuid.UUID) (*models.Review, error)`
- **func** `(r *reviewRepository) Exists(ctx context.Context, tx *gorm.DB, tenantID, cycleID, employeeID, reviewerID uuid.UUID, typ string) (bool, error)`
- **func** `(r *reviewRepository) List(ctx context.Context, db *gorm.DB, tenantID uuid.UUID, f ReviewFilter, page dto.Page) ([]models.Review, int64, error)`
- **func** `(r *reviewRepository) ExpirePending(ctx context.Context, tx *gorm.DB, tenantID, cycleID uuid.UUID, at time.Time) (int64, error)`
- **type** `outboxRepository struct{}`
- **func** `NewOutboxRepository() OutboxRepository { return &outboxRepository{} }` — NewOutboxRepository builds an OutboxRepository.
- **func** `(r *outboxRepository) Create(ctx context.Context, tx *gorm.DB, e *models.OutboxEvent) error`
- **func** `(r *outboxRepository) MarkPublished(ctx context.Context, db *gorm.DB, id uuid.UUID, at time.Time) error`
- **func** `(r *outboxRepository) RecordFailure(ctx context.Context, db *gorm.DB, id uuid.UUID, reason string) error`
- **func** `(r *outboxRepository) FetchUnpublished(ctx context.Context, db *gorm.DB, limit, maxAttempts int) ([]models.OutboxEvent, error)`
- **type** `calibrationRepository struct{}`
- **func** `NewCalibrationRepository() CalibrationRepository { return &calibrationRepository{} }` — NewCalibrationRepository builds a CalibrationRepository.
- **func** `(r *calibrationRepository) Create(ctx context.Context, tx *gorm.DB, c *models.Calibration) error`
- **func** `(r *calibrationRepository) Latest(ctx context.Context, db *gorm.DB, tenantID, cycleID, employeeID uuid.UUID) (*models.Calibration, error)`
- **func** `(r *calibrationRepository) LatestPerEmployee(ctx context.Context, db *gorm.DB, tenantID, cycleID uuid.UUID) ([]models.Calibration, error)`
- **func** `(r *calibrationRepository) List(ctx context.Context, db *gorm.DB, tenantID, cycleID uuid.UUID, employeeID *uuid.UUID, page dto.Page) ([]models.Calibration, int64, error)`
- **type** `feedbackRepository struct{}`
- **func** `NewFeedbackRepository() FeedbackRepository { return &feedbackRepository{} }` — NewFeedbackRepository builds a FeedbackRepository.
- **func** `(r *feedbackRepository) Create(ctx context.Context, tx *gorm.DB, f *models.Feedback) error`
- **func** `(r *feedbackRepository) List(ctx context.Context, db *gorm.DB, tenantID uuid.UUID, f FeedbackFilter, page dto.Page) ([]models.Feedback, int64, error)`
- **func** `(r *feedbackRepository) CreateRequest(ctx context.Context, tx *gorm.DB, fr *models.FeedbackRequest) error`
- **func** `(r *feedbackRepository) UpdateRequest(ctx context.Context, tx *gorm.DB, fr *models.FeedbackRequest) error`
- **func** `(r *feedbackRepository) LockRequest(ctx context.Context, tx *gorm.DB, tenantID, id uuid.UUID) (*models.FeedbackRequest, error)`
- **func** `(r *feedbackRepository) PendingExists(ctx context.Context, tx *gorm.DB, tenantID, requester, responder, about uuid.UUID) (bool, error)`
- **func** `(r *feedbackRepository) ListRequests(ctx context.Context, db *gorm.DB, tenantID uuid.UUID, f RequestFilter, page dto.Page) ([]models.FeedbackRequest, int64, error)`

#### `internal/hr/performance/routes/routes.go` (67 lines)
_Package:_ Package routes mounts HR/Performance endpoints (specification.md §5) behind M0 middleware.
- **type** `Middleware struct` — Middleware is the M0 middleware HR/Performance depends on.
- **func** `RegisterRoutes(rg *gin.RouterGroup, mw Middleware, ctl *controllers.Controller)` — RegisterRoutes mounts all 33 operations. Self, manager and reviewer access is decided per request (PM-008, D7-05), so those routes only authenticate and carry the read/manage flags.

#### `internal/hr/performance/score/score.go` (179 lines)
_Package:_ Package score is Module 7's pure, exact arithmetic (D7-02): hundredths fixed point, key-result and goal progress, weighted goal score, review overall score and 9-box bands. No floats are used for any stored or compared value.
- **type** `Fixed int64` — Fixed is a decimal with exactly two fractional digits, stored as hundredths (12.5 = 1250).
- **func** `FromInt(n int) Fixed { return Fixed(n) * 100 }` — FromInt converts a whole number.
- **func** `Parse(s string) (Fixed, error)` — Parse reads "1", "1.5", "-2.25"; more than 2 decimals, exponents or out-of-range values are errors.
- **func** `digits(s string) bool`
- **func** `(v Fixed) String() string` — String renders two decimals ("1.50", "-0.05").
- **func** `(v Fixed) Float() float64 { return float64(v) / 100 }` — Float is for display only — never for arithmetic.
- **func** `(v Fixed) Value() (driver.Value, error) { return v.String(), nil }` — Value stores an exact decimal string for NUMERIC columns.
- **func** `(v *Fixed) Scan(src interface{}) error` — Scan reads NUMERIC (string/[]byte), float64 or int64.
- **func** `(v *Fixed) parseInto(s string) error`
- **func** `(v Fixed) MarshalJSON() ([]byte, error) { return []byte(v.String()), nil }` — MarshalJSON emits a JSON number with 2 decimals.
- **func** `(v *Fixed) UnmarshalJSON(b []byte) error { return v.parseInto(strings.Trim(string(b), `"`)) }` — UnmarshalJSON accepts a JSON number or numeric string with at most 2 decimals.
- **func** `divRound(n, d int64) int64` — divRound divides with rounding half away from zero; a zero divisor yields 0.
- **func** `KRProgress(start, target, current Fixed) Fixed` — KRProgress is PM-006: clamp((current − start) / (target − start) × 100, 0, 100) in hundredths. A degenerate key result (target = start) yields 0; validators reject it before it is stored.
- **func** `Mean(values []Fixed) Fixed` — Mean is the rounded arithmetic mean (goal progress from key results, PM-006); empty → 0.
- **func** `WeightedScore(weights, progress []Fixed) Fixed` — WeightedScore is PM-009: Σ(weight × progress) / Σ weight, rounded; empty or zero weight → 0.
- **func** `Overall(ratings []int) Fixed` — Overall is PM-012: the mean of integer ratings as a 2-decimal score; empty → 0.
- **func** `Band(overall Fixed) string` — Band maps an overall score to its 9-box performance band: < 2.50 low, < 3.50 medium, else high.

#### `internal/hr/performance/services/calibration_service.go` (152 lines)
- **type** `CalibrationService struct{ base }` — CalibrationService implements calibration (PM-018..PM-022).
- **func** `NewCalibrationService(d Deps) *CalibrationService { return &CalibrationService{base{d}} }` — NewCalibrationService builds the service.
- **func** `(s *CalibrationService) Calibrate(ctx context.Context, a Actor, cycleID uuid.UUID, req dto.CalibrateRequest) (*dto.CalibrationResponse, error)` — Calibrate is FR-CA001: an append-only override of the employee's final rating while the cycle is in review.
- **func** `(s *CalibrationService) List(ctx context.Context, tenantID, cycleID uuid.UUID, employeeID *uuid.UUID, page dto.Page) ([]dto.CalibrationResponse, dto.PageMeta, error)` — List is FR-CA002: the cycle's calibrations, newest first.
- **func** `(b base) finalRating(ctx context.Context, db *gorm.DB, tenantID, cycleID, employeeID uuid.UUID) (*dto.FinalRating, error)` — finalRating is PM-020 for one employee: the latest calibration, else the latest submitted manager review; nil when neither exists.
- **func** `latestReviews(reviews []models.Review) map[uuid.UUID]*models.Review` — latestReviews keeps each employee's most recently submitted review.
- **func** `managerRating(r *models.Review) *dto.FinalRating`
- **func** `calibrationResponse(c *models.Calibration) dto.CalibrationResponse`

#### `internal/hr/performance/services/cycle_service.go` (361 lines)
- **type** `CycleService struct{ base }` — CycleService implements FR-CY (C1–C8).
- **func** `NewCycleService(d Deps) *CycleService { return &CycleService{base{d}} }` — NewCycleService builds the service.
- **func** `(s *CycleService) Create(ctx context.Context, a Actor, req dto.CreateCycleRequest) (*dto.CycleResponse, error)` — Create is C1 (FR-CY001, PM-001).
- **func** `(s *CycleService) Get(ctx context.Context, tenantID, id uuid.UUID) (*dto.CycleResponse, error)` — Get is C3.
- **func** `(s *CycleService) List(ctx context.Context, tenantID uuid.UUID, status string, page dto.Page) ([]dto.CycleResponse, dto.PageMeta, error)` — List is C2.
- **func** `(s *CycleService) Update(ctx context.Context, a Actor, id uuid.UUID, req dto.UpdateCycleRequest) (*dto.CycleResponse, error)` — Update is C4: title, dates and criteria, draft only (FR-CY002, PM-001).
- **func** `applyCycleUpdate(c *models.Cycle, req dto.UpdateCycleRequest) error`
- **func** `(s *CycleService) Activate(ctx context.Context, a Actor, id uuid.UUID) (*dto.CycleResponse, error)` — Activate is C5: draft → active (FR-CY003).
- **func** `(s *CycleService) Complete(ctx context.Context, a Actor, id uuid.UUID) (*dto.CycleResponse, error)` — Complete is C7: review → completed; pending reviews expire (FR-CY005, PM-013).
- **func** `(s *CycleService) transition(ctx context.Context, a Actor, id uuid.UUID, to, event string, extra func(*gorm.DB, *models.Cycle) error) (*dto.CycleResponse, error)`
- **func** `(s *CycleService) stamp(c *models.Cycle, status string)` — stamp moves the cycle to status and records when.
- **func** `(s *CycleService) LaunchReviews(ctx context.Context, a Actor, id uuid.UUID) (*dto.LaunchResponse, error)` — LaunchReviews is C6 (FR-CY004, PM-010): active → review with a self review per goal owner and a manager review where the owner's manager has a profile. Idempotent per assignment: on a cycle already in review it only creates what is missing (EC-06: a second launch creates 0).
- **func** `owners(goals []models.Goal) []uuid.UUID`
- **func** `(s *CycleService) launchFor(ctx context.Context, tx *gorm.DB, c *models.Cycle, owner uuid.UUID, out *dto.LaunchResponse) error`
- **func** `(s *CycleService) ensureReview(ctx context.Context, tx *gorm.DB, c *models.Cycle, employee, reviewer uuid.UUID, typ string) (bool, error)`
- **func** `(s *CycleService) NineBox(ctx context.Context, tenantID, id uuid.UUID) (*dto.NineBoxResponse, error)` — NineBox is C8 (PM-015, PM-020, D7-04): performance band from the final rating (latest calibration, else latest submitted manager review) × its potential, with the goal score (PM-009) alongside; cycles in review or completed only.
- **func** `finalRatings(managerReviews []models.Review, cals []models.Calibration) map[uuid.UUID]*dto.FinalRating` — finalRatings is PM-020 for a whole cycle: each employee's latest calibration, else latest submitted manager review.
- **func** `nineBox(cycleID uuid.UUID, goals []models.Goal, finals map[uuid.UUID]*dto.FinalRating) *dto.NineBoxResponse`
- **func** `(s *CycleService) lock(ctx context.Context, tx *gorm.DB, tenantID, id uuid.UUID) (*models.Cycle, error)`

#### `internal/hr/performance/services/errors.go` (33 lines)
- **func** `appErr(status int, code, msg string) *sharedErrors.AppError`
- **func** `invalid(field, msg string) *sharedErrors.AppError` — invalid builds a 400 VALIDATION_ERROR naming the offending field.

#### `internal/hr/performance/services/feedback_service.go` (325 lines)
- **type** `FeedbackService struct{ base }` — FeedbackService implements continuous feedback (F1–F9, FB-001..FB-006).
- **func** `NewFeedbackService(d Deps) *FeedbackService { return &FeedbackService{base{d}} }` — NewFeedbackService builds the service.
- **func** `(s *FeedbackService) Give(ctx context.Context, a Actor, req dto.GiveFeedbackRequest) (*dto.FeedbackResponse, error)` — Give is F1: feedback to a colleague, optionally answering a request (FB-001, FB-005).
- **func** `(s *FeedbackService) answer(ctx context.Context, tx *gorm.DB, a Actor, self, requestID uuid.UUID, f *models.Feedback) error` — answer is FB-005: the caller answers their pending request about the recipient.
- **func** `(s *FeedbackService) ListReceived(ctx context.Context, a Actor, page dto.Page) ([]dto.FeedbackResponse, dto.PageMeta, error)` — ListReceived is F2.
- **func** `(s *FeedbackService) ListGiven(ctx context.Context, a Actor, page dto.Page) ([]dto.FeedbackResponse, dto.PageMeta, error)` — ListGiven is F3.
- **func** `(s *FeedbackService) ListFor(ctx context.Context, a Actor, employeeID uuid.UUID, page dto.Page) ([]dto.FeedbackResponse, dto.PageMeta, error)` — ListFor is F4: feedback an employee received, for their direct manager or performance:read (FB-002).
- **func** `(s *FeedbackService) ListPublic(ctx context.Context, tenantID uuid.UUID, page dto.Page) ([]dto.FeedbackResponse, dto.PageMeta, error)` — ListPublic is F5: the tenant's praise wall.
- **func** `(s *FeedbackService) list(ctx context.Context, tenantID uuid.UUID, f repositories.FeedbackFilter, page dto.Page) ([]dto.FeedbackResponse, dto.PageMeta, error)`
- **func** `(s *FeedbackService) Request(ctx context.Context, a Actor, req dto.RequestFeedbackRequest) ([]dto.FeedbackRequestResponse, error)` — Request is F6 (FB-004): ask 1–10 working colleagues for feedback about yourself or a direct report.
- **func** `(s *FeedbackService) subject(ctx context.Context, a Actor, self *employeeDTO.EmployeeResponse, raw *string) (uuid.UUID, error)` — subject resolves about_employee_id: the requester, or one of their direct reports (else 404).
- **func** `(s *FeedbackService) responders(ctx context.Context, tenantID, requester, about uuid.UUID, raw []string) ([]uuid.UUID, error)` — responders validates FB-004: 1–10 distinct working colleagues, never the requester or the subject.
- **func** `(s *FeedbackService) ListAssigned(ctx context.Context, a Actor, status string, page dto.Page) ([]dto.FeedbackRequestResponse, dto.PageMeta, error)` — ListAssigned is F7: requests the caller must answer.
- **func** `(s *FeedbackService) ListMine(ctx context.Context, a Actor, status string, page dto.Page) ([]dto.FeedbackRequestResponse, dto.PageMeta, error)` — ListMine is F8: requests the caller made.
- **func** `(s *FeedbackService) listRequests(ctx context.Context, a Actor, status string, page dto.Page, by func(uuid.UUID) repositories.RequestFilter) ([]dto.FeedbackRequestResponse, dto.PageMeta, error)`
- **func** `(s *FeedbackService) Decline(ctx context.Context, a Actor, id uuid.UUID) (*dto.FeedbackRequestResponse, error)` — Decline is F9: the responder declines a pending request.
- **func** `(s *FeedbackService) workingSelf(ctx context.Context, a Actor) (*employeeDTO.EmployeeResponse, error)` — workingSelf is the caller's profile, which must be working (FB-001).
- **func** `(s *FeedbackService) working(ctx context.Context, tenantID, id uuid.UUID) error` — working checks that an employee of the tenant exists and is working.
- **func** `feedbackResponse(f *models.Feedback) dto.FeedbackResponse`
- **func** `requestResponse(r *models.FeedbackRequest) dto.FeedbackRequestResponse`

#### `internal/hr/performance/services/goal_service.go` (450 lines)
- **type** `GoalService struct{ base }` — GoalService implements FR-GL (G1–G8).
- **func** `NewGoalService(d Deps) *GoalService { return &GoalService{base{d}} }` — NewGoalService builds the service.
- **func** `(s *GoalService) Create(ctx context.Context, a Actor, req dto.CreateGoalRequest) (*dto.GoalResponse, error)` — Create is G1 (FR-GL001, PM-003..PM-005, PM-008).
- **type** `goalInput struct`
- **func** `(s *GoalService) parseCreate(ctx context.Context, a Actor, req dto.CreateGoalRequest) (*goalInput, error)`
- **func** `(s *GoalService) openCycle(ctx context.Context, tx *gorm.DB, tenantID, id uuid.UUID, open func(string) bool) error` — openCycle locks the cycle and checks that its status allows the action.
- **func** `(s *GoalService) checkParent(ctx context.Context, tx *gorm.DB, tenantID, cycleID uuid.UUID, parent *uuid.UUID) error` — checkParent is PM-005: same tenant and cycle, not cancelled, chain depth ≤ 3.
- **func** `(s *GoalService) checkWeight(ctx context.Context, tx *gorm.DB, g *models.Goal, weight score.Fixed) error` — checkWeight is PM-004 under the (cycle, owner) advisory lock.
- **func** `(s *GoalService) Get(ctx context.Context, a Actor, id uuid.UUID) (*dto.GoalResponse, error)` — Get is G4 with key results and the newest check-ins (PM-008 read).
- **func** `(s *GoalService) detail(ctx context.Context, g *models.Goal) (*dto.GoalResponse, error)`
- **func** `(s *GoalService) List(ctx context.Context, a Actor, f repositories.GoalFilter, page dto.Page) ([]dto.GoalResponse, dto.PageMeta, error)` — List is G2: any filter with performance:read; otherwise own goals or a direct report's (D7-05).
- **func** `(s *GoalService) ListMine(ctx context.Context, a Actor, cycleID *uuid.UUID, page dto.Page) ([]dto.GoalResponse, dto.PageMeta, error)` — ListMine is G3.
- **func** `(s *GoalService) list(ctx context.Context, a Actor, f repositories.GoalFilter, page dto.Page) ([]dto.GoalResponse, dto.PageMeta, error)`
- **func** `(s *GoalService) Update(ctx context.Context, a Actor, id uuid.UUID, req dto.UpdateGoalRequest) (*dto.GoalResponse, error)` — Update is G5: title, description, weight while the cycle allows goal changes (FR-GL003, PM-003, PM-004).
- **func** `applyGoalUpdate(g *models.Goal, req dto.UpdateGoalRequest) error`
- **func** `(s *GoalService) SetStatus(ctx context.Context, a Actor, id uuid.UUID, to string) (*dto.GoalResponse, error)` — SetStatus is G6 (FR-GL004).
- **func** `(s *GoalService) AddKeyResult(ctx context.Context, a Actor, id uuid.UUID, req dto.KeyResultRequest) (*dto.GoalResponse, error)` — AddKeyResult is G7 (FR-GL005, PM-006, PM-007): the goal's progress becomes the mean of its key results.
- **func** `(s *GoalService) CheckIn(ctx context.Context, a Actor, id uuid.UUID, req dto.CheckInRequest) (*dto.GoalResponse, error)` — CheckIn is G8 (FR-GL006, PM-006, PM-007).
- **func** `(s *GoalService) applyCheckIn(ctx context.Context, tx *gorm.DB, a Actor, g *models.Goal, krID *uuid.UUID, req dto.CheckInRequest, note *string) (*models.OutboxEvent, error)`
- **type** `progressChange struct` — progressChange describes what caused a progress update (recorded on the check-in).
- **func** `(s *GoalService) recompute(ctx context.Context, tx *gorm.DB, a Actor, g *models.Goal, ch progressChange) (*models.OutboxEvent, error)` — recompute derives the goal's progress from all its key results (PM-006) and records the change.
- **func** `(s *GoalService) recordProgress(ctx context.Context, tx *gorm.DB, a Actor, g *models.Goal, to score.Fixed, ch progressChange) (*models.OutboxEvent, error)` — recordProgress updates the goal and appends the check-in in the same transaction (PM-007, NFR-D001).
- **func** `(s *GoalService) mutate(ctx context.Context, a Actor, id uuid.UUID, fn func(*gorm.DB, *models.Goal) (*models.OutboxEvent, error)) (*models.Goal, error)` — mutate locks the goal, applies PM-008 write access (denial → 404) and runs fn in one transaction; the audit row and event follow the commit.

#### `internal/hr/performance/services/helpers.go` (181 lines)
- **type** `base struct{ Deps }` — base carries Deps plus helpers shared by all services.
- **func** `(b base) employeeFor(ctx context.Context, a Actor) (*employeeDTO.EmployeeResponse, error)` — employeeFor resolves the caller's own employee profile (self endpoints, NFR-SEC002).
- **func** `(b base) optionalSelf(ctx context.Context, a Actor) (*employeeDTO.EmployeeResponse, error)` — optionalSelf is employeeFor where "no profile" is not an error (callers acting only through permissions).
- **func** `(b base) employee(ctx context.Context, tenantID, id uuid.UUID) (*employeeDTO.EmployeeResponse, error)` — employee loads an employee of the tenant; unknown → EMPLOYEE_NOT_FOUND.
- **func** `(b base) managerEmployee(ctx context.Context, tenantID uuid.UUID, emp *employeeDTO.EmployeeResponse) (*employeeDTO.EmployeeResponse, error)` — managerEmployee resolves an employee's direct manager's employee profile (D7-03); nil when there is none or the manager has no profile (A7-02).
- **func** `(b base) managesEmployee(ctx context.Context, a Actor, employeeID uuid.UUID) (bool, error)` — managesEmployee reports whether the caller is the direct manager of the employee (PM-008).
- **func** `(b base) canAccessEmployee(ctx context.Context, a Actor, employeeID uuid.UUID, write bool) (bool, error)` — canAccessEmployee is PM-008 for an owner: the permission flag, the owner themself, or the owner's manager. Denials are reported as false (callers answer 404, D7-05).
- **func** `(b base) writeOutbox(ctx context.Context, tx *gorm.DB, a Actor, key string, payload interface{}) (*models.OutboxEvent, error)` — writeOutbox stores the event inside the business transaction (FR-EV001).
- **type** `auditEntry struct` — auditEntry is one audit row; metadata holds ids and statuses only (PM-017).
- **func** `(b base) afterCommit(ctx context.Context, a Actor, e auditEntry, row *models.OutboxEvent)` — afterCommit writes the audit row and tries to publish the event; the relay retries what fails.
- **func** `parseUUIDField(field, raw string) (uuid.UUID, error)`
- **func** `parseOptionalUUID(field string, raw *string) (*uuid.UUID, error)`
- **func** `text(field, s string, max int) (string, error)` — text trims s and checks 1..max characters.
- **func** `optionalText(field string, s *string, max int) (*string, error)` — optionalText trims an optional field; blank → nil; longer than max → 400.
- **func** `orNotFound(err error) error`
- **func** `uuidPtr(id uuid.UUID) *uuid.UUID { return &id }`

#### `internal/hr/performance/services/mappers.go` (64 lines)
- **func** `cycleResponse(c *models.Cycle) dto.CycleResponse`
- **func** `cycleResponses(rows []models.Cycle) []dto.CycleResponse`
- **func** `goalResponse(g *models.Goal) dto.GoalResponse`
- **func** `goalResponses(rows []models.Goal) []dto.GoalResponse`
- **func** `keyResultResponse(k *models.KeyResult) dto.KeyResultResponse`
- **func** `checkInResponse(c *models.CheckIn) dto.CheckInResponse`
- **func** `reviewResponse(r *models.Review, anonymous bool) dto.ReviewResponse` — reviewResponse maps a review; anonymous hides the reviewer of peer reviews (PM-014).
- **func** `reviewResponses(rows []models.Review, anonymous bool) []dto.ReviewResponse`

#### `internal/hr/performance/services/outbox_relay.go` (59 lines)
- **type** `OutboxRelay struct` — OutboxRelay republishes outbox rows the request path could not publish.
- **func** `NewOutboxRelay(d Deps) *OutboxRelay { return &OutboxRelay{base: base{d}, interval: relayInterval} }` — NewOutboxRelay builds a relay over the module dependencies.
- **func** `(r *OutboxRelay) RelayOnce(ctx context.Context) (int, error)` — RelayOnce publishes one batch and returns how many rows were published.
- **func** `(r *OutboxRelay) Run(ctx context.Context)` — Run ticks RelayOnce until ctx is cancelled.

#### `internal/hr/performance/services/ports.go` (79 lines)
_Package:_ Package services holds HR/Performance business rules (specification.md PM-001..PM-017).
- **type** `EmployeeDirectory interface` — EmployeeDirectory is HR/Performance's view of HR/Employee (connection C4); employee/api.Directory satisfies it.
- **method** `EmployeeDirectory.GetByID(ctx context.Context, tenantID, id uuid.UUID) (*employeeDTO.EmployeeResponse, error)`
- **method** `EmployeeDirectory.GetByUserID(ctx context.Context, tenantID, userID uuid.UUID) (*employeeDTO.EmployeeResponse, error)`
- **type** `ManagerDirectory interface` — ManagerDirectory resolves a user's direct manager from HR/Organization (connection C3, D7-03): the manager_user_id of the user's primary active mapping, or nil.
- **method** `ManagerDirectory.DirectManager(ctx context.Context, tenantID, userID uuid.UUID) (*uuid.UUID, error)`
- **type** `AuditLogger interface` — AuditLogger is HR/Performance's view of the M0 audit trail.
- **method** `AuditLogger.Log(ctx context.Context, tx *gorm.DB, tenantID, userID string, action, resource, resourceID string, metadata interface{}, ipAddress, userAgent string) error`
- **type** `TxRunner interface` — TxRunner opens database transactions.
- **method** `TxRunner.InTx(ctx context.Context, fn func(tx *gorm.DB) error) error`
- **method** `TxRunner.DB() *gorm.DB`
- **type** `Clock func() time.Time` — Clock returns the current instant.
- **type** `Actor struct` — Actor is the authenticated caller from M0 middleware context only (NFR-SEC002). CanRead and CanManage are the performance:read / performance:manage flags resolved by the route (PermissionFlag).
- **type** `Repos struct` — Repos bundles the context repositories.
- **type** `Deps struct` — Deps is everything an HR/Performance service needs.
- **type** `gormRunner struct{ db *gorm.DB }`
- **func** `NewTxRunner(db *gorm.DB) TxRunner { return gormRunner{db: db} }` — NewTxRunner wraps a GORM handle as a TxRunner.
- **func** `(g gormRunner) InTx(ctx context.Context, fn func(tx *gorm.DB) error) error`
- **func** `(g gormRunner) DB() *gorm.DB { return g.db }`

#### `internal/hr/performance/services/review_service.go` (243 lines)
- **type** `ReviewService struct{ base }` — ReviewService implements FR-RV (V1–V6).
- **func** `NewReviewService(d Deps) *ReviewService { return &ReviewService{base{d}} }` — NewReviewService builds the service.
- **func** `(s *ReviewService) Assign(ctx context.Context, a Actor, req dto.AssignReviewerRequest) (*dto.ReviewResponse, error)` — Assign is V1 (FR-RV001, PM-011).
- **func** `(s *ReviewService) List(ctx context.Context, tenantID uuid.UUID, f repositories.ReviewFilter, page dto.Page) ([]dto.ReviewResponse, dto.PageMeta, error)` — List is V2 (performance:read).
- **func** `(s *ReviewService) ListAssigned(ctx context.Context, a Actor, status string, page dto.Page) ([]dto.ReviewResponse, dto.PageMeta, error)` — ListAssigned is V3: reviews the caller must write.
- **func** `(s *ReviewService) ListMine(ctx context.Context, a Actor, cycleID *uuid.UUID) ([]dto.ReviewResponse, error)` — ListMine is V4 (PM-014, PM-021): the caller's submitted reviews of completed cycles with the cycle's final rating; peer reviewers stay anonymous and calibration reasons are never included.
- **func** `(s *ReviewService) Get(ctx context.Context, a Actor, id uuid.UUID) (*dto.ReviewResponse, error)` — Get is V5: the reviewer or performance:read; anyone else 404.
- **func** `(s *ReviewService) Submit(ctx context.Context, a Actor, id uuid.UUID, req dto.SubmitReviewRequest) (*dto.ReviewResponse, error)` — Submit is V6 (FR-RV003, PM-012): only the assigned reviewer, once, while the cycle is in review.
- **func** `(s *ReviewService) fill(r *models.Review, criteria []string, req dto.SubmitReviewRequest) error` — fill validates ratings and potential against the cycle (PM-012) and marks the review submitted.
- **func** `reviewStatus(s string) error`

#### `internal/hr/performance/validators/validators.go` (148 lines)
_Package:_ Package validators holds Module 7 field rules that struct tags cannot express.
- **func** `Criteria(in []string) ([]string, error)` — Criteria normalizes PM-001: trimmed, 1–50 characters, unique ignoring case, at most 10, order kept; empty → defaults.
- **func** `DateRange(start, end string) (time.Time, time.Time, error)` — DateRange parses start/end dates (UTC midnight) and checks end ≥ start.
- **func** `Weight(w score.Fixed) error` — Weight checks PM-004's per-goal bound 0.01–100.
- **func** `Progress(p score.Fixed) error` — Progress checks 0–100.
- **func** `KeyResult(start, target score.Fixed) error` — KeyResult rejects a degenerate key result (FR-GL005).
- **func** `Ratings(criteria []string, ratings models.Ratings) error` — Ratings checks PM-012: every criterion exactly once (case-insensitive), scores 1–5.
- **func** `Canonical(criteria []string, ratings models.Ratings) models.Ratings` — Canonical returns validated ratings in the cycle's criteria order and spelling.
- **func** `Potential(reviewType string, p *string) error` — Potential enforces PM-012: required (low|medium|high) for manager reviews, absent otherwise.

## D. Test inventory


### Shared infrastructure — 21 test functions in 13 files
- `internal/shared/config/config_test.go`: TestLoadConfig, TestLoadConfig_EnvOverrides
- `internal/shared/database/base_test.go`: TestBaseModel_BeforeCreate
- `internal/shared/database/tx_test.go`: TestBeginTxNilDB, TestFinishNilTx
- `internal/shared/errors/db_test.go`: TestNormalize
- `internal/shared/errors/errors_test.go`: TestAppError_Error
- `internal/shared/idempotency/middleware_test.go`: TestReplaySameKeySamePayload, TestInProgressExpiredAndServerErrors
- `internal/shared/middleware/correlation_test.go`: TestCorrelationIDIsValidatedEchoedAndPropagated
- `internal/shared/middleware/cors_test.go`: TestCORS
- `internal/shared/middleware/hardening_test.go`: TestBodyLimit, TestSecurityHeaders
- `internal/shared/middleware/rate_limiter_test.go`: TestRateLimiter_BlocksAfterLimitWithRetryAfter, TestFailureRateLimiter_SuccessRefundsSlot, TestRateLimiter_FailsOpenOnStoreError
- `internal/shared/outbox/relay_test.go`: TestRelayPublishesAndRecordsFailures
- `internal/shared/utils/pagination_test.go`: TestNormalizePagination, TestParsePaginationToleratesJunk, TestBindErrorDetails
- `internal/shared/utils/validator_test.go`: TestValidateStruct

### M0 Identity & Access — 111 test functions in 34 files
- `internal/identity/access/access_test.go`: TestScopeOrderAndValidity, TestResolveInheritanceAndWidestScope, TestResolveUserGrantsExpiryAndBlocks, TestResolveRejectsCyclesAndDeepChains, TestCheckParent, TestCoversAll, TestParseScopeAndCheckParentEdges
- `internal/identity/access/catalogue_test.go`: TestTemplatesAreOrderedAndChained, TestValidateDefsAndDefaults
- `internal/identity/controllers/access_admin_controller_test.go`: TestAccessControllerRoutes, TestAccessControllerErrorsAndMissingContext
- `internal/identity/controllers/access_controller_test.go`: TestUserController_ActivateAndSelfDeactivationGuard, TestRoleController_RemoveAndMe, TestRoleController_ServiceErrorsMapToEnvelope, TestAuditController_List
- `internal/identity/controllers/auth_controller_test.go`: TestAuthController_Login, TestAuthController_Logout, TestAuthController_Refresh, TestAuthController_ForgotPassword, TestAuthController_ResetPassword
- `internal/identity/controllers/mocks_test.go`: (helpers only)
- `internal/identity/controllers/permission_controller_test.go`: TestPermissionController_List
- `internal/identity/controllers/role_controller_test.go`: TestRoleController_Create, TestRoleController_GetByID, TestRoleController_List, TestRoleController_Update, TestRoleController_Delete, TestRoleController_AssignPermissions, TestRoleController_AssignRolesToUser
- `internal/identity/controllers/tenant_controller_test.go`: TestTenantController_Create, TestTenantController_GetByID, TestTenantController_List, TestTenantController_Update, TestTenantController_ActivateSuspend
- `internal/identity/controllers/user_controller_test.go`: TestUserController_Create, TestUserController_GetByID, TestUserController_List, TestUserController_Update, TestUserController_Deactivate
- `internal/identity/events/events_test.go`: TestNewEvent
- `internal/identity/middleware/audit_test.go`: TestAuditLog
- `internal/identity/middleware/auth_test.go`: TestAuthenticate
- `internal/identity/middleware/mocks_test.go`: (helpers only)
- `internal/identity/middleware/platform_test.go`: TestPlatformAdmin
- `internal/identity/middleware/rbac_test.go`: TestRequirePermissionNeedsTenantScope, TestRequireScopedPermission, TestTenantAdminBypassAndFailures, TestPermissionFlag
- `internal/identity/middleware/tenant_test.go`: TestTenantResolver
- `internal/identity/models/models_test.go`: TestGORMModels_Parsing, TestGORMModels_AuditLog_NoSoftDelete, TestGORMModels_BaseHook
- `internal/identity/models/permission_catalog_test.go`: TestIdentityCatalogueValidAndComplete
- `internal/identity/services/access_fakes_test.go`: (helpers only)
- `internal/identity/services/access_model_test.go`: TestResolverCachesByAccessVersion, TestResolverCorruptCacheAndRoleGrants, TestCatalogueRegisterAndProvision, TestAutomaticRolesReconcile, TestAutomaticRolesProvisionAndRun, TestAccessServiceGrantsAndBlocks, TestAccessServiceExplainAndErrors, TestRoleServiceScopesParentsAndClone, TestRoleServicePredefinedAndAutomaticGuards, TestTenantCreationProvisionsPredefinedRoles
- `internal/identity/services/audit_service_test.go`: TestAuditService_Log
- `internal/identity/services/auth_service_test.go`: TestAuthService_Login, TestAuthService_Logout, TestAuthService_RefreshAfterLogoutIsPlain401NotReuse, TestAuthService_RefreshToken, TestAuthService_ForgotPassword, TestAuthService_ResetPassword
- `internal/identity/services/fault_injection_test.go`: TestFaults_RoleService, TestRoleService_BusinessRules, TestFaults_UserService, TestUserService_BusinessRules, TestFaults_TenantService, TestFaults_AuthService, TestAuthService_RulesAndTimingPath
- `internal/identity/services/grants_test.go`: TestCallerContext, TestEnsureCanGrant, TestEnsureCanAssignRoles, TestServicesEnforceGrantRules
- `internal/identity/services/misc_services_test.go`: TestAuditService_List, TestTokenService_RefreshTTLAndValidation, TestUserDeactivationConvergerHook, TestSessionServiceTTL
- `internal/identity/services/mocks_test.go`: (helpers only)
- `internal/identity/services/permission_service_test.go`: TestPermissionService_ListPermissions
- `internal/identity/services/role_service_test.go`: TestRoleService_CreateRole, TestRoleService_GetRoleByID, TestRoleService_ListRoles, TestRoleService_UpdateRole, TestRoleService_DeleteRole, TestRoleService_AssignPermissions, TestRoleService_AssignRolesToUser
- `internal/identity/services/session_service_test.go`: TestSessionService_CreateSession, TestSessionService_ValidateSession, TestSessionService_RevokeSession, TestSessionService_RevokeAllForUser
- `internal/identity/services/tenant_service_test.go`: TestTenantService_CreateTenant, TestTenantService_GetTenant, TestTenantService_ListTenants, TestTenantService_UpdateTenant, TestTenantService_ActivateSuspendTenant
- `internal/identity/services/token_service_test.go`: TestTokenService_AccessToken, TestTokenService_OpaqueToken, TestEdDSASigningJWKSAndLegacyWindow
- `internal/identity/services/user_service_test.go`: TestUserService_CreateUser, TestUserService_InviteUser, TestUserService_GetByID, TestUserService_ListUsers, TestUserService_UpdateUser, TestUserService_DeactivateUser
- `internal/identity/validators/validators_test.go`: TestValidateSlug, TestIsReservedSlug

### M1 HR / Organization — 66 test functions in 22 files
- `internal/hr/organization/controllers/department_controller_test.go`: TestDepartmentController_Create, TestDepartmentController_CreateBadBody, TestDepartmentController_GetByID, TestDepartmentController_List, TestDepartmentController_MissingTenant, TestDepartmentController_Deactivate
- `internal/hr/organization/controllers/designation_controller_test.go`: TestDesignationController_CRUD
- `internal/hr/organization/controllers/mapping_controller_test.go`: TestMappingController_CreateAndList, TestMappingController_UpdateAndDeactivate
- `internal/hr/organization/controllers/org_chart_controller_test.go`: TestOrgChartController_ChartAndChain
- `internal/hr/organization/controllers/stubs_test.go`: (helpers only)
- `internal/hr/organization/controllers/team_controller_test.go`: TestTeamController_CreateAndGet, TestTeamController_ListAndDeactivate
- `internal/hr/organization/controllers/update_paths_test.go`: TestDepartmentController_UpdateAndErrors, TestTeamController_UpdateAndDeactivateErrors, TestDesignationController_UpdatePath, TestControllers_ValidationEnvelope, TestRespondErrorFallback
- `internal/hr/organization/events/events_test.go`: TestNewEvent_Envelope
- `internal/hr/organization/models/models_test.go`: TestOrgModels_Parsing, TestOrgModels_Mapping_AppendOnly, TestOrgModels_UUIDHook
- `internal/hr/organization/routes/routes_test.go`: TestChartInvalidation
- `internal/hr/organization/services/coverage_branches_test.go`: TestMappingService_UpdateRefsAndBadInput, TestMappingService_DeactivateUserRepoError, TestOrgChart_IncludeInactiveAndErrors
- `internal/hr/organization/services/department_service_test.go`: TestDepartmentService_CreateDepartment, TestDepartmentService_MoveCycle, TestDepartmentService_DeactivateGuard, TestDepartmentService_GetListUpdate, TestDepartmentService_UpdateUnknownParent
- `internal/hr/organization/services/designation_service_test.go`: TestDesignationService_CreateAndDuplicate, TestDesignationService_UpdateAndDeactivate, TestDesignationService_CaseInsensitiveTitleRepo, TestDesignationService_GetListHappyUpdate
- `internal/hr/organization/services/errorpaths_test.go`: TestServiceErrorPropagation
- `internal/hr/organization/services/event_consumer_test.go`: TestEventConsumer_UserDeactivatedConverges, TestEventConsumer_NoopHandlers, TestEventConsumer_UnknownKeyAndBadPayload
- `internal/hr/organization/services/fault_injection_test.go`: TestFaults_Departments, TestFaults_Teams, TestFaults_Designations, TestFaults_MappingsAndChart, TestTeamMoveMakesMappingsFollow
- `internal/hr/organization/services/helpers_test.go`: TestPublishOrOutbox_BrokerDown, TestPublishOrOutbox_SuccessSkipsOutbox
- `internal/hr/organization/services/hierarchy_test.go`: TestHierarchyDepthCap, TestHierarchyMoveValidation, TestDeactivateWithActiveMappingsGuard, TestUpdateRepoErrorPropagation
- `internal/hr/organization/services/mapping_service_test.go`: TestMappingService_CreateAndPrimaryGuard, TestMappingService_CRUDAndPrimarySwap, TestMappingService_ManagerCycle, TestMappingService_ManagerSelfAndMissing, TestMappingService_DeactivateUserConverges
- `internal/hr/organization/services/org_chart_service_test.go`: TestOrgChart_EmptyTenant, TestOrgChart_ForestBuild, TestOrgChart_UserChain, TestOrgChart_InvalidateChartDropsOnlyThatTenant, TestOrgChart_InvalidateChartNilCache, TestOrgChart_CacheHitAndSet, TestDepartmentService_ForceCascadeDeactivatesChildren
- `internal/hr/organization/services/team_service_test.go`: TestTeamService_CreateTeam, TestTeamService_MoveAndDeactivate, TestTeamService_GetListRenameConflict
- `internal/hr/organization/validators/validators_test.go`: TestValidateOrgCode, TestIsReservedOrgCode

### M1 HR / Employee — 23 test functions in 17 files
- `internal/hr/employee/controllers/document_controller_test.go`: TestDocumentController_Endpoints
- `internal/hr/employee/controllers/employee_controller_test.go`: TestEmployeeController_Endpoints
- `internal/hr/employee/controllers/statutory_controller_test.go`: TestStatutoryController_Endpoints
- `internal/hr/employee/controllers/stubs_test.go`: (helpers only)
- `internal/hr/employee/controllers/timeline_controller_test.go`: TestTimelineController_List
- `internal/hr/employee/events/events_test.go`: TestNewEventEnvelope
- `internal/hr/employee/models/models_test.go`: TestEmployeeModelsInstantiation
- `internal/hr/employee/services/coverage_branches_test.go`: TestEmployeeServiceEdgeCasesAndBranches, TestStatutoryAndDocumentAndTimelineEdgeCases
- `internal/hr/employee/services/document_service_test.go`: TestEmployeeDocumentService
- `internal/hr/employee/services/employee_service_test.go`: TestEmployeeServiceLifecycle
- `internal/hr/employee/services/event_consumer_test.go`: TestEventConsumerHandleUserDeactivated
- `internal/hr/employee/services/fault_injection_test.go`: TestFaults_EmployeeService, TestEmployeeService_BusinessRules, TestFaults_StatutoryDocumentsTimeline, TestEventConsumer_ConvergenceRules
- `internal/hr/employee/services/status_effects_test.go`: TestApplyStatusEffects
- `internal/hr/employee/services/statutory_service_test.go`: TestEmployeeStatutoryService
- `internal/hr/employee/services/stubs_test.go`: (helpers only)
- `internal/hr/employee/services/timeline_service_test.go`: TestEmployeeTimelineService
- `internal/hr/employee/validators/validators_test.go`: TestValidateEmployeeCode, TestValidateStatus, TestValidateEmploymentType, TestValidateTransitionMatrix, TestValidateEmergencyContacts

### M1 HR / Attendance — 74 test functions in 20 files
- `internal/hr/attendance/calc/calc_golden_test.go`: TestGolden_G4_Totals, TestGolden_G4_FirstLast, TestGolden_G5_NightShift, TestGolden_G5_TimeZoneLateness
- `internal/hr/attendance/calc/calc_test.go`: TestParseAndFormatHHMM, TestShiftShape, TestThresholdDefaults, TestAttendanceDateNoShiftIsUTC, TestAttendanceDateWindowClampedByShiftStart, TestOpenSessionStart
- `internal/hr/attendance/controllers/controllers_test.go`: TestEndpoints_HappyPathsAndStatusCodes, TestEndpoints_ServiceErrorsMapToEnvelope, TestEndpoints_ValidationEnvelope, TestEndpoints_G13_PaginationClamp, TestEndpoints_RequireActorContext, TestEndpoints_SelfEndpointsIgnoreClientIDs, TestSnake, TestFail_NormalizesPersistenceErrors
- `internal/hr/attendance/controllers/stub_test.go`: (helpers only)
- `internal/hr/attendance/dto/dto_test.go`: TestParsePage_G13, TestPageOffsetAndMeta
- `internal/hr/attendance/events/events_test.go`: TestNewEnvelope, TestPayloadsCarryNoLocationOrNetworkData
- `internal/hr/attendance/models/models_test.go`: TestAttendanceModels_TableNames, TestAttendanceModels_Indexes, TestAttendanceModels_PendingIndexIsPartial, TestAttendanceModels_IndexParityWithSQL, TestAttendanceModels_IntegerColumns, TestAttendanceModels_AppendOnly, TestAttendanceModels_BeforeCreateGeneratesIDs
- `internal/hr/attendance/routes/routes_test.go`: TestRoutes_ContractMatchesSpec, TestRoutes_SelfEndpointsHaveNoPermissionGuard
- `internal/hr/attendance/services/assignment_golden_test.go`: TestGolden_G10_AssignmentOverlap, TestAssignmentService_Validation, TestAssignmentService_List
- `internal/hr/attendance/services/fakes_more_test.go`: (helpers only)
- `internal/hr/attendance/services/fakes_test.go`: (helpers only)
- `internal/hr/attendance/services/harness_test.go`: (helpers only)
- `internal/hr/attendance/services/leave_sync_test.go`: TestLeaveSync_MarkAndClear, TestLeaveSync_ClearRecomputesForeignRecords, TestLeaveSync_Errors, TestLeaveSync_MarkExistingUnpunched
- `internal/hr/attendance/services/outbox_golden_test.go`: TestGolden_G12_RelayDrainsOutbox, TestRelay_SkipsExhaustedRowsAndPropagatesFetchErrors, TestRelay_RunStopsOnContextCancel
- `internal/hr/attendance/services/punch_golden_test.go`: TestGolden_G2_PunchAlternation, TestGolden_G3_ServerClockOnly, TestGolden_G11_PunchAudit, TestGolden_G12_OutboxAtomicAndPrivate, TestPunch_EmployeeRules, TestPunch_DebounceAndExpiry, TestPunch_NightShiftThroughService, TestPunch_PersistsLocationButNeverReturnsIP
- `internal/hr/attendance/services/query_service_test.go`: TestQuery_TodayStates, TestQuery_MineRange, TestQuery_AdminListAndDetail, TestQuery_Summary
- `internal/hr/attendance/services/regularization_golden_test.go`: TestGolden_G6_RegularizationLifecycle, TestGolden_G6_RegularizationCreatesMissingRecord, TestGolden_G7_SelfApprovalForbidden, TestGolden_G8_OnePendingPerDate, TestRegularization_CreateValidation, TestRegularization_RejectCancelList, TestRegularization_ApproveRollsBackOnFailure
- `internal/hr/attendance/services/resolve_day_test.go`: TestResolveDay_DateMovesForwardWithShiftZone, TestResolveDay_MovedOntoUnassignedDayFallsBack, TestResolveDay_RepoErrorsPropagate, TestPunch_MobileSource, TestReview_DirectoryFailureFailsClosed
- `internal/hr/attendance/services/shift_service_test.go`: TestShiftService_CreateDefaultsAndDerivedFields, TestShiftService_CreateValidation, TestShiftService_GetListUpdate, TestShiftService_DeactivateGuard
- `internal/hr/attendance/validators/validators_test.go`: TestValidateShiftCode, TestLoadTimezone, TestParseDate, TestDateRange, TestThresholdPair

### M1 HR / Leave — 48 test functions in 16 files
- `internal/hr/leave/calc/calc_golden_test.go`: TestGolden_G2_CountDays, TestGolden_G3_Accrual, TestGolden_G4_CarryForward, TestDateHelpers
- `internal/hr/leave/controllers/controllers_test.go`: TestEndpoints_HappyPaths, TestEndpoints_ErrorsAndAuth, TestEndpoints_Validation, TestEndpoints_G15_Pagination, TestEndpoints_SelfIgnoresClientIdentity, TestFail_NormalizesPersistenceErrors
- `internal/hr/leave/controllers/stub_test.go`: (helpers only)
- `internal/hr/leave/dto/dto_test.go`: TestParsePage_G15, TestPage_OffsetAndMeta
- `internal/hr/leave/events/events_test.go`: TestNew_Envelope, TestPayloads_NoFreeText
- `internal/hr/leave/models/models_test.go`: TestLeaveModels_TableNames, TestLeaveModels_Indexes, TestLeaveModels_ColumnTypes, TestLeaveModels_DeletionPolicy, TestLeaveModels_BeforeCreateGeneratesIDs, TestBalance_Available
- `internal/hr/leave/routes/routes_test.go`: TestRoutes_ContractMatchesSpec, TestRoutes_OpenToAuthenticated
- `internal/hr/leave/services/admin_services_test.go`: TestTypeService_Lifecycle, TestHolidayService, TestBalanceService
- `internal/hr/leave/services/fakes_more_test.go`: (helpers only)
- `internal/hr/leave/services/fakes_test.go`: (helpers only)
- `internal/hr/leave/services/harness_test.go`: (helpers only)
- `internal/hr/leave/services/query_relay_test.go`: TestRequestQueries, TestOutboxRelay, TestTxRunner_DB, TestFaultInjection, TestAttendanceSyncFailureRollsBack
- `internal/hr/leave/services/request_golden_test.go`: TestGolden_G5_ApplyReserves, TestGolden_G6_InsufficientBalance, TestGolden_G7_Overlap, TestGolden_G8_ApproveMarksAttendance, TestGolden_G10_SelfApproval, TestGolden_G11_AuditPrivacy, TestGolden_G13_CancelApprovedFuture
- `internal/hr/leave/services/request_rules_test.go`: TestApply_Rules, TestApply_EmployeeChecks, TestPreview_CalendarAndSandwich, TestReview_States, TestApprove_HalfDayNoAttendance
- `internal/hr/leave/services/unpaid_leave_test.go`: TestUnpaidLeave_UnpaidDays
- `internal/hr/leave/validators/validators_test.go`: TestNormalizeTypeCode, TestAmounts, TestParseDateAndYear, TestGenderApplies, TestNoticeOK

### M1 HR / Payroll — 37 test functions in 14 files
- `internal/hr/payroll/calc/calc_golden_test.go`: TestGolden_G2_Breakdown, TestGolden_G3_Overflow, TestGolden_G4_Proration, TestGolden_G5_PF, TestGolden_G6_ESI, TestGolden_G7_PT, TestGolden_G8_TDS, TestGolden_G9_NetCap, TestCompute_FullPayslip, TestCompute_OverflowAndInterleavedCap
- `internal/hr/payroll/calc/structure_test.go`: TestValidateStructure, TestMonthlyAndDaysInMonth
- `internal/hr/payroll/controllers/controllers_test.go`: TestEndpoints, TestEndpoints_ErrorsAuthValidation, TestEndpoints_G16
- `internal/hr/payroll/dto/dto_test.go`: TestParsePage_G16
- `internal/hr/payroll/events/events_test.go`: TestNewAndPayload
- `internal/hr/payroll/models/models_test.go`: TestPayrollModels_TableNames, TestPayrollModels_Indexes, TestPayrollModels_ColumnTypes, TestPayrollModels_DeletionAndIDs
- `internal/hr/payroll/routes/routes_test.go`: TestRoutes_Contract
- `internal/hr/payroll/services/admin_test.go`: TestStructureService, TestAssignmentService, TestRunCreateAndQueries
- `internal/hr/payroll/services/fakes_more_test.go`: (helpers only)
- `internal/hr/payroll/services/fakes_test.go`: (helpers only)
- `internal/hr/payroll/services/harness_test.go`: (helpers only)
- `internal/hr/payroll/services/payslip_test.go`: TestPayslipQueriesAndCSV, TestCalculateWarnings, TestCalculateSkipsOutsideWindow, TestPayrollFaultInjection, TestOutboxRelayAndRunner
- `internal/hr/payroll/services/run_golden_test.go`: TestGolden_Calculate, TestGolden_G11_G12_StateMachine, TestGolden_G15_Recalculate, TestGolden_SelfVisibilityAndPrivacy
- `internal/hr/payroll/validators/validators_test.go`: TestParseDateAndCTC, TestValidatePeriod, TestCSVSafe

### M1 HR / Recruitment — 29 test functions in 13 files
- `internal/hr/recruitment/module_test.go`: TestOrgDirectory
- `internal/hr/recruitment/controllers/controllers_test.go`: TestControllers_HappyPathAndAuth, TestControllers_ServiceErrorsAndBadInput, TestControllers_QueryFilters
- `internal/hr/recruitment/dto/dto_test.go`: TestParsePage_G14
- `internal/hr/recruitment/events/events_test.go`: TestNewEnvelope
- `internal/hr/recruitment/models/models_test.go`: TestRecruitmentModels_TablesAndIndexes, TestRecruitmentModels_ColumnTypesAndIDs
- `internal/hr/recruitment/pipeline/pipeline_test.go`: TestGolden_G2_StageGraph, TestGolden_G3_JobStatusGraph
- `internal/hr/recruitment/routes/routes_test.go`: TestRoutes_Contract
- `internal/hr/recruitment/services/admin_test.go`: TestJobs_UpdateListAndOrgRefs, TestCandidates_ListUpdateAndOfferExit, TestInterviews_ListCancelAndLookups, TestOffers_ValidationWithdrawAndExpiry, TestHire_PortFailures, TestOutboxRelay, TestHelpers
- `internal/hr/recruitment/services/failures_test.go`: TestRepositoryFailures
- `internal/hr/recruitment/services/fakes_test.go`: (helpers only)
- `internal/hr/recruitment/services/golden_test.go`: TestGolden_G4_ClosedJob, TestGolden_G5_DuplicateEmail, TestGolden_G6_StageHistory, TestGolden_G7_InterviewGate, TestGolden_G8_FeedbackOwnership, TestGolden_G9_OneOpenOffer, TestGolden_G10_Hire, TestGolden_G11_HireIdempotency, TestGolden_G12_NoPIIInEventsOrAudit
- `internal/hr/recruitment/services/harness_test.go`: (helpers only)
- `internal/hr/recruitment/validators/validators_test.go`: TestValidators

### M1 HR / Performance & OKRs — 38 test functions in 13 files
- `internal/hr/performance/controllers/controllers_test.go`: TestControllers_EveryOperation, TestControllers_Errors, TestControllers_BadInput
- `internal/hr/performance/controllers/parity_controllers_test.go`: TestControllers_ParityInput
- `internal/hr/performance/lifecycle/lifecycle_test.go`: TestGolden_G2_CycleGraph, TestGolden_G2_GoalGraph
- `internal/hr/performance/models/models_test.go`: TestPerformanceModels_TablesAndIndexes, TestPerformanceModels_ColumnTypes, TestPerformanceModels_JSONColumnsAndIDs
- `internal/hr/performance/score/score_test.go`: TestParseAndCodec, TestGolden_G3_ScoreMath
- `internal/hr/performance/services/admin_test.go`: TestCycles_CRUDAndStateGraph, TestReviews_AssignAndLists, TestGoals_Validation, TestOutboxRelay
- `internal/hr/performance/services/failures_test.go`: TestFailures_Propagate, TestFailures_Submit
- `internal/hr/performance/services/fakes_parity_test.go`: (helpers only)
- `internal/hr/performance/services/fakes_test.go`: (helpers only)
- `internal/hr/performance/services/golden_test.go`: TestGolden_G4_WeightCap, TestGolden_G5_KeyResultProgress, TestGolden_G6_Access, TestGolden_G7_CycleLocks, TestGolden_G8_LaunchReviews, TestGolden_G9_Submit, TestGolden_G10_SubjectVisibility, TestGolden_G11_NineBox, TestGolden_G12_Privacy, TestGolden_G15_Cascading
- `internal/hr/performance/services/harness_test.go`: (helpers only)
- `internal/hr/performance/services/parity_test.go`: TestGolden_G16_Calibration, TestCalibration_Validation, TestGolden_G17_FinalRatingForSubject, TestGolden_G18_FeedbackVisibility, TestGolden_G19_FeedbackRequests, TestGolden_G20_ParityPrivacy, TestFeedback_Validation, TestFailures_Parity
- `internal/hr/performance/validators/validators_test.go`: TestCriteria_PM001, TestDatesWeightsProgressKR, TestRatings_PM012

### Integration suite (`backend/tests/integration`)
- `access_test.go`: TestAccessPredefinedRoles, TestAccessScopesInheritanceAndCycles, TestAccessUserGrantsAndBlocks, TestAccessAntiEscalation, TestAccessAutomaticRoles, TestAccessTenantIsolation
- `attendance_test.go`: TestAttendancePunchFlow, TestAttendanceRegularization, TestAttendanceRBAC, TestAttendanceIsolation, TestAttendanceConcurrentPunches, TestAttendancePagination
- `contract_test.go`: TestSwaggerMatchesRoutes
- `employee_test.go`: TestEmployeeLifecycle, TestEmployeeStatusStateMachine, TestEmployeeStatutoryAndPIIMasking, TestEmployeeDocuments, TestEmployeeSelfService, TestEmployeeRBACTiers, TestUserDeactivationConvergesEmployee
- `golden_test.go`: TestGoldenM1_TeamMoveKeepsMappingsConsistent, TestGoldenM1_AuditOnEveryOrgWrite, TestGoldenNoSecretLeakage, TestGoldenM2_EmployeeForForeignTenantUser, TestGoldenM2_AtomicOnboarding, TestGoldenM1_RejectedHierarchyChangeLeavesNoTrace
- `harness_test.go`: TestMain
- `identity_test.go`: TestTenantRoutesRequirePlatformKey, TestTenantLifecycle, TestTenantAdminProvisioningIsAtomic, TestPlatformRoutesDisabledWithoutKeyIsCoveredByMiddlewareUnit, TestAuthLoginErrors, TestLoginRateLimitCountsFailuresOnly, TestRefreshRotationReuseAndLogout, TestAccessTokenValidation, TestForgotAndResetPassword, TestAuthMe, TestUserLifecycle, TestDeactivatedUserSessionsAreRevoked, TestAssignAndRemoveRoles, TestRolesAndPermissions, TestRBACEnforcement, TestAuditTrail, TestPaginationIsClampedEverywhere, TestErrorEnvelopeIsUniform, TestCORSHeaders, TestTenantIsolation
- `invitation_test.go`: TestInvitationLifecycle, TestInvitationRevokeAndEscalation
- `leave_test.go`: TestLeaveLifecycle, TestLeaveOverlapAndPagination, TestLeaveRBACAndSelfApproval, TestLeaveIsolation, TestLeaveConcurrency
- `org_test.go`: TestDepartmentLifecycleAndHierarchy, TestDepartmentDepthLimit, TestTeamLifecycle, TestDesignationLifecycle, TestMappingRulesAndUserDeactivationConvergence, TestOrgChartIsReadYourWritesConsistent, TestOrgChartUserChain, TestOrgRBAC
- `payroll_test.go`: TestPayrollLifecycle, TestPayrollRBACAndIsolation
- `perf_test.go`: TestPerformanceBudgets
- `performance_parity_test.go`: TestPerformanceCalibrationAndFeedback
- `performance_test.go`: TestPerformanceLifecycle
- `platform_test.go`: TestHealthLivenessAndReadiness, TestCorrelationIDReachesAuditTrail, TestIdentityEventsGoThroughTheOutbox, TestIdempotencyKeyReplaysAndRejectsReuse, TestJWKSPublishesTheEdDSAKey
- `recruitment_test.go`: TestRecruitmentLifecycle, TestRecruitmentRBACAndIsolation
- `schema_test.go`: TestMigrationsAreIdempotentAndComplete, TestSchemasAreOwnedAndIsolated, TestBusinessCodesAreTenantScoped
- `security_test.go`: TestSecurityHeadersAndBodyLimit, TestInjectionAttemptsAreInert, TestPrivilegeEscalationIsBlocked
- `smoke_test.go`: TestHealth
- `time_helpers_test.go`: (helpers only)

## E. Business rules, errors and decisions (verbatim from each module bundle)


### m0-identity-access  _(from specification.md)_

**Requirements and rules (ID-tagged table rows)**

| ID | Text | … |
|---|---|---|
| FR-T001 | The system shall allow creation of a tenant with name, slug, optional domain, and plan. |
| FR-T002 | The system shall enforce unique slugs across all tenants. |
| FR-T003 | The system shall enforce unique domains across all tenants (when provided). |
| FR-T004 | The system shall allow retrieval of a single tenant by ID. |
| FR-T005 | The system shall allow retrieval of all tenants with pagination. |
| FR-T006 | The system shall allow partial update of tenant name, domain, and plan. |
| FR-T007 | The system shall allow activation of a suspended tenant. |
| FR-T008 | The system shall allow suspension of an active tenant. |
| FR-T009 | The system shall support soft deletion of tenants. |
| FR-T010 | The system shall support tenant-scoped settings as key-value pairs. |
| FR-A001 | The system shall authenticate a user by email, password, and tenant slug. |
| FR-A002 | The system shall issue a JWT access token upon successful authentication. |
| FR-A003 | The system shall issue a refresh token upon successful authentication. |
| FR-A004 | The system shall create a session record upon successful authentication. |
| FR-A005 | The system shall allow a user to log out, revoking their current session and refresh token. |
| FR-A006 | The system shall allow access token renewal using a valid refresh token. |
| FR-A007 | The system shall rotate the refresh token upon each renewal (issue a new one, revoke the old one). |
| FR-A008 | The system shall initiate a password reset by sending a time-limited token to the user's email. |
| FR-A009 | The system shall allow a user to set a new password using a valid, unused password reset token. |
| FR-A010 | The system shall reject authentication for users belonging to suspended tenants. |
| FR-A011 | The system shall reject authentication for inactive, suspended, or invited users. |
| FR-U001 | The system shall allow creation of a user within a tenant with email, password, first name, last name, and optional role assignments. |
| FR-U002 | The system shall enforce unique email addresses within a tenant. |
| FR-U003 | The system shall allow retrieval of a single user by ID within the authenticated tenant. |
| FR-U004 | The system shall allow retrieval of all users within the authenticated tenant with pagination. |
| FR-U005 | The system shall allow partial update of user profile (first name, last name, phone, avatar URL). |
| FR-U006 | The system shall allow deactivation of a user, revoking all active sessions. |
| FR-U007 | The system shall allow invitation of a user by email, creating a user record with status `invited`. |
| FR-U008 | The system shall hash passwords before storage. Plaintext passwords must never be persisted. |
| FR-R001 | The system shall allow creation of a role within a tenant with name, description, and optional permission assignments. |
| FR-R002 | The system shall enforce unique role names within a tenant. |
| FR-R003 | The system shall allow retrieval of all roles within the authenticated tenant with pagination. |
| FR-R004 | The system shall allow partial update of role name and description. |
| FR-R005 | The system shall allow deletion of non-system roles. |
| FR-R006 | The system shall prevent deletion of system roles. |
| FR-R007 | The system shall allow assignment of permissions to a role. |
| FR-R008 | The system shall allow assignment of roles to a user. |
| FR-P001 | The system shall provide a list of all available permissions. |
| FR-P002 | Permissions shall be defined as resource-action pairs (e.g., `users:create`). |
| FR-P003 | Permissions are globally defined and not tenant-scoped. |
| FR-S001 | The system shall create a session upon login, recording IP address, user agent, and expiry. |
| FR-S002 | The system shall support session revocation on logout. |
| FR-S003 | The system shall revoke all sessions when a user is deactivated. |
| FR-S004 | The system shall enforce session expiry. Expired sessions cannot be used for authentication. |
| FR-AL001 | The system shall record audit logs for all state-changing operations. |
| FR-AL002 | Audit logs shall be immutable — no updates or deletes. |
| FR-AL003 | Audit logs shall include tenant_id, user_id, action, resource, resource_id, metadata, IP address, user agent, and timestamp. |
| NFR-P001 | API response time for single-resource endpoints | < 200ms (p95) |
| NFR-P002 | API response time for list endpoints | < 500ms (p95) |
| NFR-P003 | Authentication (login) response time | < 300ms (p95) |
| NFR-P004 | Database query execution time | < 50ms (p95) |
| NFR-P005 | Concurrent users per tenant | 1,000+ |
| NFR-S001 | The system shall support 10,000+ tenants on shared infrastructure. |
| NFR-S002 | The system shall support 1,000,000+ total users across all tenants. |
| NFR-S003 | Audit log storage shall scale independently of transactional data. |
| NFR-SEC001 | Passwords shall be hashed using bcrypt with a cost factor of 12. |
| NFR-SEC002 | JWT access tokens shall expire after 15 minutes. |
| NFR-SEC003 | Refresh tokens shall expire after 7 days. |
| NFR-SEC004 | Password reset tokens shall expire after 1 hour. |
| NFR-SEC005 | All API endpoints (except login, forgot-password, reset-password) shall require authentication. |
| NFR-SEC006 | The system shall rate-limit authentication endpoints (login, forgot-password). |
| NFR-SEC007 | The system shall log all failed authentication attempts. |
| NFR-SEC008 | The system shall enforce minimum password length of 8 characters. |
| NFR-A001 | M0 Identity shall target 99.9% uptime. |
| NFR-A002 | Database failures shall be handled gracefully with appropriate error responses. |
| NFR-A003 | Queue unavailability shall not block synchronous API operations. Event publishing shall be best-effort with retry. |
| NFR-D001 | All database writes shall use transactions where multiple tables are affected. |
| NFR-D002 | UUID primary keys shall be used for all tables. |
| NFR-D003 | Soft deletes shall be used for tenants, users, and roles. Hard deletes only for join tables. |
| NFR-D004 | All tenant-scoped queries must be filtered by `tenant_id`. No cross-tenant data leakage. |
| AS-001 | A single PostgreSQL database instance is shared across all tenants. No schema-per-tenant isolation. |
| AS-002 | Tenant resolution is performed via subdomain extraction from the HTTP `Host` header. |
| AS-003 | The API gateway (Kong) handles TLS termination. The Go application receives plain HTTP internally. |
| AS-004 | Email delivery (for password reset, invitations) is handled by an external SMTP provider. M0 Identity publishes events; a notification service consumes them. |
| AS-005 | The JAAS Super Admin is a special user not scoped to any tenant, or scoped to a dedicated platform tenant. |
| AS-006 | Redis is available for session caching and rate limiting but is not the source of truth. PostgreSQL is the source of truth. |
| AS-007 | Permissions are seeded as part of application deployment, not created via API. |
| AS-008 | The system runs behind a reverse proxy that provides the `X-Forwarded-For` header for IP tracking. |
| CO-001 | The system must use Go as the backend language. |
| CO-002 | The system must use PostgreSQL as the primary database. |
| CO-003 | The system must use GORM as the ORM. |
| CO-004 | The system must use Gin as the HTTP framework. |
| CO-005 | The system must expose RESTful JSON APIs. No GraphQL. |
| CO-006 | The system must follow Clean Architecture principles. |
| CO-007 | M0 Identity must be deployable independently of other modules. |
| CO-008 | All API responses must follow a consistent envelope format. |
| CO-009 | M0 Identity must not depend on any other JAAS module. |
| CO-010 | The system must use RabbitMQ for event publishing. No Kafka, no NATS. |
| SC-001 | All 12 database tables are created and migrated successfully | Migration runs without errors |
| SC-002 | All 22 API endpoints return correct responses | API integration tests pass |
| SC-003 | Authentication flow (login → access → refresh → logout) works end-to-end | E2E test passes |
| SC-004 | Tenant isolation is enforced — no cross-tenant data access | Security tests pass |
| SC-005 | RBAC enforcement blocks unauthorized access | Permission-denied tests pass |
| SC-006 | All domain events are published on state changes | Event integration tests pass |
| SC-007 | Audit logs are created for all state-changing operations | Audit log assertions pass |
| SC-008 | Password reset flow works end-to-end | E2E test passes |
| SC-009 | API response times meet NFR targets | Load test report shows p95 < thresholds |
| SC-010 | Code coverage exceeds 80% for services layer | Coverage report |
| FE-001 | OAuth2 / SSO integration (Google, Microsoft) | High | M0 Identity v2 |
| FE-002 | API key authentication for machine-to-machine access | Medium | M0 Identity v2 |
| FE-003 | Tenant-level feature flags | Medium | M0 Identity v2 |
| FE-004 | Account lockout after N failed login attempts | High | M0 Identity v2 |
| FE-005 | User email verification flow | High | M0 Identity v2 |
| FE-006 | Hierarchical roles (role inheritance) | Low | M0 Identity v3 |
| FE-007 | Attribute-Based Access Control (ABAC) | Low | M0 Identity v3 |
| FE-008 | Tenant data export (GDPR compliance) | Medium | M0 Identity v2 |
| FE-009 | Login history and device tracking | Low | M0 Identity v3 |
| FE-010 | Webhook registration for tenant events | Medium | M0 Identity v2 |
| **TN-001** | A tenant must have a unique, non-empty `slug`. The slug is used for subdomain routing. |
| **TN-002** | A tenant slug must be lowercase, alphanumeric, and may contain hyphens. No spaces, no special characters. Regex: `^[a-z][a-z0-9-]{1,62}[a-z0-9]$`. |
| **TN-003** | A tenant slug is immutable once created. It cannot be updated. |
| **TN-004** | A tenant `domain` (if provided) must be unique across all tenants, including soft-deleted tenants. |
| **TN-005** | A tenant is created with status `active` by default. |
| **TN-006** | Valid tenant statuses are: `active`, `suspended`, `inactive`. |
| **TN-007** | Only an `active` tenant can be suspended. Transition: `active` → `suspended`. |
| **TN-008** | Only a `suspended` tenant can be activated. Transition: `suspended` → `active`. |
| **TN-009** | A suspended tenant's users cannot log in. All authentication attempts must be rejected with a tenant-suspended error. |
| **TN-010** | Suspending a tenant does not delete any data. All data remains intact and is accessible upon reactivation. |
| **TN-011** | Soft-deleting a tenant does not cascade-delete users, roles, or other data. It sets `deleted_at` and prevents all operations on the tenant. |
| **TN-012** | A soft-deleted tenant cannot be activated, suspended, or updated. |
| **TN-013** | Tenant settings are key-value pairs scoped to a single tenant. The combination of (`tenant_id`, `key`) must be unique. |
| **VL-001** | All UUID fields must be valid UUIDv4 format. |
| **VL-002** | Email addresses must conform to RFC 5322 format. |
| **VL-003** | All required string fields must be non-empty after trimming whitespace. |
| **VL-004** | Pagination `page` must be ≥ 1. Default: 1. |
| **VL-005** | Pagination `per_page` must be between 1 and 100 (inclusive). Default: 20. |
| **VL-006** | String fields with a `max` constraint must not exceed the defined limit. |
| **VL-007** | Enum fields must contain one of the allowed values. Invalid values must be rejected with a 400 error. |
| **VL-008** | Request bodies must be valid JSON. Malformed JSON must be rejected with a 400 error. |
| **VL-009** | Unknown fields in request bodies should be silently ignored (not cause errors). |
| **AU-001** | Login requires three fields: `email`, `password`, and `tenant_slug`. All three are mandatory. |
| **AU-002** | The system must resolve the tenant by `tenant_slug` before authenticating the user. If the tenant is not found, return a generic "invalid credentials" error. Do not reveal whether the tenant exists. |
| **AU-003** | The system must verify the user exists within the resolved tenant. If the user is not found, return a generic "invalid credentials" error. Do not reveal whether the user exists. |
| **AU-004** | The system must compare the provided password against the stored `password_hash` using bcrypt. If the password is incorrect, return a generic "invalid credentials" error. |
| **AU-005** | Authentication must be rejected if the tenant status is not `active`. Return error: "tenant is suspended or inactive." |
| **AU-006** | Authentication must be rejected if the user status is not `active`. Return error: "account is not active." |
| **AU-007** | On successful authentication, the system must issue a JWT access token containing: `user_id`, `tenant_id`, `email`, `roles`, `exp`, `iat`. |
| **AU-008** | The JWT access token must expire after 15 minutes. |
| **AU-009** | On successful authentication, the system must issue a refresh token (opaque, random, hashed before storage). |
| **AU-010** | On successful authentication, the system must create a session record. |
| **AU-011** | On logout, the system must revoke the current session and the associated refresh token. |
| **AU-012** | Failed login attempts must be recorded in the audit log. |
| **AU-013** | Successful login attempts must be recorded in the audit log. |
| **AU-014** | The system must update the user's `last_login_at` timestamp on successful login. |
| **AZ-001** | All API endpoints (except public endpoints) require a valid JWT access token in the `Authorization: Bearer <token>` header. |
| **AZ-002** | Public endpoints that do not require authentication: `POST /auth/login`, `POST /auth/forgot-password`, `POST /auth/reset-password`. |
| **AZ-003** | The JWT must be validated for: signature integrity, expiration, required claims (`user_id`, `tenant_id`). |
| **AZ-004** | After JWT validation, the system must verify the user still exists and is active. |
| **AZ-005** | After JWT validation, the system must verify the tenant still exists and is active. |
| **AZ-006** | The `tenant_id` from the JWT must match the tenant resolved from the subdomain. Cross-tenant access is forbidden. |
| **AZ-007** | For protected endpoints, the system must check that the authenticated user has the required permission (`resource:action`) via their assigned roles. |
| **AZ-008** | If the user lacks the required permission, return 403 Forbidden. |
| **AZ-009** | Tenant management endpoints (`/api/v1/tenants`) are accessible only to the JAAS Super Admin. |
| **RB-001** | Roles are scoped to a tenant. The same role name can exist in different tenants. |
| **RB-002** | Role names must be unique within a tenant. Case-insensitive comparison. |
| **RB-003** | System roles (`is_system = true`) cannot be deleted or renamed. They can only have permissions modified. |
| **RB-004** | When a tenant is created, the following system roles must be auto-created: `tenant_admin`, `member`. |
| **RB-005** | A user can have multiple roles within a tenant. |
| **RB-006** | A role can have multiple permissions. |
| **RB-007** | Assigning a role to a user that already has that role is idempotent — no error, no duplicate. |
| **RB-008** | Assigning a permission to a role that already has that permission is idempotent — no error, no duplicate. |
| **RB-009** | When a role is deleted, all `user_roles` and `role_permissions` entries for that role must also be removed. |
| **RB-010** | Revoking a role from a user does not revoke the user's other roles. |
| **RB-011** | Permissions define what actions can be performed on what resources. Format: `resource:action` (e.g., `users:create`, `roles:delete`). |
| **RB-012** | Removing a permission from a role takes effect immediately. Existing JWTs still contain the old roles, but the permission check happens at request time against the database. |
| **PW-001** | Passwords must be at least 8 characters long. |
| **PW-002** | Passwords must be hashed using bcrypt with a cost factor of 12 before storage. |
| **PW-003** | Plaintext passwords must never be logged, returned in API responses, or stored. |
| **PW-004** | The password reset token must be a cryptographically random string (at least 32 bytes). |
| **PW-005** | The password reset token must be hashed (SHA-256) before storage. The raw token is sent to the user via email. |
| **PW-006** | A password reset token expires after 1 hour. |
| **PW-007** | A password reset token can only be used once. After use, `used_at` is set and the token is invalidated. |
| **PW-008** | Requesting a password reset for a non-existent email must return a success response (do not reveal whether the email exists). |
| **PW-009** | When a password is reset, all existing sessions and refresh tokens for that user must be revoked. |
| **PW-010** | The `confirm_password` field in the reset request must match the `new_password` field. |
| **SS-001** | A session is created on every successful login. |
| **SS-002** | A session records: `user_id`, `tenant_id`, `ip_address`, `user_agent`, `expires_at`. |
| **SS-003** | Sessions expire after 24 hours by default. |
| **SS-004** | A user can have multiple concurrent sessions (e.g., different devices). |
| **SS-005** | Revoking a session sets `revoked_at` to the current timestamp. It does not delete the record. |
| **SS-006** | A revoked session cannot be used for any operations. |
| **SS-007** | When a user is deactivated, all their sessions must be revoked. |
| **SS-008** | When a tenant is suspended, active session checks should fail at the middleware level (tenant status check). |
| **RT-001** | Refresh tokens are opaque, cryptographically random strings (at least 32 bytes). |
| **RT-002** | Refresh tokens are hashed (SHA-256) before storage. The raw token is returned to the client. |
| **RT-003** | Refresh tokens expire after 7 days. |
| **RT-004** | When a refresh token is used, it must be revoked and a new refresh token issued (token rotation). |
| **RT-005** | If a revoked refresh token is presented, the system must revoke ALL refresh tokens and sessions for that user (compromise detection). This is called "refresh token reuse detection." |
| **RT-006** | A refresh token is bound to a specific user and tenant. It cannot be used across tenants. |
| **RT-007** | On logout, the refresh token associated with the session must be revoked. |
| **MF-001** | Supported MFA types: `totp`, `sms`, `email`. |
| **MF-002** | A user can have one MFA configuration per type. |
| **MF-003** | MFA must be verified before it is enabled. The `verified_at` timestamp must be set. |
| **MF-004** | MFA secrets must be encrypted at rest. |
| **MF-005** | If MFA is enabled for a user, the login flow must require a second factor after credential validation. |
| **MF-006** | MFA configuration is tenant-scoped. |
| **SD-001** | Soft delete sets `deleted_at` to the current timestamp. The record is not physically removed. |
| **SD-002** | Soft-deleted records are excluded from all default queries. |
| **SD-003** | Soft deletes apply to: `tenants`, `users`, `roles`. |
| **SD-004** | Soft deletes do NOT apply to: `permissions`, `user_roles`, `role_permissions`, `sessions`, `refresh_tokens`, `password_reset_tokens`, `mfa_configs`, `tenant_settings`, `audit_logs`. |
| **SD-005** | Unique constraints must consider soft-deleted records to prevent reuse of slugs, domains, and emails during the soft-deleted state. |
| **SD-006** | A soft-deleted record can be restored by setting `deleted_at` to null (admin operation, not exposed via API in v1). |
| **AD-001** | Audit logs are immutable. No UPDATE or DELETE operations are permitted. |
| **AD-002** | Audit logs do not have `updated_at` or `deleted_at` fields. |
| **AD-003** | Every state-changing API request must produce an audit log entry. |
| **AD-004** | Audit log entries must include: `tenant_id`, `user_id` (nullable for system actions), `action`, `resource`, `resource_id`, `metadata` (JSONB), `ip_address`, `user_agent`, `created_at`. |
| **AD-005** | The `action` field uses a verb format: `tenant.created`, `user.login.success`, `user.login.failed`, `role.assigned`, `permission.revoked`, etc. |
| **AD-006** | The `metadata` field stores action-specific context as JSON (e.g., changed fields, old values, new values). |
| **AD-007** | Audit logs for failed authentication must not include the attempted password. |
| **AD-008** | Audit log queries must be filtered by `tenant_id` to prevent cross-tenant data leakage. |
| EC-T001 | Create tenant with a slug that already exists | Return error: "tenant slug already exists" | 409 Conflict |
| EC-T002 | Create tenant with a slug that belongs to a soft-deleted tenant | Return error: "tenant slug already exists" (soft-deleted slugs are reserved) | 409 Conflict |
| EC-T003 | Create tenant with a domain that already exists | Return error: "domain already in use" | 409 Conflict |
| EC-T004 | Create tenant with an empty slug | Return validation error: "slug is required" | 400 Bad Request |
| EC-T005 | Create tenant with a slug containing uppercase or special characters | Return validation error: "slug must be lowercase alphanumeric with hyphens" | 400 Bad Request |
| EC-T006 | Get tenant by non-existent ID | Return error: "tenant not found" | 404 Not Found |
| EC-T007 | Get tenant by ID that belongs to a soft-deleted tenant | Return error: "tenant not found" | 404 Not Found |
| EC-T008 | Update tenant slug (immutable field) | Ignore the slug field in the request body; do not update it; do not error | 200 OK |
| EC-T009 | Activate an already active tenant | Return error: "tenant is already active" | 409 Conflict |
| EC-T010 | Activate a soft-deleted tenant | Return error: "tenant not found" | 404 Not Found |
| EC-T011 | Suspend an already suspended tenant | Return error: "tenant is already suspended" | 409 Conflict |
| EC-T012 | Suspend a soft-deleted tenant | Return error: "tenant not found" | 404 Not Found |
| EC-T013 | Update a soft-deleted tenant | Return error: "tenant not found" | 404 Not Found |
| EC-T014 | Create tenant with slug that is a reserved word (e.g., `api`, `admin`, `www`, `mail`, `app`) | Return error: "slug is reserved" | 409 Conflict |
| EC-T015 | List tenants when no tenants exist | Return empty array with pagination metadata showing 0 total | 200 OK |
| EC-A001 | Login with non-existent tenant slug | Return generic error: "invalid credentials" (do not reveal tenant existence) | 401 Unauthorized |
| EC-A002 | Login with non-existent email within a valid tenant | Return generic error: "invalid credentials" (do not reveal email existence) | 401 Unauthorized |
| EC-A003 | Login with wrong password | Return generic error: "invalid credentials" | 401 Unauthorized |
| EC-A004 | Login with correct credentials but tenant is suspended | Return error: "tenant is suspended" | 403 Forbidden |
| EC-A005 | Login with correct credentials but user status is `inactive` | Return error: "account is not active" | 403 Forbidden |
| EC-A006 | Login with correct credentials but user status is `invited` | Return error: "account is not active" | 403 Forbidden |
| EC-A007 | Login with correct credentials but user status is `suspended` | Return error: "account is not active" | 403 Forbidden |
| EC-A008 | Login with correct credentials but user is soft-deleted | Return generic error: "invalid credentials" | 401 Unauthorized |
| EC-A009 | Login with empty email | Return validation error: "email is required" | 400 Bad Request |
| EC-A010 | Login with empty password | Return validation error: "password is required" | 400 Bad Request |
| EC-A011 | Login with empty tenant_slug | Return validation error: "tenant_slug is required" | 400 Bad Request |
| EC-A012 | Logout without a valid session | Return error: "session not found" | 401 Unauthorized |
| EC-A013 | Logout with an already revoked session | Return error: "session already revoked" | 401 Unauthorized |
| EC-A014 | Access protected endpoint with expired JWT | Return error: "token expired" | 401 Unauthorized |
| EC-A015 | Access protected endpoint with malformed JWT | Return error: "invalid token" | 401 Unauthorized |
| EC-A016 | Access protected endpoint with JWT signed by a different key | Return error: "invalid token" | 401 Unauthorized |
| EC-A017 | Access protected endpoint with JWT for a user that no longer exists | Return error: "user not found" | 401 Unauthorized |
| EC-A018 | Access protected endpoint with JWT for a user that has been deactivated | Return error: "account is not active" | 403 Forbidden |
| EC-A019 | JWT tenant_id does not match subdomain tenant_id | Return error: "unauthorized" | 403 Forbidden |
| EC-A020 | Concurrent logins from different devices | Allow — create separate sessions for each device | 200 OK |
| EC-R001 | Refresh with a valid refresh token | Issue new access token and new refresh token. Revoke old refresh token. | 200 OK |
| EC-R002 | Refresh with an expired refresh token | Return error: "refresh token expired" | 401 Unauthorized |
| EC-R003 | Refresh with a revoked refresh token (reuse detection) | Revoke ALL refresh tokens and sessions for the user. Return error: "refresh token reused — all sessions revoked" | 401 Unauthorized |
| EC-R004 | Refresh with a token that does not exist in the database | Return error: "invalid refresh token" | 401 Unauthorized |
| EC-R005 | Refresh with an empty token | Return validation error: "refresh_token is required" | 400 Bad Request |
| EC-R006 | Refresh with a valid token but user has been deactivated since token was issued | Revoke all tokens and sessions. Return error: "account is not active" | 403 Forbidden |
| EC-R007 | Refresh with a valid token but tenant has been suspended since token was issued | Return error: "tenant is suspended" | 403 Forbidden |
| EC-P001 | Forgot password with a valid, active email | Generate token, publish event for email delivery. Return success message. | 200 OK |
| EC-P002 | Forgot password with a non-existent email | Return the same success message as EC-P001 (do not reveal email existence) | 200 OK |
| EC-P003 | Forgot password with email of a soft-deleted user | Return the same success message (do not reveal email existence) | 200 OK |
| EC-P004 | Forgot password with email of an inactive user | Return the same success message (do not reveal email existence) | 200 OK |
| EC-P005 | Multiple forgot-password requests for the same email within a short period | Allow — generate a new token each time. Previous tokens remain valid until expiry. Consider rate limiting. | 200 OK |
| EC-P006 | Reset password with a valid, unused token | Reset password, mark token as used, revoke all sessions and refresh tokens | 200 OK |
| EC-P007 | Reset password with an expired token | Return error: "token has expired" | 400 Bad Request |
| EC-P008 | Reset password with an already-used token | Return error: "token has already been used" | 400 Bad Request |
| EC-P009 | Reset password with a non-existent token | Return error: "invalid token" | 400 Bad Request |
| EC-P010 | Reset password where `new_password` and `confirm_password` don't match | Return validation error: "passwords do not match" | 400 Bad Request |
| EC-P011 | Reset password with a password shorter than 8 characters | Return validation error: "password must be at least 8 characters" | 400 Bad Request |
| EC-P012 | Rapid repeated forgot-password requests (potential abuse) | Rate limit: max 5 requests per email per hour | 429 Too Many Requests |
| EC-U001 | Create user with an email that already exists in the tenant | Return error: "email already exists" | 409 Conflict |
| EC-U002 | Create user with an email that exists in a different tenant | Allow — emails are unique per tenant, not globally | 201 Created |
| EC-U003 | Create user with an email that belongs to a soft-deleted user in the same tenant | Return error: "email already exists" (soft-deleted emails are reserved) | 409 Conflict |
| EC-U004 | Create user with a password shorter than 8 characters | Return validation error: "password must be at least 8 characters" | 400 Bad Request |
| EC-U005 | Create user with invalid email format | Return validation error: "invalid email format" | 400 Bad Request |
| EC-U006 | Create user with non-existent role IDs | Return error: "one or more role IDs are invalid" | 400 Bad Request |
| EC-U007 | Create user with role IDs from a different tenant | Return error: "one or more role IDs are invalid" | 400 Bad Request |
| EC-U008 | Get user from a different tenant than the authenticated tenant | Return error: "user not found" (tenant isolation) | 404 Not Found |
| EC-U009 | Update a soft-deleted user | Return error: "user not found" | 404 Not Found |
| EC-U010 | Deactivate a user that is already inactive | Return error: "user is already inactive" | 409 Conflict |
| EC-U011 | Deactivate a user and they try to use an existing JWT | Middleware must check user status on every request — return "account is not active" | 403 Forbidden |
| EC-U012 | Invite a user with an email that already exists in the tenant | Return error: "email already exists" | 409 Conflict |
| EC-U013 | List users with page number exceeding total pages | Return empty array with pagination metadata | 200 OK |
| EC-RL001 | Create role with a name that already exists in the tenant | Return error: "role name already exists" | 409 Conflict |
| EC-RL002 | Create role with a name that exists in a different tenant | Allow — role names are unique per tenant | 201 Created |
| EC-RL003 | Delete a system role (`is_system = true`) | Return error: "system roles cannot be deleted" | 403 Forbidden |
| EC-RL004 | Delete a role that has users assigned | Delete the role and remove all `user_roles` entries for that role | 200 OK |
| EC-RL005 | Delete a non-existent role | Return error: "role not found" | 404 Not Found |
| EC-RL006 | Update a system role's name | Return error: "system role name cannot be changed" | 403 Forbidden |
| EC-RL007 | Assign a role to a user that already has that role | Idempotent — return success, do not create duplicate entry | 200 OK |
| EC-RL008 | Assign a non-existent role to a user | Return error: "role not found" | 404 Not Found |
| EC-RL009 | Assign a role from a different tenant to a user | Return error: "role not found" (tenant isolation) | 404 Not Found |
| EC-PM001 | Assign a permission to a role that already has it | Idempotent — return success, do not create duplicate entry | 200 OK |
| EC-PM002 | Assign a non-existent permission ID to a role | Return error: "one or more permission IDs are invalid" | 400 Bad Request |
| EC-PM003 | Remove a permission from a role, user has an active session with that permission | Permission check at request time will deny access. JWT is not revoked — it will work until it expires, but permission checks are real-time. | — |
| EC-M001 | Enable MFA of the same type twice for a user | Return error: "MFA type already configured" | 409 Conflict |
| EC-M002 | Enable MFA without verifying it first | Return error: "MFA must be verified before enabling" | 400 Bad Request |
| EC-M003 | MFA verification with incorrect code | Return error: "invalid MFA code" | 401 Unauthorized |
| EC-M004 | MFA verification with expired TOTP code | Return error: "invalid MFA code" | 401 Unauthorized |
| EC-M005 | Login with MFA enabled but no MFA code provided | Return partial response indicating MFA is required; do not issue tokens yet | 200 OK (with MFA challenge) |
| EC-RC001 | Two concurrent requests try to create a tenant with the same slug | The database unique constraint ensures only one succeeds. The second receives 409 Conflict. |
| EC-RC002 | Two concurrent requests try to create a user with the same email in the same tenant | The database unique constraint ensures only one succeeds. The second receives 409 Conflict. |
| EC-RC003 | A role is deleted while a user is being assigned that role | The foreign key constraint or a pre-check prevents the assignment. Return error: "role not found." |
| EC-RC004 | A user is deactivated while they are in the middle of a refresh token exchange | The refresh completes or fails based on timing. Next request will fail at middleware level. |
| EC-RC005 | Two concurrent refresh token requests with the same token | One succeeds (rotates the token); the other encounters a revoked token and triggers reuse detection, revoking all tokens. |
| EC-RC006 | A tenant is suspended while a user is mid-request | The current request may complete. Subsequent requests will be rejected by tenant status middleware. |
| EC-DB001 | Database connection failure during login | Return 503 Service Unavailable |
| EC-DB002 | Database timeout during a write operation | Return 503 Service Unavailable. The transaction is rolled back. |
| EC-DB003 | Unique constraint violation on any field | Return 409 Conflict with a descriptive message |
| EC-DB004 | Foreign key violation (e.g., reference to non-existent tenant) | Return 400 Bad Request with a descriptive message |
| EC-DB005 | Transaction deadlock between concurrent operations | Retry the transaction once. If it fails again, return 503 Service Unavailable. |
| EC-SA001 | Brute-force login attempts on a single account | Rate limit: max 10 failed login attempts per account per 15-minute window. Return 429 Too Many Requests after threshold. |
| EC-SA002 | Credential stuffing (many accounts, automated login) | Rate limit by IP: max 100 login attempts per IP per 15-minute window. Return 429. |
| EC-SA003 | JWT tampering (modified payload or signature) | JWT signature verification fails. Return 401 Unauthorized. |
| EC-SA004 | SQL injection via input fields | Parameterized queries via GORM prevent SQL injection. Return 400 if input validation fails. |
| EC-SA005 | XSS via stored string fields (name, description) | Input sanitization on write. Output encoding on read. All responses are JSON (no HTML rendering). |
| EC-SA006 | Enumeration attack via forgot-password endpoint | Always return the same success response regardless of whether the email exists. |
| EC-SA007 | Enumeration attack via login endpoint | Always return the same "invalid credentials" error regardless of which field is wrong. |
| EC-SA008 | Privilege escalation by modifying JWT claims | JWT signature verification prevents claim modification. Claims are verified against the database on each request. |
| EC-SA009 | Cross-tenant access by modifying tenant_id in request | JWT tenant_id is validated against subdomain. All queries are filtered by JWT's tenant_id, not request body. |
| EC-SA010 | Replay attack with a stolen access token | Short token lifetime (15 min) limits damage. Implement token blacklisting on logout for immediate revocation. |
| EC-FR001 | RabbitMQ is unavailable when an event needs to be published | Log the failure. The synchronous operation (e.g., user creation) completes successfully. The event is published asynchronously with retry (see events.md). |
| EC-FR002 | Redis is unavailable for session caching | Fall back to PostgreSQL for session validation. Log the cache miss. |
| EC-FR003 | Email delivery fails for password reset | The token is still created. The user can request a new reset. The failed delivery should be logged and retried. |
| EC-FR004 | Application crash mid-transaction | PostgreSQL transaction rollback ensures no partial writes. |
| EC-FR005 | Migration failure on a production database | The migration tool (golang-migrate) supports rollback. The `.down.sql` file reverses the change. |
| AC-DB-001 | All 12 database tables exist in PostgreSQL after running migrations | Run migrations; `\dt` shows all tables |
| AC-DB-002 | All migrations are reversible — running `.down.sql` drops all tables cleanly | Run up, then down, then up again without errors |
| AC-DB-003 | UUID primary keys are used on every table | `\d+ {table}` shows `uuid` type for `id` column |
| AC-DB-004 | `tenants.slug` has a unique index | `\di` shows unique index on `slug` |
| AC-DB-005 | `tenants.domain` has a unique index | `\di` shows unique index on `domain` |
| AC-DB-006 | `users` has a unique composite index on `(tenant_id, email)` | `\di` shows composite unique index |
| AC-DB-007 | `roles` has a unique composite index on `(tenant_id, name)` | `\di` shows composite unique index |
| AC-DB-008 | `permissions` has a unique composite index on `(resource, action)` | `\di` shows composite unique index |
| AC-DB-009 | `user_roles` has a unique composite index on `(user_id, role_id, tenant_id)` | `\di` shows composite unique index |
| AC-DB-010 | `role_permissions` has a unique composite index on `(role_id, permission_id, tenant_id)` | `\di` shows composite unique index |
| AC-DB-011 | `refresh_tokens.token_hash` has a unique index | `\di` shows unique index |
| AC-DB-012 | `password_reset_tokens.token_hash` has a unique index | `\di` shows unique index |
| AC-DB-013 | `mfa_configs` has a unique composite index on `(user_id, type)` | `\di` shows composite unique index |
| AC-DB-014 | Soft delete (`deleted_at`) is present on `tenants`, `users`, `roles` | Column exists with nullable timestamp type |
| AC-DB-015 | `audit_logs` has no `updated_at` or `deleted_at` columns | `\d+ audit_logs` confirms absence |
| AC-DB-016 | All foreign keys are properly defined and cascading correctly | Attempt to insert with invalid FK → error |
| AC-DB-017 | All tenant-scoped tables have an index on `tenant_id` | `\di` shows index |
| AC-API-001 | `POST /api/v1/tenants` creates a tenant and returns 201 | API test |
| AC-API-002 | `GET /api/v1/tenants` returns paginated tenant list and 200 | API test |
| AC-API-003 | `GET /api/v1/tenants/{id}` returns a single tenant and 200 | API test |
| AC-API-004 | `PATCH /api/v1/tenants/{id}` updates tenant fields and returns 200 | API test |
| AC-API-005 | `POST /api/v1/tenants/{id}/activate` activates a suspended tenant and returns 200 | API test |
| AC-API-006 | `POST /api/v1/tenants/{id}/suspend` suspends an active tenant and returns 200 | API test |
| AC-API-007 | `POST /api/v1/auth/login` returns JWT + refresh token on valid credentials | API test |
| AC-API-008 | `POST /api/v1/auth/login` returns 401 on invalid credentials | API test |
| AC-API-009 | `POST /api/v1/auth/logout` revokes session and returns 200 | API test |
| AC-API-010 | `POST /api/v1/auth/refresh` rotates tokens and returns new pair | API test |
| AC-API-011 | `POST /api/v1/auth/forgot-password` returns 200 regardless of email existence | API test |
| AC-API-012 | `POST /api/v1/auth/reset-password` resets password with valid token | API test |
| AC-API-013 | `POST /api/v1/users` creates a user and returns 201 | API test |
| AC-API-014 | `GET /api/v1/users` returns paginated user list scoped to tenant | API test |
| AC-API-015 | `GET /api/v1/users/{id}` returns a single user and 200 | API test |
| AC-API-016 | `PATCH /api/v1/users/{id}` updates user profile and returns 200 | API test |
| AC-API-017 | `POST /api/v1/users/{id}/deactivate` deactivates user and revokes sessions | API test |
| AC-API-018 | `POST /api/v1/roles` creates a role and returns 201 | API test |
| AC-API-019 | `GET /api/v1/roles` returns paginated role list scoped to tenant | API test |
| AC-API-020 | `PATCH /api/v1/roles/{id}` updates role and returns 200 | API test |
| AC-API-021 | `DELETE /api/v1/roles/{id}` deletes non-system role and returns 200 | API test |
| AC-API-022 | `GET /api/v1/permissions` returns all permissions | API test |
| AC-API-023 | `POST /api/v1/roles/{id}/permissions` assigns permissions to role | API test |
| AC-API-024 | `POST /api/v1/users/{id}/roles` assigns roles to user | API test |
| AC-API-025 | All endpoints return consistent JSON error format | Review all error responses |
| AC-API-026 | All list endpoints support `page` and `per_page` query parameters | API test |
| AC-AUTH-001 | Login with valid credentials returns JWT access token with correct claims | Decode JWT and verify claims |
| AC-AUTH-002 | JWT expires after 15 minutes | Attempt API call with token after 15 min → 401 |
| AC-AUTH-003 | Login with wrong password returns 401 with generic message | API test |
| AC-AUTH-004 | Login with non-existent email returns 401 with same generic message | API test |
| AC-AUTH-005 | Login against a suspended tenant returns 403 | API test |
| AC-AUTH-006 | Login with an inactive user returns 403 | API test |
| AC-AUTH-007 | Logout revokes session and refresh token | Verify in database |
| AC-AUTH-008 | Refresh token rotation works — new token pair issued, old revoked | API test + DB check |
| AC-AUTH-009 | Refresh token reuse triggers security response — all tokens/sessions revoked | API test |
| AC-AUTH-010 | Forgot-password creates token and publishes event | DB check + event verification |
| AC-AUTH-011 | Reset-password with valid token updates password hash | DB check |
| AC-AUTH-012 | Reset-password revokes all sessions and refresh tokens | DB check |
| AC-AUTH-013 | Expired password reset token is rejected | API test |
| AC-AUTH-014 | Used password reset token is rejected | API test |
| AC-AUTH-015 | `last_login_at` is updated on successful login | DB check |
| AC-ISO-001 | User in Tenant A cannot see users in Tenant B | API test with two tenants |
| AC-ISO-002 | User in Tenant A cannot see roles in Tenant B | API test |
| AC-ISO-003 | User in Tenant A cannot assign roles from Tenant B | API test |
| AC-ISO-004 | JWT with Tenant A's tenant_id cannot access Tenant B's subdomain | API test → 403 |
| AC-ISO-005 | Audit logs are filtered by tenant_id | DB query verification |
| AC-EVT-001 | `TenantCreated` event is published after tenant creation | RabbitMQ consumer test |
| AC-EVT-002 | `TenantActivated` event is published after activation | RabbitMQ consumer test |
| AC-EVT-003 | `TenantSuspended` event is published after suspension | RabbitMQ consumer test |
| AC-EVT-004 | `UserCreated` event is published after user creation | RabbitMQ consumer test |
| AC-EVT-005 | `UserInvited` event is published after invitation | RabbitMQ consumer test |
| AC-EVT-006 | `UserDeactivated` event is published after deactivation | RabbitMQ consumer test |
| AC-EVT-007 | `RoleAssigned` event is published after role assignment | RabbitMQ consumer test |
| AC-EVT-008 | `PermissionAssigned` event is published after permission assignment | RabbitMQ consumer test |
| AC-EVT-009 | `PasswordReset` event is published after password reset | RabbitMQ consumer test |
| AC-EVT-010 | `SessionRevoked` event is published after logout | RabbitMQ consumer test |
| AC-EVT-011 | All events contain correct envelope fields (event_id, type, occurred_at, tenant_id) | Payload inspection |
| AC-EVT-012 | Events are published after database transaction commits, not before | Test with transaction rollback |
| AC-AUD-001 | Login success creates audit log entry | DB query |
| AC-AUD-002 | Login failure creates audit log entry | DB query |
| AC-AUD-003 | User creation creates audit log entry | DB query |
| AC-AUD-004 | User deactivation creates audit log entry | DB query |
| AC-AUD-005 | Role assignment creates audit log entry | DB query |
| AC-AUD-006 | Tenant suspension creates audit log entry | DB query |
| AC-AUD-007 | Audit log entries cannot be updated | Attempt UPDATE → verify failure or no-op |
| AC-AUD-008 | Audit log entries cannot be deleted | Attempt DELETE → verify failure or no-op |
| AC-AUD-009 | Audit logs include IP address and user agent | DB query inspection |
| AC-AUD-010 | Failed login audit logs do not contain passwords | DB query inspection |
| AC-SEC-001 | Passwords are stored as bcrypt hashes | DB query: `password_hash` starts with `$2a$12$` |
| AC-SEC-002 | No API response includes password hash | Inspect all user-related API responses |
| AC-SEC-003 | Rate limiting blocks excessive login attempts | Send 11 requests in 15 min → 429 on 11th |
| AC-SEC-004 | Rate limiting blocks excessive forgot-password requests | Send 6 requests in 1 hour → 429 on 6th |
| AC-SEC-005 | Forgot-password returns same response for existent and non-existent emails | Compare responses → identical |
| AC-SEC-006 | Login returns same error for wrong email and wrong password | Compare error messages → identical |
| AC-SEC-007 | JWT signature tampering is detected | Modify JWT payload → 401 |
| AC-SEC-008 | Expired JWT is rejected | Wait 15 min → 401 |
| AC-SEC-009 | Cross-tenant JWT usage is rejected | JWT for Tenant A used on Tenant B subdomain → 403 |
| AC-SEC-010 | SQL injection via input fields is prevented | Attempt injection payload → no SQL execution |
| AC-PERF-001 | Login endpoint responds in < 300ms (p95) | Load test with k6 or wrk |
| AC-PERF-002 | Single-resource GET responds in < 200ms (p95) | Load test |
| AC-PERF-003 | List endpoint with 100 items responds in < 500ms (p95) | Load test |
| AC-PERF-004 | System handles 100 concurrent login requests without errors | Load test |
| AC-CQ-001 | `go build ./...` compiles without errors | CI build |
| AC-CQ-002 | `go vet ./...` passes without warnings | CI build |
| AC-CQ-003 | `go test ./internal/identity/...` passes | CI test |
| AC-CQ-004 | `go test -race ./internal/identity/...` passes | CI test with race detector |
| AC-CQ-005 | Service layer test coverage ≥ 90% | `go test -cover` report |
| AC-CQ-006 | Overall module test coverage ≥ 80% | `go test -cover` report |
| AC-CQ-007 | No TODO comments in production code | `grep -r "TODO" internal/identity/` returns empty |
| AC-CQ-008 | All exported functions have documentation comments | `golint` or `go doc` check |
| AC-CQ-009 | Consistent error handling — no raw `panic()` calls | Code review |
| AC-CQ-010 | All database queries use parameterized inputs (no string concatenation) | Code review |

**Decision log (decisions.md, verbatim)**

2026-09-30 — Scaffold: 17-file operational bundle layered onto the 16-file spec base on branch loop_engineering (docs-only, zero production impact). Reason: MASTER_PROMPT Step 4 — loop needs module context to start. Alternative: write all operational files during G0-1. Tradeoff: bundle exists before code; content filled with real repo facts, no placeholders.
2026-09-30 — plan.md owns phase truth; current-goal.md mirrors CURRENT row; implementation-checklist.md remains canonical task list; todo.md is the operational mirror with Goal IDs. Reason: three artifacts serve different readers (loop tick, human scan, phase tracker) without triple-maintenance — goal text written once in plan.md, mirrored by reference.
2026-10-02 — identity.Module gained read-only accessors (UserService, AuditService, UserRoleRepository, RolePermissionRepository, TenantResolver, AuthMiddleware) for dependent modules. Reason: HR/Organization route registration needs identity-owned middleware + RBAC repos without re-constructing them; single DI instance kept in main.go. Alternative: main.go rebuilds repos/services by hand (duplicate wiring). Tradeoff: internal Go API surface grows; no REST contract change. Mirrors module-1 decisions.md entry.
### D-HARDEN-1 (2026-10-06): login latency budget
bcrypt cost 12 (NFR-SEC001) costs ~400 ms; the <300 ms login p95 budget is relaxed to <600 ms. Security takes precedence.
### D-HARDEN-2: platform routes use an operator key, not a JWT
There is no super-admin identity in the data model. `/tenants*` are guarded by `X-Platform-Key` (config `platform.admin_key`, env `PLATFORM_ADMIN_KEY`), constant-time compared, fail-closed when unset. A real super-admin realm can replace it later without changing the tenant API.
### D-HARDEN-3: SQL migrations are the schema authority
`database.RunMigrations` applies `migrations/*.sql` (embedded) at start-up. GORM AutoMigrate (`database.auto_migrate: true`) is legacy/dev only because its constraints drift from the SQL files (it made business codes globally unique).
### D-HARDEN-4: cross-module rules are synchronous hooks
No broker consumer exists, so user deactivation converges org mappings and employee profiles via in-process hooks inside the same DB transaction (`UserDeactivationConverger`). Events are still published for future consumers.
2026-10-08 — Cross-module record (from HR/Attendance & Leave3-10): HR/Attendance & Leave boot inserts `attendance:read`, `attendance:manage`, `attendance:approve` into the M0 Identity `permissions` table (ON CONFLICT DO NOTHING) using identity `models.Permission`. No M0 Identity code or REST contract changes. Reason: no deploy-time permission seed exists (AS-007). Tradeoff: HR/Attendance & Leave depends on the permissions table shape (resource, action, description).
##### Changelog
2026-09-30 — Added: operational bundle (agent.md, current-goal.md G0-1, plan.md P1 CURRENT, current-status.md, todo.md, handoff.md, assumptions/decisions/changelog/files entries). Reason: MASTER_PROMPT Step 4 on branch loop_engineering. Tests: structure validation. Impact: docs only, no contract change.
### 2026-10-06 — hardening pass (branch `fix/module-0-2-hardening`)
Verification-console findings fixed and pinned by tests; full list in the hardening report in `docs/modules/m0-identity-access/testing.md`. Highlights: logout + refresh revocation, platform-key protected tenant routes, admin bootstrap, permission seed, non-escalating grants, audit read API, `/auth/me`, activate/unassign endpoints, atomic transactions, standard error envelope, atomic rate limiter, SQL migration runner.
Sensors: build/vet/gofmt clean; unit + integration + golden + security tests pass; coverage 86.5 % (services 93-97 %); migrations up/down/up clean.
2026-10-20 — Access model adopted (owner-approved; `docs/ARCHITECTURE.md` §17, `plan.md` §3 B0): scoped grants,
role inheritance, predefined and automatic roles, per-user grants with expiry and blocks (blocks win), access
version for cache invalidation, explain-access API. Legacy `RequirePermission` requires tenant scope so no existing
route widens; scoped routes opt in through `RequireScopedPermission`.

### m1-hr-management/contexts/organization  _(from specification.md)_

**Requirements and rules (ID-tagged table rows)**

| ID | Text | … |
|---|---|---|
| FR-D001 | Create a department in the authenticated tenant: name (required, 1–100), code (optional, unique per tenant, slug-like `^[a-z0-9-]{2,32}$`), description, parent department (optional, same tenant). |
| FR-D002 | Department names unique per tenant (case-insensitive). Codes unique per tenant when set. |
| FR-D003 | Get department by ID (tenant-scoped, 404 otherwise). |
| FR-D004 | List departments paginated (`page`/`per_page`, defaults 1/20, max 100). Support `parent_id` filter + `root_only` flag. |
| FR-D005 | Update name/description/code/parent (code immutable once set — ignored, not error; mirrors TN slug rule). |
| FR-D006 | Parent must exist in the same tenant; parent chain must stay acyclic (see FR-H). |
| FR-D007 | Deactivate department (soft status `active`/`inactive`, default active). Deactivation blocked (409) while active teams or active mappings reference it unless `force=true` cascades deactivation to child departments only (never deletes). |
| FR-D008 | Soft delete departments (`deleted_at`); slugs/codes/emails of soft-deleted rows stay reserved. |
| FR-T001 | Create a team in the authenticated tenant: name (1–100), code (optional, unique per tenant), department (required, same tenant), lead user (optional, must be an active user in the tenant per M0 Identity UserService). |
| FR-T002 | Team names unique per department (case-insensitive); codes unique per tenant when set. |
| FR-T003 | Get team by ID (tenant-scoped). |
| FR-T004 | List teams paginated with `department_id` filter. |
| FR-T005 | Update name/description/code/lead (code immutable once set). Moving a team across departments allowed; audit old/new department. |
| FR-T006 | Deactivate team (status flag). Blocked while active mappings reference it unless reassigned first (409 with conflicting mapping IDs). |
| FR-T007 | Soft delete teams; names/codes of soft-deleted rows stay reserved. |
| FR-DG001 | Create a designation (job title/level): title (1–100), code (optional, unique per tenant), level (optional int ≥1, higher = senior), description. |
| FR-DG002 | Titles unique per tenant (case-insensitive); codes unique per tenant when set. |
| FR-DG003 | Get/List (paginated, `level` ordering), Update (code immutable), Deactivate, soft delete — same lifecycle as departments. |
| FR-DG004 | Deactivation blocked while active mappings reference the designation (409) unless mappings are migrated first. |
| FR-H001 | Departments form a forest: `parent_department_id` nullable; exactly one path to a root; cycles rejected (409 `hierarchy.cycle`). |
| FR-H002 | Team→department edge is many-to-one and mandatory. |
| FR-H003 | Setting a parent validates: parent exists, same tenant, not self, not a descendant (cycle check in tx). |
| FR-H004 | Depth cap: 10 levels. Exceeding returns 422 with the offending chain. |
| FR-H005 | Moving a subtree revalidates the whole subtree (depth + acyclicity) atomically; failure rolls back. |
| FR-M001 | Map a user to org: `user_id` (must resolve via M0 Identity UserService: exists, same tenant, active), `department_id` (optional), `team_id` (optional), `designation_id` (optional), `is_primary` bool, `manager_user_id` (optional, active same-tenant user, must not equal self). |
| FR-M002 | One primary mapping per user per tenant (`unique(tenant_id, user_id, is_primary=true)` effectively; enforced in service + partial unique index). Secondary mappings allowed. |
| FR-M003 | Manager chain must stay acyclic (user A cannot transitively manage themselves); validated in tx, 409 on cycle. |
| FR-M004 | Team mapping requires the team to belong to the mapping's department when both set (or department derived from team). |
| FR-M005 | On M0 Identity `UserDeactivated` event: mark mappings `inactive`, clear `manager_user_id` references to that user (reassign or null with audit). |
| FR-M006 | On M0 Identity `UserCreated`: no auto-mapping (explicit mapping only — decision, see decisions.md). |
| FR-M007 | Unmap (deactivate mapping) with reason + audit; history retained (no hard delete). |
| FR-O001 | `GET /org-chart` returns the tenant forest: departments with nested children, teams per department, headcount per node (active mappings), lead user per team. |
| FR-O002 | `GET /org-chart?user_id=` returns the user's chain to root (user → team → department → ancestors) + direct reports (users with `manager_user_id` = user). |
| FR-O003 | Read model is computed from transactional tables (no separate store in v1); response cached 60s in Redis, invalidated on any FR-D/T/DG/H/M write. |
| FR-O004 | Depth/pruning params: `max_depth` (default full, max 10), `include_inactive=false` default. |
| FR-E001 | Consume `identity.tenant.created` → no-op record check (tenant exists via M0 Identity read; org starts empty). |
| FR-E002 | Consume `identity.tenant.suspended` → freeze all writes (403 `tenant.suspended` on mutating endpoints until active). |
| FR-E003 | Consume `identity.user.created` → no auto-mapping (FR-M006). |
| FR-E004 | Consume `identity.user.deactivated` → FR-M005. |
| FR-E005 | Produce `organization.department.created/deactivated`, `organization.team.created/moved/deactivated`, `organization.mapping.created/updated/deactivated`, `organization.hierarchy.moved` on exchange `jaas.organization.events` (topic, durable), routing `organization.{resource}.{action}`, M0 Identity envelope (event_id/type/version/occurred_at/producer/tenant_id/correlation_id/payload). Publish after commit; queue-down never blocks API (fallback table + retry, same pattern as M0 Identity). |
| NFR-P001 | Single-resource GET | p95 <200ms |
| NFR-P002 | List (100 items) | p95 <500ms |
| NFR-P003 | Org-chart full tenant (≤5k nodes) | p95 <800ms (cached <100ms) |
| NFR-P004 | DB query | p95 <50ms |
| NFR-S001 | Tenants/tenant scale inherits M0 Identity (10k tenants); per tenant ≤10k departments, ≤50k teams, ≤500k mappings | — |
| NFR-SEC001 | All endpoints require JWT; tenant_id from JWT == subdomain tenant; every query tenant-scoped | — |
| NFR-SEC002 | Hierarchy/mapping writes require `organization:update`; reads require `organization:read` (new permissions, seeded) | — |
| NFR-D001 | Multi-table writes in tx (move subtree, mapping+manger updates) | — |
| NFR-D002 | UUID PKs; soft deletes for departments/teams/designations; mappings append-only (status flag, no hard delete) | — |

**Requirements and rules (ID-tagged bullets)**

- EC-01 create dept with parent in another tenant → 404 parent. EC-02 set parent = self → 400. EC-03 set parent = descendant → 409 cycle. EC-04 move subtree exceeding depth 10 → 422 + chain. EC-05 map inactive/nonexistent user → 404 (same message; no enumeration of user existence beyond M0 Identity semantics). EC-06 map user from another tenant → 404. EC-07 two primaries for one user → 409 (second wins only via explicit `make_primary` swap in tx). EC-08 manager = self → 400; manager cycle A→B→A → 409. EC-09 team move across departments with mappings → allowed, mappings follow team, audit records old/new. EC-10 tenant suspended mid-write → tx rolls back, 403. EC-11 concurrent same-name creates → unique constraint wins, loser 409. EC-12 org-chart on empty tenant → `{data: [], meta}` 200. EC-13 `UserDeactivated` race with mapping write → mapping write fails closed (user inactive → 404/409), event converges state.

**Decision log (decisions.md, verbatim)**

> Part of **M1 HR Management** (bounded context `organization`). Formerly the stand-alone "module-1-organization" bundle; module numbers in older text were renamed to the canonical M0–M12 / HR-context names.
Format: Decision / Reason / Alternatives / Tradeoff.
2026-09-30 — Scaffold: 17-file bundle created before design (docs-only, zero production impact). Reason: loop engineering needs module context to start. Alternative: design all modules upfront. Tradeoff: bundles for unbuilt modules stay thin until P1.
2026-10-01 (P1 design) —
- Decision: Separate HR/Organization with own tables (not columns on users). Reason: org has independent lifecycle, hierarchy invariants, 4 consumers. Alternatives: extend user model; separate microservice. Tradeoff: more tables, but identity stays decoupled and deployable.
- Decision: Logical (service-checked) FK to users, not hard DB FK. Reason: M0 Identity owns users; hard cross-module FKs freeze independent deploys. Alternatives: hard FK; duplicate user snapshot. Tradeoff: convergence via UserDeactivated event + write-time checks instead of DB enforcement.
- Decision: No auto-mapping on UserCreated (explicit mapping only). Reason: org placement is a human decision with RBAC + audit; auto-placement guesses wrong silently. Alternatives: auto-map to default dept/team. Tradeoff: extra mapping step per user, recorded in UX.
- Decision: Computed org-chart + 60s cache (not materialized). Reason: v1 scale fits compute; invalidation trivial; no dual-write drift. Alternatives: materialized path/closure table. Tradeoff: revisit if NFR-P003 breaches.
- Decision: Append-only mappings (status flag, no hard delete). Reason: history needed for audit/payroll/approvals. Alternatives: hard delete + audit only. Tradeoff: table growth; mitigated by tenant scoping + pagination.
- Decision: In-process Go calls to M0 Identity (same deployable v1), not HTTP. Reason: single binary, no network hop, contract = Go interfaces + DTOs. Alternatives: HTTP between modules. Tradeoff: revisit at multi-service split (RS256 + HTTP contracts already shaped for it).
2026-10-02 (P2 wiring) —
- Decision: M0 Identity exposes read-only accessors on identity.Module (services, RBAC repos, TenantResolver/AuthMiddleware builders); org consumes them in cmd/main.go. Reason: single DI graph, no duplicated wiring. Alternative: main.go constructs identity internals directly. Tradeoff: small internal API surface addition to M0 Identity (no REST change). Recorded in module-0 decisions.md too.
- Decision: Live schema currently comes from GORM AutoMigrate at boot; raw SQL pairs (000013..000017) are the production path once golang-migrate is wired. Consequence: case-insensitive uniqueness (lower(name)) and partial indexes exist only in SQL — AutoMigrate creates plain unique indexes; service layer enforces case-insensitive pre-checks. Tradeoff: acceptable at dev boot; must wire golang-migrate before production (tracked in current-status known issues).
- Decision: routes.go signature takes (dbForAudit, auditSvc) and builds auditMW internally; module.go passes them through from main.go. Reason: matches identity route style; audit middleware stays identity-owned.
2026-10-09 — Cross-module record (from M7 Approvals D6-03): organization.Module gains read-only accessors DepartmentService() and DesignationService() so Recruitment can validate job references. No behaviour or REST change.
##### Changelog
2026-09-30 — Created: 17-file doc bundle scaffold with real dependency metadata. Reason: MASTER_PROMPT Step 4 on branch loop_engineering. Tests: structure validation. Impact: docs only, no contract change.
2026-10-01 — P1 design DONE: specification (FR-D/T/DG/H/M/O/E), architecture, connections C1..C9, goldens G1..G10, security/testing/files/decisions/assumptions, contracts frozen. Reason: G1-1 (M0 Identity merged 3af3d09; context01 next task). Tests: structure check (docs phase). Impact: contracts frozen for P2; no code yet.
### 2026-10-06 — hardening pass (branch `fix/module-0-2-hardening`)
Verification-console findings fixed and pinned by tests; full list in the hardening report in `docs/modules/m0-identity-access/testing.md`. Highlights: duplicate codes → 409, active-team deactivation rule, org-chart cache invalidation, team move keeps mappings consistent, tenant-scoped unique codes, optional deactivate bodies.
Sensors: build/vet/gofmt clean; unit + integration + golden + security tests pass; coverage 86.5 % (services 93-97 %); migrations up/down/up clean.

### m1-hr-management/contexts/employee  _(from specification.md)_

**Requirements and rules (ID-tagged table rows)**

| ID | Text | … |
|---|---|---|
| FR-EP001 | Create an employee profile in the authenticated tenant: `user_id` (1:1 with active M0 Identity User), `employee_code` (required, unique per tenant, e.g. `EMP-001`), `first_name`, `last_name`, `display_name`, `gender`, `date_of_birth`, `marital_status`, `blood_group`, `avatar_url`. |
| FR-EP002 | `employee_code` must be unique per tenant (case-insensitive) across active and soft-deleted rows. Format: `^[A-Z0-9-]{2,32}$`. |
| FR-EP003 | Get employee profile by ID (tenant-scoped, 404 otherwise). Standard response returns sanitized personal/contact data. |
| FR-EP004 | List employees paginated (`page`/`per_page`, defaults 1/20, max 100) with filters: `status`, `department_id`, `search` (name or code). |
| FR-EP005 | Update profile details (first/last/display name, gender, marital status, blood group, avatar). Employee code is immutable once set. |
| FR-EP006 | Soft delete employee (`deleted_at`); employee codes remain reserved. |
| FR-EP007 | Self-service profile endpoint `GET /employees/me` returns current user's profile and mapping snapshot. |
| FR-ED001 | Onboard employment terms: `employment_type` (`full_time`, `part_time`, `contract`, `intern`), `joining_date` (required), `probation_end_date`, `confirmation_date`, `notice_period_days` (default 30). |
| FR-ED002 | Status transitions: `active`, `probation`, `notice`, `terminated`, `resigned`, `on_leave`. Validated via state transition matrix. |
| FR-ED003 | Confirm employee (`probation` -> `active`): sets `confirmation_date`, appends timeline event `confirmed`. |
| FR-ED004 | Initiate exit (`resigned` or `notice`): sets `resignation_date`, calculates tentative `exit_date` from notice period. |
| FR-ED005 | Finalize termination/exit (`terminated` or `resigned`): sets `exit_date`, `exit_reason`, marks status `terminated`/`resigned`, automatically triggers M0 Identity user deactivation or status sync. |
| FR-EC001 | Maintain contact info: `personal_email`, `work_phone`, `personal_phone`, `current_address`, `permanent_address`, `emergency_contacts` (JSONB array of name, relation, phone). |
| FR-EC002 | Self-service contact update (`PATCH /employees/me`): employee can update own personal phone, current address, and emergency contacts. |
| FR-ES001 | Store sensitive statutory information: `tax_id` (PAN / SSN / National Tax Identifier), `national_id`, `bank_name`, `bank_account_number`, `bank_routing_swift`. |
| FR-ES002 | Field-level privacy: statutory endpoints require `employee:read_sensitive` / `employee:update_sensitive`. |
| FR-ES003 | Standard employee GET masks bank account (`****1234`) and tax ID (`*****5678`) unless sensitive access is authorized. |
| FR-DOC001 | Attach employee documents: `document_type` (`id_proof`, `contract`, `resume`, `education`, `certification`, `nda`), `file_name`, `file_url`, `file_size`, `mime_type`. |
| FR-DOC002 | List documents for employee (tenant-scoped). |
| FR-DOC003 | Verify document: `POST /employees/{id}/documents/{doc_id}/verify` records `verified_at` and `verified_by` user ID (requires `employee:admin`). |
| FR-TL001 | System-generated immutable timeline events: `hired`, `probation_completed`, `promoted`, `transferred`, `resigned`, `terminated`. |
| FR-TL002 | Read timeline: `GET /employees/{id}/timeline` returns chronological events. |
| FR-EV001 | Consume `identity.user.deactivated` -> marks employee status as `inactive`/`terminated`, appends exit timeline event. |
| FR-EV002 | Produce `employee.created`, `employee.updated`, `employee.status.changed`, `employee.exited`, `employee.document.verified` on exchange `jaas.employee.events` (topic, durable). |
| FR-EV003 | Reliable publishing via Outbox pattern (`employee_events_outbox`). |
| NFR-P001 | Single-employee GET | p95 < 200ms |
| NFR-P002 | Paginated employee directory (100 items) | p95 < 500ms |
| NFR-P003 | Database single query latency | p95 < 50ms |
| NFR-SEC001 | Multi-tenant isolation on every query (`WHERE tenant_id = ?`) | Mandatory |
| NFR-SEC002 | Sensitive field masking for bank/tax PII | Mandatory |
| NFR-SEC003 | RBAC permission enforcement (`employee:read`, `employee:create`, `employee:update`, `employee:read_sensitive`, `employee:admin`) | Mandatory |

**Decision log (decisions.md, verbatim)**

> Part of **M1 HR Management** (bounded context `employee`). Formerly the stand-alone "module-2-employee" bundle; module numbers in older text were renamed to the canonical M0–M12 / HR-context names.
| Date | ID | Context | Decision | Impact |
|---|---|---|---|---|
| 2026-10-05 | DEC-E001 | 1:1 relationship between User and EmployeeProfile | Enforce unique `user_id` per tenant on `EmployeeProfile`. An employee must link to an authenticated User. | Clear identity separation while maintaining referential integrity. |
| 2026-10-05 | DEC-E002 | Sensitive PII separation | Store bank accounts and government tax IDs in a separate `employee_statutory` table with strict RBAC (`employee:read_sensitive`). | Zero leak of sensitive financial PII on general employee directory queries. |
| 2026-10-05 | DEC-E003 | Atomic Onboarding | Create profile, employment details, contact records, and initial timeline event inside a single DB transaction. | Prevents dangling or incomplete employee records. |
| 2026-10-05 | DEC-E004 | Reactive user deactivation convergence | When M0 Identity user is deactivated, sync employee status to `inactive`/`terminated` and log timeline exit. | Guarantees consistency between authentication state and HR status. |
### D-HARDEN-5 (2026-10-06): sensitive data and lifecycle
* `GET /employees/:id/statutory` is masked unless `?unmasked=true` AND `employee:read_sensitive`; unmasked reads are audited.
* Status transitions follow a matrix (`resigned`/`terminated` are terminal); exit (`resigned`/`terminated`) deactivates the M0 Identity user in the same transaction.
* `emergency_contacts` is a JSON array of `{name, relation, phone}`.
##### Changelog
- **2026-10-05:** Frozen P1 design bundle (specification FRs, Clean architecture, connections edge table, candidates G1..G10). Kicked off P2 backend implementation.
### 2026-10-06 — hardening pass (branch `fix/module-0-2-hardening`)
Verification-console findings fixed and pinned by tests; full list in the hardening report in `docs/modules/m0-identity-access/testing.md`. Highlights: PII unmask requires employee:read_sensitive (audited), status state machine + exit/confirmation effects, synchronous user-deactivation convergence (FR-EV001), emergency-contact validation, document verify ownership, statutory upsert fix.
Sensors: build/vet/gofmt clean; unit + integration + golden + security tests pass; coverage 86.5 % (services 93-97 %); migrations up/down/up clean.

### m1-hr-management/contexts/attendance  _(from specification.md)_

**Requirements and rules (ID-tagged table rows)**

| ID | Text | … |
|---|---|---|
| FR-SH001 | Create a shift: `name` (1–100), `code` (optional, `^[a-z0-9-]{2,32}$`, unique per tenant), `start_time` / `end_time` (`HH:MM`, 24h), `grace_period_mins` (0–240, default 15), `break_duration_mins` (0–480, default 60), `full_day_minutes` / `half_day_minutes` (optional, 1–1440; defaults AT-010), `timezone` (IANA, default `UTC`). |
| FR-SH002 | Shift names unique per tenant (case-insensitive). Codes unique per tenant when set. |
| FR-SH003 | `is_night_shift` is derived, never client-set: true when `end_time <= start_time` (span crosses midnight). `start_time == end_time` is rejected. |
| FR-SH004 | Get / list shifts (paginated, `status` filter, ordered by name). |
| FR-SH005 | Update name, times, grace, break, thresholds, timezone. `code` is immutable once set (ignored, not error). Updates affect only records computed afterwards; existing records are not recomputed (AT-021). |
| FR-SH006 | Deactivate a shift (`status=inactive`). Blocked with 409 while any assignment covering today or a future date references it. |
| FR-SA001 | Assign an employee to a shift from `effective_from` (date) to optional `effective_to` (date, inclusive, ≥ from). Shift must be active; employee must exist in tenant. |
| FR-SA002 | Assignments for one employee never overlap. If an open-ended assignment starts before the new `effective_from`, it is closed at `effective_from − 1 day` in the same transaction; any other overlap → 409 `ASSIGNMENT_OVERLAP`. |
| FR-SA003 | List assignments by employee (newest first). The active assignment for a date is the one whose range covers it. |
| FR-PU001 | Self-service punch `POST /attendance/punch` with `type` (`in`/`out`), optional `latitude` (−90..90), `longitude` (−180..180), `device_id` (≤100), `source` (`web` default, `mobile`). Punch time is the server clock (AT-001). |
| FR-PU002 | Punch rules AT-002..AT-006. The response returns the punch and the recomputed record. |
| FR-AR001 | One record per (tenant, employee, attendance date). Created by the first IN of that date or by an approved regularization. |
| FR-AR002 | Totals recomputed on every punch and regularization: `first_punch_in`, `last_punch_out`, `total_work_minutes`, `total_break_minutes`, `late_minutes`, `overtime_minutes`, `status` (AT-006..AT-010). |
| FR-AR003 | Self views: today's state (`/attendance/me/today`: record or null, open-session flag, active shift) and own records by date range (`/attendance/me`, max 62 days). |
| FR-AR004 | Admin views (`attendance:read`): records filtered by employee / date range / status, paginated; record detail with its punches. |
| FR-AR005 | Daily summary (`attendance:read`) for a date: `total_employees`, `present`, `late`, `half_day`, `on_leave`, `absent` (AT-020). |
| FR-RG001 | Employee requests a correction for a date: `attendance_date`, `requested_punch_in`, `requested_punch_out` (RFC3339), `reason` (1–500). Rules AT-014, AT-015. |
| FR-RG002 | Employee lists own requests (filter `status`) and cancels own pending requests (AT-018). |
| FR-RG003 | Approvers (`attendance:approve`) list all requests (filter `status`, `employee_id`), approve (optional `comment`) or reject (required `comment`). Rules AT-016, AT-017. |
| FR-EV001 | Produce on exchange `jaas.attendance.events` (topic, durable), routing `attendance.{resource}.{action}`, standard envelope (event_id, event_type, routing_key, version `1.0`, occurred_at, producer `attendance-service`, tenant_id, correlation_id, payload): `attendance.punch.in`, `attendance.punch.out`, `attendance.regularization.requested`, `attendance.regularization.approved`, `attendance.regularization.rejected`. |
| FR-EV002 | Transactional outbox: the event row is written to `attendance_events_outbox` in the same DB transaction as the state change; published after commit; a relay retries unpublished rows (30s tick, max 10 attempts). Queue down never blocks the API. |
| FR-EV003 | Payloads carry IDs, dates, times and computed totals only — never IP, device id or coordinates. |
| AT-001 | Punch time = server UTC clock. No client time field exists in the contract. |
| AT-002 | Caller must have an employee profile in the tenant (HR/Employee `GetByUserID`) → else 404 `EMPLOYEE_NOT_FOUND`. Profile status must be `active`, `probation` or `notice` → else 403 `EMPLOYEE_NOT_ACTIVE`. |
| AT-003 | Sessions alternate: `in` while a session is open → 409 `ALREADY_PUNCHED_IN`; `out` with no open session → 409 `NOT_PUNCHED_IN`. |
| AT-004 | A punch within 60 s of the employee's previous punch → 409 `DUPLICATE_PUNCH`. Evaluated before AT-003 (D3-13). |
| AT-005 | Attendance date for `in` = local date (shift timezone, else UTC) of the punch; for a night shift, an `in` whose local time-of-day is before `end_time + 4h` belongs to the previous date. `out` always attaches to the record holding the open session. |
| AT-006 | An open session older than 20 h is abandoned: `out` → 409 `SESSION_EXPIRED` (regularize instead); a new `in` is allowed and starts a new session. Totals ignore abandoned sessions. Work = Σ(out − in) of closed sessions; break = Σ gaps between an `out` and the next `in` on the same record. Superseded punches (AT-017) are ignored. |
| AT-007 | `late_minutes` = max(0, first_in − (shift start + grace)) in the shift timezone on the attendance date; 0 when no shift is assigned. |
| AT-008 | Expected minutes = shift span − break, span = end − start (+1440 when night shift). No shift → 480. |
| AT-009 | `overtime_minutes` = max(0, work − expected). |
| AT-010 | Status: open session → `present`. Otherwise work ≥ full-day threshold → `present`; work ≥ half-day threshold → `half_day`; else `absent`. Defaults: full = expected, half = expected / 2 (shift fields override). Statuses `on_leave`, `holiday`, `week_off` are reserved for HR/Leave / calendar writers and are never set by punches. |
| AT-011 | Assignment ranges never overlap per employee (FR-SA002). Dates are calendar dates (no time). |
| AT-012 | Shift deactivation blocked while referenced by an assignment covering today or later (FR-SH006). |
| AT-013 | `HH:MM` 00:00–23:59; start ≠ end; timezone must load via IANA database; thresholds: half ≤ full when both set. |
| AT-014 | Regularization: `attendance_date` ≤ today (requester's shift tz) and ≥ today − 30 days → else 400 `REGULARIZATION_WINDOW`; `requested_punch_out` > `requested_punch_in`; span ≤ 20 h; the local date of `requested_punch_in` must equal `attendance_date` (night shifts: or the day after attendance_date for a punch before end+4h — same AT-005 attribution). |
| AT-015 | At most one `pending` regularization per employee per date → 409 `REGULARIZATION_PENDING`. |
| AT-016 | Approve/reject only `pending` → else 409 `REGULARIZATION_NOT_PENDING`. A reviewer whose own employee profile is the requester → 403 `SELF_APPROVAL_FORBIDDEN`. Reject requires `comment` (1–500). |
| AT-017 | Approval (one transaction): lock employee; create the record if missing; mark all its punches `is_superseded=true`; insert `in`/`out` punches with source `regularization`; recompute totals; set `is_regularized=true`; set request `approved`, `reviewer_user_id`, `reviewed_at`; write outbox event. |
| AT-018 | Cancel: requester's own `pending` request only → status `cancelled`. |
| AT-019 | Records and punches are never deleted (append-only; corrections supersede). |
| AT-020 | Summary for date D: `total_employees` = HR/Employee employees with status active/probation/notice; `present` = records on D with status `present` or open session; `half_day`; `late` = records with late_minutes > 0; `on_leave` = records with status `on_leave`; `absent` = max(0, total − present − half_day − on_leave). |
| AT-021 | Shift/assignment changes never rewrite existing records; recomputation happens only on that record's next punch or approved regularization. |
| AT-022 | Per-employee writes (punch, approval) serialize through a transaction-scoped advisory lock keyed on (tenant_id, employee_profile_id). |
| A1 | POST `/attendance/punch` | self (employee) | `PunchRequest{type, latitude?, longitude?, device_id?, source?}` | 201 `PunchResult{punch, record}` |
| A2 | GET `/attendance/me/today` | self | — | 200 `TodayResponse{attendance_date, record?, open_session, shift?}` |
| A3 | GET `/attendance/me?from&to` | self | dates `YYYY-MM-DD`, default last 7 days, max 62 | 200 `[]RecordResponse` |
| A4 | POST `/attendance/regularizations` | self | `CreateRegularizationRequest{attendance_date, requested_punch_in, requested_punch_out, reason}` | 201 `RegularizationResponse` |
| A5 | GET `/attendance/regularizations/me?status&page&per_page` | self | — | 200 list + meta |
| A6 | POST `/attendance/regularizations/{id}/cancel` | self (owner) | — | 200 `RegularizationResponse` |
| A7 | GET `/attendance/records?employee_id&from&to&status&page&per_page` | `attendance:read` | — | 200 list + meta |
| A8 | GET `/attendance/records/{id}` | `attendance:read` | — | 200 `RecordDetailResponse{record, punches[]}` |
| A9 | GET `/attendance/summary?date` | `attendance:read` | default today UTC | 200 `SummaryResponse` |
| A10 | GET `/attendance/regularizations?status&employee_id&page&per_page` | `attendance:read` | — | 200 list + meta |
| A11 | POST `/attendance/regularizations/{id}/approve` | `attendance:approve` | `ReviewRequest{comment?}` | 200 `RegularizationResponse` |
| A12 | POST `/attendance/regularizations/{id}/reject` | `attendance:approve` | `ReviewRequest{comment}` | 200 `RegularizationResponse` |
| S1 | POST `/shifts` | `attendance:manage` | `CreateShiftRequest` | 201 `ShiftResponse` |
| S2 | GET `/shifts?status&page&per_page` | `attendance:read` | — | 200 list + meta |
| S3 | GET `/shifts/{id}` | `attendance:read` | — | 200 `ShiftResponse` |
| S4 | PATCH `/shifts/{id}` | `attendance:manage` | `UpdateShiftRequest` (pointer fields) | 200 `ShiftResponse` |
| S5 | POST `/shifts/{id}/deactivate` | `attendance:manage` | — | 200 `ShiftResponse` |
| S6 | POST `/shifts/{id}/assignments` | `attendance:manage` | `AssignShiftRequest{employee_id, effective_from, effective_to?}` | 201 `AssignmentResponse` |
| S7 | GET `/shift-assignments?employee_id&page&per_page` | `attendance:read` | `employee_id` required | 200 list + meta |
| NFR-P001 | Punch p95 < 200 ms (one tx: lock + ≤ 4 queries) |
| NFR-P002 | Record / regularization lists (100 items) p95 < 500 ms |
| NFR-P003 | Summary for 10k employees p95 < 500 ms (aggregate SQL, no N+1) |
| NFR-P004 | DB query p95 < 50 ms (indexes §8) |
| NFR-SEC001 | Every query `tenant_id`-scoped; every id resolved in caller tenant |
| NFR-SEC002 | Self endpoints derive employee from JWT user only — no employee_id parameter accepted |
| NFR-D001 | Multi-table writes in one transaction; events via outbox in the same transaction |

**Decision log (decisions.md, verbatim)**

> Part of **M1 HR Management** (bounded context `attendance`). Formerly the stand-alone "module-3-attendance" bundle; module numbers in older text were renamed to the canonical M0–M12 / HR-context names.
Format: Decision / Reason / Alternatives / Tradeoff.
2026-09-30 — Scaffold: 17-file bundle created before design (docs-only).
2026-10-08 (P1 design)
- D3-01 Scope = masterplan HR/Attendance (Attendance, Shifts & Time Tracking); Leave moves to HR/Leave. Reason: newest architecture doc (SYSTEM_ARCHITECTURE_MASTERPLAN, 2026-10-06) and the frontend Phase-2 plan split them; combined module would exceed one-service-at-a-time loop size. Alternative: DEPENDENCY-GRAPH combined "Attendance & Leave". Tradeoff: DEPENDENCY-GRAPH/MASTER_PROMPT registry rows updated; HR/Leave bundle must be re-scoped from Projects to Leave before G4-1 (user confirmation requested).
- D3-02 Key everything by `employee_profile_id` (HR/Employee), not user_id. Reason: attendance is employment data; history survives user deactivation. Tradeoff: self endpoints resolve user→employee per request (one indexed lookup).
- D3-03 Shift times as minutes-from-midnight ints + per-shift IANA timezone. Reason: integer night-shift math, no TIME-type driver quirks, correct lateness across zones. Alternative: Postgres TIME + tenant timezone setting. Tradeoff: API converts HH:MM.
- D3-04 Server clock only for punches. Reason: anti-backdating (AS-T1). Corrections go through regularization approval.
- D3-05 Controllers do not hold `*gorm.DB`; services own a `txRunner` and open real transactions. Reason: M0 Identity, HR/Organization, HR/Employee controllers pass raw DB (no atomicity); HR/Attendance must not copy that. Tradeoff: differs from sibling modules' constructor shape (documented here, not a contract change).
- D3-06 Transactional outbox written inside the business tx + relay goroutine. Reason: FR-EV002 guarantee; HR/Organization, HR/Employee publish-then-fallback can lose events on crash. Tradeoff: one extra insert per write; relay started from cmd/main.go.
- D3-07 Audit after commit, best-effort. Reason: an audit insert failing inside a Postgres tx aborts the whole tx; audit must never fail the API (docs/SECURITY.md §7).
- D3-08 Supersede punches on regularization approval (append-only). Reason: AT-019 audit trail.
- D3-09 Advisory xact lock per employee for punch/approve. Reason: no row to lock before the first punch of a day; lock is released automatically at commit.
- D3-10 Permission seed: HR/Attendance boot inserts `attendance:read|manage|approve` into M0 Identity `permissions` (ON CONFLICT DO NOTHING) through identity `models.Permission`. Reason: AS-007 says permissions seeded at deploy and nothing seeds module permissions today. Cross-module touch recorded in module-0 decisions.md. Alternative: SQL-only seed migration. Tradeoff: dev (AutoMigrate) and prod paths both seeded.
- D3-11 Live tests behind `//go:build integration`. Reason: `go test ./...` must stay runnable without Docker; existing untagged org tests in tests/api are left untouched (HR/Organization scope).
- D3-12 Thresholds (`full_day_minutes`, `half_day_minutes`) per shift with derived defaults. Reason: tenants tune tolerance without code; no hidden constants.
- D3-13 (P7) Rule precedence: AT-004 debounce is evaluated before AT-003 alternation. A second tap inside 60 s is a duplicate (409 `DUPLICATE_PUNCH`), not a state error. Found when the live run returned DUPLICATE_PUNCH for in→in within 60 s; the service order is intended, the spec was silent. Live tests backdate punch rows in SQL instead of sleeping.
- D3-14 (P7) Dev/prod schema parity is enforced by model tests: minute counters `type:integer`; `uq_shifts_tenant_name` = (tenant_id, lower(name)) WHERE deleted_at IS NULL; `uq_shifts_tenant_code` WHERE deleted_at IS NULL AND code IS NOT NULL; `uq_attendance_regularizations_pending` partial on status='pending'; `idx_attendance_outbox_published` WHERE published = false. Reason: the live diff showed AutoMigrate missing the AT-015 backstop and the case-insensitive/soft-delete-aware shift uniqueness. Note: AutoMigrate does not replace an existing same-name index, so a dev DB booted before this change needs `DROP INDEX uq_shifts_tenant_name, uq_shifts_tenant_code, idx_attendance_outbox_published` once (done on local jaas_dev 2026-10-09).
- D3-15 (P7) Live suite clears M0 Identity's per-IP login counter in Redis inside the shared `doReq` helper for login calls only. Reason: a full run bootstraps >10 tenants/users from one IP and tripped the 10/15 min limiter (also broke HR/Organization's TestOrgRBAC). Production limiter unchanged.
- D3-16 (P9) Params ≤4 (loop.md) counts business parameters; `ctx context.Context` and the transaction handle `tx *gorm.DB` are infrastructure plumbing and are not counted. All HR/Attendance functions comply under this reading (AST check 2026-10-09); five that did not were refactored (`events.Source`, `auditEntry`, `punchTx` state, `RouteDeps`). Reason: repository methods must take ctx + tx + tenantID by design (tenant isolation, D3-05), so a literal count would force struct-wrapping every query; HR/Employee repositories follow the same shape. Revisit if the human wants the literal rule.
- D3-17 (2026-10-09, from HR/Leave D4-08) HR/Attendance exposes `services.LeaveSync` (MarkLeave/ClearLeave running on the caller's tx under the HR/Attendance per-employee lock) and record source `leave`, fulfilling reserved contract C10. Leave-only records (source `leave`, no punches) are deleted on ClearLeave; records with punches are recomputed.
- D3-18 (2026-10-09) Integration onto M0 Identity, HR/Organization, HR/Employee hardening: migrations renumbered to 000026–000031 (000025 = refresh_token_session_link); module wired in internal/app/time_modules.go — relay lifecycle via app.Options.Background (nil in tests); controller fail() runs shared errors.Normalize; live tests ported to tests/integration (tests/api removed by hardening). Module-owned permission seed (D3-10) kept alongside the M0 Identity DefaultPermissions catalogue.
##### Changelog
2026-09-30 — Created: 17-file doc bundle scaffold. Impact: docs only.
2026-10-08 — P1 design DONE: scope re-defined to Attendance, Shifts & Time Tracking (D3-01); specification/architecture/connections/security/goldens/testing/decisions/assumptions/files frozen. Impact: contracts frozen for P2 (19 endpoints, 6 tables, 5 events). Tests: n/a (docs).
2026-10-08 — P2 DONE: 6 GORM models + constants, models_test (table names, index contracts, append-only, UUID hooks), migrations 000026–000031. Tests: go test ./internal/attendance/models (100% cov). Impact: new tables only, no existing contract changed.
2026-10-09 — P3 DONE: calc engine (goldens G4/G5), DTOs (+G13 pagination clamp), validators. Tests: go test ./internal/attendance/... (calc/dto/validators/models 100%). Impact: none external.
2026-10-09 — P4 DONE: repositories (6 interfaces + GORM impls, advisory lock, FindOrCreate, status aggregate). Tests: build/vet (SQL verified live at P7). Impact: none external.
2026-10-09 — P5 DONE: services (shift, assignment, punch, query, regularization, outbox relay) + events; unit goldens G2/G3/G6/G7/G8/G10/G11/G12 green; services 91.8%. Impact: none external.
2026-10-09 — P6 DONE: controllers + routes (19, RBAC contract test), module.go (directory adapter, permission seed, relay, punch rate limit), cmd/main.go wiring, swagger 19 operations + 24 schemas. Tests: go build/vet ./..., go test ./internal/... PASS; controllers/routes 100%. Impact: new public API (gated GO 2026-10-08); 3 rows in M0 Identity permissions (D3-10).
2026-10-09 — P7 DONE: live goldens G1, G2, G6–G9, G11–G13 PASS on Docker Postgres; migrations up/down/up clean; AutoMigrate↔SQL parity fixed (D3-14: integer columns, partial/expression unique indexes, pending backstop); AT-004 precedence clarified (D3-13); suite-level login limiter reset (D3-15, also unblocks HR/Organization TestOrgRBAC in full runs). Impact: dev DBs created before this need 3 indexes dropped once (see D3-14).
2026-10-09 — P9 DONE (backend CLOSED): DoD §24 walked with evidence; 5 functions refactored to params ≤4 (D3-16); routes↔swagger 19/19; threat→evidence map; live AS-T6 concurrency test (10 parallel INs → 1 session). P8 frontend BLOCKED (no identity shell).
2026-10-09 — Integrated onto M0 Identity, HR/Organization, HR/Employee hardening (branch integrate/m3-m4-on-hardening): migrations renumbered 000026–000037, app wiring, error normalization, integration tests ported; 59/59 PASS.

### m1-hr-management/contexts/leave  _(from specification.md)_

**Requirements and rules (ID-tagged table rows)**

| ID | Text | … |
|---|---|---|
| LV-001 | Caller must have an employee profile (404 `EMPLOYEE_NOT_FOUND`) in a working status `active`/`probation`/`notice` (403 `EMPLOYEE_NOT_ACTIVE`). |
| LV-002 | Type must be active (409 `LEAVE_TYPE_INACTIVE`) and match `applicable_gender` (case-insensitive vs HR/Employee gender; 403 `LEAVE_TYPE_NOT_APPLICABLE`). |
| LV-003 | `start_date ≤ end_date`, both in the current leave year (400 `INVALID_LEAVE_RANGE`). |
| LV-004 | Half day only when `allow_half_day` and `start_date == end_date`; `half_day` ∈ `first_half`/`second_half` (400 `VALIDATION_ERROR`). |
| LV-005 | Notice: if `min_notice_days > 0`, `start_date ≥ today + min_notice_days`; if `min_notice_days == 0`, `start_date ≥ today − 30` (backdating window). Else 400 `LEAVE_NOTICE`. |
| LV-006 | total_days = working days in range (D4-05); with sandwich rule, non-working days strictly between first and last working day are added; half day = 0.50. 0 days → 400 `NO_WORKING_DAYS`. |
| LV-007 | `max_consecutive_days` (if set) ≥ total_days, else 400 `LEAVE_TOO_LONG`. |
| LV-008 | No overlap with the employee's own `pending`/`approved` requests (409 `LEAVE_OVERLAP`). Two half days on the same date with different halves do not overlap. |
| LV-009 | Paid types: `available ≥ total_days` at apply time, else 409 `INSUFFICIENT_BALANCE`. Unpaid types (`is_paid = false`) skip the balance check; days still recorded in `used` on approval (LOP for payroll). |
| LV-010 | Approver ≠ requester (403 `SELF_APPROVAL_FORBIDDEN`); only `pending` can be approved/rejected (409 `LEAVE_NOT_PENDING`). Reject requires a comment. |
| LV-011 | Owner cancel: `pending` any time; `approved` only when `start_date > today` (else 409 `LEAVE_ALREADY_STARTED`); other statuses 409 `LEAVE_NOT_CANCELLABLE`. |
| LV-012 | `total_days` and type rules are snapshotted at apply; later type/holiday edits never change existing requests. |
| LV-013 | Every balance change writes ledger rows in the same transaction; invariant: balance columns == sums of ledger rows per kind. |
| LV-014 | Apply/approve/reject/cancel/adjust take a per-employee advisory xact lock (`leave:<employee_id>`) so balances cannot race. |
| LV-015 | Approve marks each full-day working date `on_leave` in HR/Attendance (creating the day record with source `leave` if missing). Half-day leave does not change attendance status. Cancel of approved leave clears those marks (leave-only records are removed; records with punches are recomputed). |
| LV-016 | Leave days, balances and ledger rows are never hard-deleted; cancellation is a status + reversal row. |
| LV-017 | Adjustment days: non-zero, |days| ≤ 365, reason 1–500 chars; may make available negative only via adjustment (HR correction). |
| L1 | POST `/leave/types` | `leave:manage` | `CreateLeaveTypeRequest` | 201 `LeaveTypeResponse` |
| L2 | GET `/leave/types?status&page&per_page` | authenticated | — | 200 list + meta |
| L3 | GET `/leave/types/{id}` | authenticated | — | 200 `LeaveTypeResponse` |
| L4 | PATCH `/leave/types/{id}` | `leave:manage` | `UpdateLeaveTypeRequest` (pointers; no code) | 200 `LeaveTypeResponse` |
| L5 | POST `/leave/types/{id}/deactivate` | `leave:manage` | — | 200 `LeaveTypeResponse` |
| H1 | POST `/leave/holidays` | `leave:manage` | `CreateHolidayRequest{date, name, is_optional?}` | 201 `HolidayResponse` |
| H2 | GET `/leave/holidays?year` | authenticated | default current year | 200 `[]HolidayResponse` |
| H3 | DELETE `/leave/holidays/{id}` | `leave:manage` | — | 200 `HolidayResponse` |
| B1 | GET `/leave/balances/me` | self | — | 200 `[]BalanceResponse` (all active types, current year) |
| B2 | GET `/leave/balances?employee_id` | `leave:read` | `employee_id` required | 200 `[]BalanceResponse` |
| B3 | POST `/leave/balances/adjust` | `leave:manage` | `AdjustBalanceRequest{employee_id, leave_type_id, days, reason}` | 200 `BalanceResponse` |
| B4 | GET `/leave/ledger?employee_id&leave_type_id&page&per_page` | `leave:read` | `employee_id` required | 200 `[]LedgerEntryResponse` + meta |
| R1 | POST `/leave/requests/preview` | self | `ApplyLeaveRequest` | 200 `PreviewResponse{total_days, available, available_after, working_dates[]}` |
| R2 | POST `/leave/requests` | self | `ApplyLeaveRequest{leave_type_id, start_date, end_date, half_day?, reason}` | 201 `LeaveRequestResponse` |
| R3 | GET `/leave/requests/me?status&page&per_page` | self | — | 200 list + meta |
| R4 | POST `/leave/requests/{id}/cancel` | self (owner) | — | 200 `LeaveRequestResponse` |
| R5 | GET `/leave/requests?status&employee_id&from&to&page&per_page` | `leave:read` | — | 200 list + meta |
| R6 | GET `/leave/requests/{id}` | `leave:read` | — | 200 `LeaveRequestResponse` |
| R7 | POST `/leave/requests/{id}/approve` | `leave:approve` | `ReviewRequest{comment?}` | 200 `LeaveRequestResponse` |
| R8 | POST `/leave/requests/{id}/reject` | `leave:approve` | `ReviewRequest{comment}` | 200 `LeaveRequestResponse` |
| NFR-P001 | Apply/approve p95 < 200 ms for ranges ≤ 31 days (one tx: lock + ≤ 8 queries; attendance sync batched per request) |
| NFR-P002 | Lists p95 < 500 ms at 100 items |
| NFR-SEC001 | Every query tenant-scoped; foreign ids → 404 |
| NFR-SEC002 | Self endpoints derive the employee from the JWT user only |
| NFR-D001 | Request + balance + ledger + attendance + outbox writes in one transaction |
| NFR-D002 | Day arithmetic in integer hundredths; no floating-point in balances |

**Requirements and rules (ID-tagged bullets)**

- FR-LT001 Create a type: name, code (2–10, `^[A-Z0-9_]+$`, uppercased, immutable), is_paid, annual_allowance (0–365), accrual (`annual`|`monthly`), carry_forward_limit (0–365), max_consecutive_days? (1–365), min_notice_days (0–90), allow_half_day, sandwich_rule, applicable_gender (`all`|`male`|`female`).
- FR-LT002 List / get types (any authenticated tenant member — employees must see what they can apply for).
- FR-LT003 Update any field except code; changes apply to future requests and future accruals only.
- FR-LT004 Deactivate: no new requests; existing requests/balances unaffected. Name and code unique per tenant among non-deleted.
- FR-HD001 Create holiday {date, name, is_optional}; one per tenant per date.
- FR-HD002 List holidays by year (authenticated). FR-HD003 Delete (soft) a holiday. Holiday changes never re-price existing requests (total_days is a snapshot, LV-012).
- FR-BL001 A balance row is materialized lazily on first touch (apply, read, adjust) for (employee, type, current year) — no scheduler (D4-06).
- FR-BL002 Accrual on materialization and on every later touch: `annual` → full allowance prorated by joining date; `monthly` → allowance/12 per month from the later of Jan and joining month through the current month, December absorbs the rounding remainder. Each increase writes an `accrual` ledger row and a `leave.accrued` event.
- FR-BL003 Carry-forward at materialization: `min(max(previous year available, 0), carry_forward_limit)` as one `carry_forward` row (opening).
- FR-BL004 HR adjustment: signed non-zero days with reason → `adjustment` row.
- FR-BL005 Self balance view; HR view by employee; ledger view (paginated).
- FR-LR001 Preview (quote): compute total_days and balance impact without saving.
- FR-LR002 Apply: validates LV rules, reserves days (`reserve` row), status `pending`, event `leave.applied`.
- FR-LR003 Approve (`leave:approve`): reserved → used (`release` + `consume` rows), status `approved`, attendance days marked `on_leave` (HR/Attendance port, same tx), event `leave.approved`.
- FR-LR004 Reject (`leave:approve`, comment required): reservation released, event `leave.rejected`.
- FR-LR005 Cancel (owner): pending → release; approved and start date in the future → `reversal` row, attendance leave marks cleared, event `leave.cancelled`. Approved leave that has started cannot be cancelled by the owner (LV-011).
- FR-LR006 Lists: own requests; all requests (filters status, employee_id, from, to); detail.
- FR-EV001 Exchange `jaas.leave.events`, routing keys `leave.applied|approved|rejected|cancelled|accrued`; envelope identical to HR/Attendance (event_id, event_type, routing_key, version, occurred_at, producer=`leave-service`, tenant_id, correlation_id, payload).
- FR-EV002 Written to `leave_events_outbox` inside the business transaction; published after commit; relay retries unpublished rows.
- FR-EV003 Payloads carry ids, dates, days, status only — never the free-text reason or review comment.

**Decision log (decisions.md, verbatim)**

> Part of **M1 HR Management** (bounded context `leave`). Formerly the stand-alone "module-4-leave" bundle; module numbers in older text were renamed to the canonical M0–M12 / HR-context names.
- D4-01 HR/Leave = Leave & Absence (masterplan), following D3-01. The Projects scaffold bundle was renamed to module-4-leave; Projects has no module number until the human assigns one (DEPENDENCY-GRAPH updated).
- D4-02 Ledger is the source of truth; `leave_balances` is a projection updated in the same tx (LV-013). No generated column (masterplan) because AutoMigrate cannot express it and dev/prod parity is enforced (D3-14 style).
- D4-03 Days are integer hundredths in Go (`calc.Days`), NUMERIC(7,2) in SQL. Reason: no float drift in balances (NFR-D002).
- D4-04 Leave year = calendar year of the UTC date; requests must lie in the current year. Reason: simple, matches masterplan `year INT`; cross-year requests are split by the employee. Fiscal-year config deferred.
- D4-05 Weekly off fixed Sat/Sun; one tenant holiday calendar; optional holidays count as working days. Reason: no weekly-off config exists yet (HR/Attendance has none); v1.1.
- D4-06 Lazy accrual/carry-forward on first touch instead of a scheduler. Reason: no job runner in the stack; deterministic and testable.
- D4-07 Single-tier approval by any `leave:approve` holder except the requester. Reason: M7 Approvals owns chains and manager routing; HR/Organization manager data is not exposed as a service.
- D4-08 HR/Attendance gains exported `services.LeaveSync` (MarkLeave/ClearLeave on the caller's tx) and record source `leave` — contract C10 reserved by HR/Attendance P1. Recorded in module-3 decisions.md.
- D4-09 Half-day leave does not change attendance status (attendance computes half_day from work minutes); full-day leave sets `on_leave`. Punches on a leave day win (recompute → present).
- D4-10 (P5) Accrual events (`leave.accrued`) are written to the outbox inside the materializing tx but published by the relay, not inline; request events publish inline after commit. Reason: accrual can happen inside any command (even a balance read) and must never delay or fail it.
- D4-11 (P7) Performance budgets NFR-P001/P002 were not verifiable on the dev host (Docker under memory pressure, ~70 ms/query, login 2–4 s; apply observed 881 ms). Query count per apply is bounded (lock + ≤ 10 statements + holidays). Re-measure on a normal host before production; not claimed as met.
- D4-12 (2026-10-09) Integration onto M0 Identity, HR/Organization, HR/Employee hardening: migrations renumbered to 000032–000037; wiring in internal/app/time_modules.go; errors.Normalize in controllers; live goldens ported to tests/integration/leave_test.go; RBAC golden G9 strengthened with an exact-grant user (leave:approve only).
- D4-13 (2026-10-09, from HR/Payroll D5-03) HR/Leave exposes read-only `services.UnpaidLeave.UnpaidDays` (approved requests of is_paid=false types, clipped to a range; holidays, sandwich and half days honoured) via `leave.Module.UnpaidLeave()`. No writes, no schema change.
##### Changelog
2026-10-09 — P1 DONE: bundle re-scoped Projects → Leave (D4-01); full design frozen.
2026-10-09 — P2 DONE: calc.Days codec, 6 models, migrations 000032–000037 (cycle verified).
2026-10-09 — P3 DONE: calc engine (goldens G2–G4), DTOs (G15), validators — all 100%.
2026-10-09 — P4 DONE: six repositories + advisory lock.
2026-10-09 — P5 DONE: leave services, events, relay, HR/Attendance LeaveSync; goldens green; services 96.3%.
2026-10-09 — P6 DONE: HTTP edge, routes, wiring, permission seed, swagger.
2026-10-09 — P7 DONE: live goldens PASS (11 goldens), full suite 16/16, migration cycle + parity identical.
2026-10-09 — P9 DONE (backend CLOSED): DoD walked with evidence; perf caveat recorded (D4-11).
2026-10-09 — Integrated onto M0 Identity, HR/Organization, HR/Employee hardening (branch integrate/m3-m4-on-hardening): migrations renumbered 000026–000037, app wiring, error normalization, integration tests ported; 59/59 PASS.

### m1-hr-management/contexts/payroll  _(from specification.md)_

**Requirements and rules (ID-tagged table rows)**

| ID | Text | … |
|---|---|---|
| PY-001 | Component amount (full month): fixed = value; percent_of_ctc = monthly CTC × p%; percent_of_basic = BASIC × p%; balance = monthly CTC − Σ other earnings (< 0 → `STRUCTURE_OVERFLOW`). Rounded to paise, half away from zero. |
| PY-002 | Earned amount = full-month amount × payable_days / days_in_month, rounded to paise. Applies to earnings and fixed deductions. |
| PY-003 | Window = [max(month start, joining), min(month end, exit)]; empty window → no payslip. |
| PY-004 | LOP = approved unpaid leave days inside the window (half day 0.5), from HR/Leave; payable = window days − LOP, never below 0. |
| PY-005 | PF (pf_enabled): 12% × min(earned BASIC, ₹15,000), rounded to the rupee. |
| PY-006 | ESI (esi_enabled): if full-month gross ≤ ₹21,000 → 0.75% × earned gross, rounded **up** to the rupee; else 0. |
| PY-007 | Professional Tax (pt_enabled): earned gross ≥ ₹25,000 → ₹200, else 0 (single default slab, D5-06). |
| PY-008 | TDS (tds_enabled), new regime FY 2025-26: annual taxable = 12 × full-month taxable earnings − ₹75,000; slabs 0–4L 0%, 4–8L 5%, 8–12L 10%, 12–16L 15%, 16–20L 20%, 20–24L 25%, >24L 30%; rebate 87A → tax 0 if taxable ≤ ₹12,00,000 (no marginal relief, D5-07); + 4% cess; monthly TDS = annual tax / 12 rounded to the rupee. |
| PY-009 | Net pay = earned gross − (custom deductions + PF + ESI + PT + TDS); never negative (deductions are capped at gross, a `NET_CAPPED` warning is recorded). |
| PY-010 | One run per tenant period; state machine draft → calculated ⇄ (recalculate) → approved → finalized. |
| PY-011 | Recalculate replaces all payslips of the run in one transaction. |
| PY-012 | Approver ≠ last calculator (maker-checker). |
| PY-013 | Employees see only their own payslips of finalized runs; ids of others → 404. |
| PY-014 | Assignments are append-only history; a new assignment closes the open one at effective_from − 1 day. |
| PY-015 | Structures with any assignment keep their components forever (immutable). |
| P1 | POST `/payroll/structures` | `payroll:manage` | structure + components |
| P2 | GET `/payroll/structures?status&page&per_page` | `payroll:read` | |
| P3 | GET `/payroll/structures/{id}` | `payroll:read` | with components |
| P4 | PATCH `/payroll/structures/{id}` | `payroll:manage` | name, description, statutory flags |
| P5 | POST `/payroll/structures/{id}/deactivate` | `payroll:manage` | |
| P6 | POST `/payroll/assignments` | `payroll:manage` | `{employee_id, structure_id, annual_ctc, effective_from}` |
| P7 | GET `/payroll/assignments?employee_id&page&per_page` | `payroll:read` | history |
| P8 | GET `/payroll/assignments/me` | self | current assignment + full-month breakdown |
| P9 | POST `/payroll/assignments/preview` | `payroll:manage` | `{structure_id, annual_ctc}` → breakdown |
| P10 | POST `/payroll/runs` | `payroll:manage` | `{month, year}` |
| P11 | GET `/payroll/runs?status&page&per_page` | `payroll:read` | |
| P12 | GET `/payroll/runs/{id}` | `payroll:read` | totals, warnings |
| P13 | POST `/payroll/runs/{id}/calculate` | `payroll:manage` | |
| P14 | POST `/payroll/runs/{id}/approve` | `payroll:approve` | maker-checker |
| P15 | POST `/payroll/runs/{id}/finalize` | `payroll:approve` | |
| P16 | GET `/payroll/runs/{id}/payslips?page&per_page` | `payroll:read` | |
| P17 | GET `/payroll/runs/{id}/payout.csv` | `payroll:read` | finalized only, `text/csv` |
| P18 | GET `/payroll/payslips/{id}` | `payroll:read` | any payslip |
| P19 | GET `/payroll/payslips/me?page&per_page` | self | finalized only |
| P20 | GET `/payroll/payslips/me/{id}` | self | own + finalized, else 404 |

**Requirements and rules (ID-tagged bullets)**

- FR-ST001 Create a structure {name, description?, pf_enabled, esi_enabled, pt_enabled, tds_enabled, components[]}. Component {code `^[A-Z0-9_]{2,20}$`, name, kind `earning`|`deduction`, calc `fixed`|`percent_of_ctc`|`percent_of_basic`|`balance`, value (rupees for fixed, percent 0–100 for percents; none for balance), taxable (earnings, default true)}.
- FR-ST002 Rules: exactly one `BASIC` earning with calc `fixed` or `percent_of_ctc`; at most one `balance` earning; codes unique in the structure; `percent_of_basic` cannot apply to `BASIC`; deductions may be `fixed` or `percent_of_basic` only.
- FR-ST003 List / get / update (name, description, statutory flags) / deactivate. Components are immutable after create (create a new structure to change pay design, D5-05). Inactive structures cannot be assigned.
- FR-AS001 Assign {employee_id, structure_id, annual_ctc (> 0, ≤ 1,000,000,000), effective_from}: closes the previous current assignment the day before; earlier-than-current effective dates → 409 `ASSIGNMENT_BACKDATED`.
- FR-AS002 Preview {structure_id, annual_ctc} → full-month breakdown incl. statutory (no save). A structure whose fixed earnings exceed monthly CTC → 400 `STRUCTURE_OVERFLOW`.
- FR-AS003 List by employee (`payroll:read`); self view of own current assignment with breakdown.
- FR-RN001 Create run {month, year}: one per tenant period (409 `RUN_EXISTS`); period not in the future (400 `INVALID_PERIOD`). Status `draft`.
- FR-RN002 Calculate (`draft`|`calculated`): (re)builds payslips for every employee with an assignment effective on or before month end whose window is non-empty; status `calculated`; totals stored. Employees whose structure overflows are skipped and listed in `warnings`.
- FR-RN003 Approve (`calculated` → `approved`), maker-checker: approver ≠ the user who last calculated (403 `SELF_APPROVAL_FORBIDDEN`).
- FR-RN004 Finalize (`approved` → `finalized`): payslips become visible to employees; run is immutable.
- FR-RN005 List runs; get run with totals; list its payslips; payout CSV (`finalized` only): `employee_code,employee_name,net_pay`.
- FR-RN006 Invalid transitions → 409 `RUN_STATE`.
- FR-PS001 Payslip: employee snapshot (code, name), structure, annual_ctc, days_in_month, payable_days, lop_days, lines [{code, name, kind, amount}], gross_earnings, total_deductions, net_pay.
- FR-PS002 Self: list/get own payslips of **finalized** runs only. Admin (`payroll:read`): any payslip by id.
- FR-EV001 Exchange `jaas.payroll.events`: `payroll.run_initiated`, `payroll.calculated`, `payroll.approved`, `payroll.finalized`; HR/Attendance, HR/Leave envelope; outbox in-tx; relay.
- FR-EV002 Payloads carry run id, period, employee count and run totals only — never per-employee amounts.

**Errors**

400 VALIDATION_ERROR, INVALID_PERIOD, STRUCTURE_OVERFLOW · 403 FORBIDDEN, SELF_APPROVAL_FORBIDDEN · 404 NOT_FOUND, EMPLOYEE_NOT_FOUND · 409 CONFLICT, RUN_EXISTS, RUN_STATE, STRUCTURE_INACTIVE, ASSIGNMENT_BACKDATED · 500 INTERNAL_ERROR (opaque).

**Decision log (decisions.md, verbatim)**

> Part of **M1 HR Management** (bounded context `payroll`). Formerly the stand-alone "module-5-payroll" bundle; module numbers in older text were renamed to the canonical M0–M12 / HR-context names.
- D5-01 HR/Payroll = Payroll & Statutory Compliance (masterplan). Bundle moved from `module-9-payroll`; the Meetings scaffold that sat at 5 is now `module-x-meetings` (unnumbered, like Projects D4-01). Remaining graph numbers 6–12 still diverge from the masterplan and are fixed as each module starts.
- D5-02 Money as integer paise (`calc.Money`), NUMERIC(14,2); rounding half away from zero at paise; statutory amounts rounded to the rupee as the statutes do.
- D5-03 Loss of pay comes from HR/Leave approved unpaid leave via a read-only exported port `leave/services.UnpaidLeave` (no writes into HR/Leave). Attendance absences are not LOP in v1 (they may be unregularized).
- D5-04 Monthly CTC is treated as monthly gross; employer PF/ESI contributions are not modelled in v1.
- D5-05 Structure components are immutable once created; new pay designs = new structure. Keeps finalized payslips explainable.
- D5-06 Professional Tax: one default slab (≥ ₹25,000 → ₹200). State-wise slabs need a tenant statutory profile (v1.1).
- D5-07 TDS: new regime FY 2025-26, full-month projection × 12, standard deduction ₹75,000, 87A rebate up to ₹12,00,000 without marginal relief, 4% cess. Approximate by design; investment declarations/old regime out of scope.
- D5-08 Maker-checker on approve (approver ≠ last calculator); finalize also needs `payroll:approve`.
- D5-09 Payslips store an employee snapshot (code, name) so later HR/Employee edits never rewrite history.
- D5-10 Eligibility picks each employee's latest assignment overlapping the period (DISTINCT ON); a mid-month CTC change pays the newer CTC for the whole month in v1 (no split-period proration).
##### Changelog
2026-10-09 — P1 DONE: HR/Payroll = Payroll (D5-01); bundle moved from module-9-payroll; design frozen.
2026-10-09 — P2 DONE: money codec, 7 models, migrations 000038–000044; P3 calc engine goldens G2–G9 green.
2026-10-09 — P3 DONE: DTOs, validators.
2026-10-09 — P4 DONE: repositories.
2026-10-09 — P5 DONE: payroll services, events, relay; HR/Leave UnpaidLeave port; goldens green.
2026-10-09 — P6 DONE: HTTP edge, app wiring, swagger.
2026-10-09 — P7+P9 DONE (backend CLOSED): integration goldens, parity, DoD.

### m1-hr-management/contexts/recruitment  _(from specification.md)_

**Requirements and rules (ID-tagged table rows)**

| ID | Text | … |
|---|---|---|
| RC-001 | Candidates can be added only to `open` jobs (409 `JOB_NOT_OPEN`). |
| RC-002 | Manual stage moves: applied→screening\|rejected\|withdrawn; screening→interview\|rejected\|withdrawn; interview→rejected\|withdrawn; offer→rejected\|withdrawn. Anything else → 409 `INVALID_STAGE`. Terminal stages are final. |
| RC-003 | Every stage change writes a stage event in the same transaction. |
| RC-004 | Interviews only for candidates in `interview` (409 `INVALID_STAGE`); scheduled_at in the future (400). |
| RC-005 | Only the assigned interviewer submits feedback (others 404); only `scheduled` interviews (409 `INTERVIEW_STATE`). |
| RC-006 | One open (`offered`) offer per candidate (409 `OFFER_EXISTS`); withdrawing/declining frees it. |
| RC-007 | Hire requires stage `offer` and an `accepted` offer (409 `NOT_HIREABLE`). |
| RC-008 | Hire increments hired_count; at headcount the job becomes `filled`; hiring into a `filled`/`closed` job → 409 `JOB_NOT_OPEN`. |
| RC-009 | Job/candidate/interview/offer ids are tenant-scoped; foreign ids → 404. |
| RC-010 | Hire is idempotent: the invited user id is stored on the candidate before the employee is created; a retry reuses it and an existing profile for that user. |
| RC-011 | Candidate PII and feedback never appear in events or audit metadata. |
| R1 | POST `/recruitment/jobs` | `recruitment:manage` |
| R2 | GET `/recruitment/jobs?status&department_id&page&per_page` | `recruitment:read` |
| R3 | GET `/recruitment/jobs/{id}` | `recruitment:read` |
| R4 | PATCH `/recruitment/jobs/{id}` | `recruitment:manage` |
| R5 | POST `/recruitment/jobs/{id}/status` `{status}` | `recruitment:manage` |
| R6 | POST `/recruitment/candidates` | `recruitment:manage` |
| R7 | GET `/recruitment/candidates?job_id&stage&page&per_page` | `recruitment:read` |
| R8 | GET `/recruitment/candidates/{id}` (with stage history) | `recruitment:read` |
| R9 | PATCH `/recruitment/candidates/{id}` | `recruitment:manage` |
| R10 | POST `/recruitment/candidates/{id}/stage` `{stage, note?}` | `recruitment:manage` |
| R11 | POST `/recruitment/candidates/{id}/hire` `{employee_code, employment_type?}` | `recruitment:hire` |
| R12 | POST `/recruitment/interviews` | `recruitment:manage` |
| R13 | GET `/recruitment/interviews?candidate_id&page&per_page` | `recruitment:read` |
| R14 | GET `/recruitment/interviews/me?status&page&per_page` | self (interviewer) |
| R15 | POST `/recruitment/interviews/{id}/feedback` | self (assigned interviewer) |
| R16 | POST `/recruitment/interviews/{id}/cancel` | `recruitment:manage` |
| R17 | POST `/recruitment/offers` | `recruitment:manage` |
| R18 | GET `/recruitment/offers?candidate_id&page&per_page` | `recruitment:read` |
| R19 | POST `/recruitment/offers/{id}/decision` `{decision: accepted\|declined}` | `recruitment:manage` |
| R20 | POST `/recruitment/offers/{id}/withdraw` | `recruitment:manage` |

**Requirements and rules (ID-tagged bullets)**

- FR-JB001 Create {title, department_id?, designation_id?, headcount 1–1000, location?, employment_type `full_time|part_time|contract|intern`, min_experience_years 0–50, description} → status `draft`. Department/designation validated in HR/Organization.
- FR-JB002 List (filters status, department_id) / get / update (any field while not `closed`/`filled`).
- FR-JB003 Status transitions: draft→open; open↔on_hold; draft|open|on_hold→closed; `filled` only automatically when hired_count reaches headcount (RC-008).
- FR-CD001 Add a candidate to an **open** job {first_name, last_name, email, phone?, source `career_site|referral|agency|linkedin|other`, resume_url?, expected_ctc?, notice_period_days?}; email unique per job (case-insensitive) → stage `applied` + stage event.
- FR-CD002 List (job_id, stage filters) / get (with stage history) / update contact fields.
- FR-CD003 Move stage (manual): only along RC-002; `offer` is entered by creating an offer, `hired` only by hire.
- FR-IV001 Schedule {candidate_id, round_name, interviewer_employee_id, scheduled_at (future), duration_mins 15–480, meeting_link?} for a candidate in `interview`; interviewer must be a working employee (HR/Employee).
- FR-IV002 Interviewer self-service: list my interviews; submit feedback {rating 1–5, recommendation `strong_hire|hire|hold|reject`, notes} on my own scheduled interview → `completed`.
- FR-IV003 Cancel (manage); list by candidate (read).
- FR-OF001 Create {candidate_id, offered_ctc > 0, joining_date ≥ today, expires_on?} for a candidate in `interview` or `offer` with no open offer → status `offered`; candidate moves to `offer`.
- FR-OF002 Record decision `accepted|declined` (manage, on the candidate's behalf) or withdraw; list by candidate.
- FR-HR001 Hire {employee_code, employment_type?} for a candidate in `offer` with an `accepted` offer: (1) invite user (email, names) unless already invited for this candidate; (2) create employee profile with joining_date = offer joining date; (3) candidate → `hired`, job hired_count + 1, job `filled` when full. Idempotent on retry (RC-010).
- FR-EV001 Exchange `jaas.recruitment.events`: `recruitment.job_published`, `recruitment.candidate_applied`, `recruitment.interview_scheduled`, `recruitment.candidate_hired`; outbox in-tx; relay.
- FR-EV002 Payloads carry ids, stages and job/period data only — never candidate name/email/phone, CTC or feedback.

**Errors**

400 VALIDATION_ERROR · 403 FORBIDDEN · 404 NOT_FOUND, EMPLOYEE_NOT_FOUND · 409 CONFLICT (duplicate email per job, employee code taken), JOB_NOT_OPEN, JOB_STATE, INVALID_STAGE, INTERVIEW_STATE, OFFER_EXISTS, OFFER_STATE, NOT_HIREABLE · 500 opaque.

**Decision log (decisions.md, verbatim)**

> Part of **M1 HR Management** (bounded context `recruitment`). Formerly the stand-alone "module-6-recruitment" bundle; module numbers in older text were renamed to the canonical M0–M12 / HR-context names.
- D6-01 HR/Recruitment = Recruitment & ATS (masterplan). The Approvals scaffold that sat at 6 is now `module-x-approvals` (unnumbered). Earlier references to "M7 Approvals" in HR/Attendance, HR/Leave decisions (approval chains) now mean that unnumbered Approvals module.
- D6-02 Hire uses M0 Identity `InviteUser` (status invited, no password handled by recruitment); the new user gets no roles (RS-T5).
- D6-03 HR/Organization gains read-only accessors `DepartmentService()` and `DesignationService()` on its Module so job openings can validate references (recorded in module-1 decisions).
- D6-04 Hire is a three-step idempotent saga (architecture § Hire flow) because HR/Employee `CreateEmployee` runs its own transaction and must see the committed user row.
- D6-05 Offer decisions are recorded by recruiters on the candidate's behalf; a candidate portal is out of scope.
- D6-06 No automatic payroll assignment on hire: CTC is assigned in HR/Payroll by HR (keeps salary decisions in payroll).
- D6-07 (P5) Moving a candidate out of `offer` (rejected/withdrawn) withdraws its open offer in the same transaction, so RC-006 never leaves an orphan open offer.
- D6-08 (P5) A new offer is refused while one is open **or accepted** (OFFER_EXISTS); hire uses the accepted one. Accepting an offer past expires_on → OFFER_STATE.
- D6-09 (P5) Hire is allowed for jobs in open or on_hold; closed/filled → JOB_NOT_OPEN. If the candidate's email already belongs to a tenant user, M0 Identity InviteUser refuses (409) and hire stops before any employee is created.
##### Changelog
2026-10-09 — P1 DONE: HR/Recruitment = Recruitment (D6-01); design frozen.
2026-10-09 — P2 DONE: models, pipeline graphs, migrations 000045–000050.
2026-10-09 — P3 DONE: DTOs, validators.
2026-10-09 — P4 DONE: repositories.
2026-10-09 — P5 DONE: services, hire saga, events, relay; D6-07..D6-09.
2026-10-09 — P6 DONE: HTTP, wiring, swagger.
2026-10-09 — P7 + P9 DONE: integration 63/63; backend CLOSED.

### m1-hr-management/contexts/performance  _(from specification.md)_

**Requirements and rules (ID-tagged table rows)**

| ID | Text | … |
|---|---|---|
| PM-001 | Criteria: 1–10 names, trimmed, 1–50 chars, unique ignoring case; the order is kept. Cycle edits only in `draft` (409 `CYCLE_STATE`). |
| PM-002 | Cycle transitions are only `draft→active→review→completed` (409 `CYCLE_STATE`). |
| PM-003 | Goals are created/edited/status-changed only while the cycle is `draft` or `active` (409 `CYCLE_STATE`). The owner must be a working employee (`active|probation|notice`; else 409 `EMPLOYEE_STATE`). |
| PM-004 | Weight is 0.01–100.00. The sum of an owner's non-cancelled goal weights in a cycle is ≤ 100.00 (409 `WEIGHT_EXCEEDED`), checked under a per-(cycle, owner) lock. |
| PM-005 | The parent goal must be in the same tenant and cycle, not cancelled, and the chain depth ≤ 3 (goal → parent → grandparent; 409 `GOAL_DEPTH`). The parent is fixed at creation. |
| PM-006 | KR progress = clamp((current − start) / (target − start) × 100, 0, 100), rounded half away from zero to 0.01. A goal with ≥ 1 KR has progress = mean of its KRs' progress (same rounding). Direct progress on such a goal → 409 `HAS_KEY_RESULTS`; a KR check-in on a goal without KRs is impossible (KR must belong to the goal, else 404). |
| PM-007 | Every progress change appends a check-in (from → to, author, note) in the same transaction. Check-ins are allowed only on `active` goals while the cycle is `active` or `review` (409 `GOAL_STATE` / `CYCLE_STATE`). KRs can be added only while the cycle is `draft` or `active`. |
| PM-008 | Access to a goal (read/write): owner, owner's manager, or `performance:manage` (write) / `performance:read` (read). Anyone else → 404. |
| PM-009 | Goal score = Σ(weight × progress) / Σ weight over non-cancelled goals (0 when none), rounded to 0.01. |
| PM-010 | Launch reviews: for each employee owning ≥ 1 non-cancelled goal in the cycle, create a `self` review (reviewer = employee) and, when a manager with an employee profile exists, a `manager` review. Idempotent: unique (cycle, employee, reviewer, type). The response reports counts, including employees without a manager. |
| PM-011 | Assign reviewer: cycle in `review`; reviewer ≠ subject; reviewer is a working employee; duplicates → 409 `CONFLICT`. |
| PM-012 | Submit: only the assigned reviewer (others 404); review `pending` (409 `REVIEW_STATE`); cycle `review` (409 `CYCLE_STATE`). Ratings must name exactly the cycle criteria, once each, with integer scores 1–5 (400). `potential` is required for `manager` reviews and forbidden otherwise (400). overall_score = mean of scores, rounded half away from zero to 0.01. Submitted reviews are immutable. |
| PM-013 | Completing a cycle marks its pending reviews `expired`. |
| PM-014 | A subject sees their own **submitted** reviews only after the cycle is `completed`. `reviewer_employee_id` is omitted for peer reviews (anonymity). |
| PM-015 | 9-box: per employee, performance band from the latest submitted manager review's overall_score (< 2.50 `low`, < 3.50 `medium`, else `high`) × that review's potential. Employees without a submitted manager review are listed as `unrated`. Each entry also carries the goal score (PM-009). |
| PM-016 | All ids are tenant-scoped; foreign ids → 404. |
| PM-017 | Ratings, scores, potential and review text never appear in events or audit metadata. |
| C1 | POST `/cycles` | manage |
| C2 | GET `/cycles?status&page&per_page` | authenticated |
| C3 | GET `/cycles/{id}` | authenticated |
| C4 | PATCH `/cycles/{id}` | manage |
| C5 | POST `/cycles/{id}/activate` | manage |
| C6 | POST `/cycles/{id}/launch-reviews` | manage |
| C7 | POST `/cycles/{id}/complete` | manage |
| C8 | GET `/cycles/{id}/nine-box` | read |
| G1 | POST `/goals` | self / manager / manage (PM-008) |
| G2 | GET `/goals?cycle_id&employee_id&status&page&per_page` | read: any filter. Without read: `employee_id` defaults to self and must be self or a direct report (else 404, D7-05) |
| G3 | GET `/goals/me?cycle_id&page&per_page` | self |
| G4 | GET `/goals/{id}` | PM-008 read |
| G5 | PATCH `/goals/{id}` | PM-008 write |
| G6 | POST `/goals/{id}/status` `{status: completed|cancelled|active}` | PM-008 write |
| G7 | POST `/goals/{id}/key-results` | PM-008 write |
| G8 | POST `/goals/{id}/check-ins` | PM-008 write |
| V1 | POST `/reviews` | manage |
| V2 | GET `/reviews?cycle_id&employee_id&status&page&per_page` | read |
| V3 | GET `/reviews/assigned?status&page&per_page` | self (reviewer) |
| V4 | GET `/reviews/me?cycle_id` | self (subject, PM-014) |
| V5 | GET `/reviews/{id}` | reviewer or read |
| V6 | POST `/reviews/{id}/submit` | reviewer |
| PM-018 | Calibration only while the cycle is `review` (409 `CYCLE_STATE`). The employee must have a submitted manager review in the cycle (409 `NOT_RATED`). |
| PM-019 | Each calibration is an append-only row with the before values (the current final score and potential), the after values, the reason, and who made it and when. Omitted fields keep their current final value. A calibration that changes nothing is 400. |
| PM-020 | The **final rating** of an employee in a cycle is the latest calibration if one exists, otherwise the latest submitted manager review. The 9-box (PM-015) uses the final rating and marks `calibrated: true`. |
| PM-021 | After completion, the subject's `GET /reviews/me` response carries `final` (score, potential, calibrated) for each completed cycle. Calibration reasons and the calibrator are never shown to the subject. |
| PM-022 | Audit and events (`performance.rating_calibrated`) carry ids only: never scores, potential or reason (PM-017). |
| F1 | POST `/feedback` `{to_employee_id, kind: praise\|constructive, visibility: private\|public, message, request_id?}` | self |
| F2 | GET `/feedback/received?page&per_page` | self |
| F3 | GET `/feedback/given?page&per_page` | self |
| F4 | GET `/feedback?employee_id&page&per_page` | the employee's manager, or performance:read |
| F5 | GET `/feedback/public?page&per_page` | authenticated (praise wall) |
| F6 | POST `/feedback/requests` `{responder_employee_ids[1..10], about_employee_id?, question}` | self; `about` = self or a direct report |
| F7 | GET `/feedback/requests/assigned?status&page&per_page` | self (responder) |
| F8 | GET `/feedback/requests/mine?status&page&per_page` | self (requester) |
| F9 | POST `/feedback/requests/{id}/decline` | the responder |
| FB-001 | Giver ≠ recipient; both are working employees (409 `EMPLOYEE_STATE`); message 1–2000 characters; `public` only with `praise` (400). |
| FB-002 | Visibility: `private` feedback is visible to the giver, the recipient, the recipient's direct manager and performance:read. `public` feedback is also visible to every tenant user on F5. Anyone else → 404. |
| FB-003 | Feedback is immutable once given. |
| FB-004 | Requests: the requester asks 1–10 distinct working colleagues (never the subject or the requester); question 1–500 characters. `about_employee_id` defaults to the requester and may be a direct report (else 404). One `pending` request per (requester, responder, about); a repeat → 409 `CONFLICT`. |
| FB-005 | Answering: F1 with `request_id`. The caller must be the request's responder, the request must be `pending` (409 `REQUEST_STATE`), and `to_employee_id` must be the request's subject (400). The request becomes `answered` and links the feedback. Declining sets `declined`. |
| FB-006 | Events `performance.feedback_given` and `performance.feedback_requested` and the audit rows carry ids, kind and visibility only, never the message or the question (PM-017). |

**Requirements and rules (ID-tagged bullets)**

- FR-CY001 Create `{title 1–150, start_date, end_date ≥ start, criteria? (1–10 distinct names 1–50 chars; default Quality, Delivery, Collaboration, Ownership)}` → `draft`.
- FR-CY002 List (status filter) / get; update title, dates and criteria only in `draft`.
- FR-CY003 Activate `draft → active` (event `performance.cycle_started`).
- FR-CY004 Launch reviews `active → review`: creates self + manager reviews (PM-010). On a cycle already in `review` it only creates missing reviews (EC-06, D7-07).
- FR-CY005 Complete `review → completed`: pending reviews become `expired` (event `performance.cycle_completed`).
- FR-CY006 9-box for a cycle in `review` or `completed` (PM-015).
- FR-GL001 Create `{cycle_id, employee_id?, title 1–200, description?, weight 0.01–100, parent_goal_id?}`. `employee_id` defaults to the caller's own profile.
- FR-GL002 List by cycle/employee; my goals; get with key results and check-ins (newest 50).
- FR-GL003 Update title, description and weight (PM-003/PM-004).
- FR-GL004 Status: `active → completed | cancelled`, `completed → active`. Cancelled is final.
- FR-GL005 Add key result `{title 1–200, unit? ≤ 20, start_value, target_value ≠ start}`. Current value starts at `start_value`.
- FR-GL006 Check-in `{progress}` (goal without KRs) or `{key_result_id, value}` (goal with KRs), plus `note? ≤ 1000` → updates progress and appends a check-in (event `performance.goal_updated`).
- FR-RV001 Assign a reviewer `{cycle_id, employee_id, reviewer_employee_id, type: peer|manager}` (manage, cycle in `review`).
- FR-RV002 List all (read), assigned to me, mine (subject, completed cycles only), get one.
- FR-RV003 Submit `{ratings: [{criterion, score 1–5}], strengths?, improvements?, potential? (low|medium|high)}` → `submitted` (event `performance.review_submitted`).
- FR-EV001 Exchange `jaas.performance.events` (outbox in the same transaction, relay):
- FR-EV002 Payloads carry ids, statuses and progress only — never ratings, scores, potential or review text.
- NFR-SEC001: tenant scoping on every query.
- NFR-SEC002: self endpoints resolve the employee from the JWT, never from input.
- NFR-SEC003: PM-017 privacy.
- NFR-D001: progress + check-in + outbox in one transaction.
- NFR-D002: exact decimal arithmetic (hundredths integers, no floats).
- NFR-P001: list endpoints p95 < 300 ms at 1k goals per tenant (indexed by cycle + employee).
- EC-01: Goal weights 60 + 40 OK; a third goal of 0.01 → 409. After cancelling one, it fits again.
- EC-02: KR with a decreasing target (start 50 → target 10, current 30) → 50.00%. A value beyond the target clamps to 100; below the start clamps to 0.
- EC-03: Direct progress on a goal with KRs → 409.
- EC-04: A colleague (not manager, no permission) reads a goal → 404.
- EC-05: Create a goal in a `review` cycle → 409 `CYCLE_STATE`.
- EC-06: Launch reviews twice → second call creates 0.
- EC-07: Submit with a missing or extra criterion → 400. A manager review without potential → 400.
- EC-08: Submit after the cycle completed → 409 (review expired).
- EC-09: Subject lists reviews before completion → empty. After completion, peer reviews have no reviewer id.
- EC-10: A cascading chain of depth 4 → 409 `GOAL_DEPTH`.
- EC-11: Cross-tenant ids → 404.
- **FR-CA001** Calibrate `POST /cycles/{id}/calibrations {employee_id, overall_score?, potential?, reason}`. At least one
- **FR-CA002** History `GET /cycles/{id}/calibrations?employee_id&page&per_page` (performance:read), newest first.

**Errors**

- 400: `VALIDATION_ERROR`.
- 401: unauthenticated.
- 403: `FORBIDDEN` (RBAC).
- 404: `NOT_FOUND`, `EMPLOYEE_NOT_FOUND`.
- 409: `CYCLE_STATE`, `GOAL_STATE`, `REVIEW_STATE`, `WEIGHT_EXCEEDED`, `GOAL_DEPTH`, `HAS_KEY_RESULTS`, `EMPLOYEE_STATE`, `CONFLICT`.
- 500: opaque.

**Decision log (decisions.md, verbatim)**

> Part of **M1 HR Management** (bounded context `performance`). Formerly the stand-alone "module-7-performance" bundle; module numbers in older text were renamed to the canonical M0–M12 / HR-context names.
- D7-01 (P1) HR/Performance = Performance Management & OKRs per the masterplan; the repo's Notifications scaffold becomes unnumbered `module-x-notifications` (same pattern as D5-01, D6-01).
- D7-02 (P1) Decimals (weight, progress, KR values, scores) use a hundredths fixed-point type with half-away-from-zero rounding; no floats (NFR-D002).
- D7-03 (P1) The direct manager is read from HR/Organization: the user's primary active mapping's manager_user_id, via the existing `organization.Module.MappingService()` accessor. No HR/Organization change is needed.
- D7-04 (P1) The 9-box performance axis is the manager review's overall score; the goal score is reported alongside, not blended (blending formulas are a calibration feature, out of scope).
- D7-05 (P1) Access denials on goals/reviews are 404, not 403, so existence is not disclosed (same as HR/Attendance, HR/Leave, HR/Payroll, HR/Recruitment self endpoints).
- D7-06 (P1) Cycle list/get are open to any authenticated tenant user (needed to pick a cycle when setting goals); they contain no personal data.
- D7-07 (P5) Launch reviews is idempotent on a cycle already in `review`: it creates only missing self/manager reviews
  and does not change the status (EC-06 "a second launch creates 0"; PM-002 forbids every other repeat transition).
- D7-08 (P5) HR/Performance reads the direct manager through its own `ManagerDirectory` port; the HR root passes
  `organization.Module.DirectManager` (primary active mapping), so performance never imports HR/Organization (import-boundary test).
- D7-09 (P6) A parent goal from another tenant is 404 (PM-016); a parent in another cycle or cancelled is 400.
- D7-10 (parity) Calibration records an override next to the manager review instead of editing it, so the submitted
  review stays immutable (PM-012) and every change is auditable with a reason; the "final rating" is derived (PM-020).
- D7-11 (parity) A separate `performance:calibrate` permission: calibration changes outcomes for people outside one's
  team, so it is narrower than `performance:manage` (which runs cycles).
- D7-12 (parity) Continuous feedback is cycle-independent and visible to the recipient's direct manager; public
  visibility is limited to praise so the wall cannot carry criticism.
##### Changelog
2026-10-09 — P1 DONE: HR/Performance = Performance & OKRs (D7-01); design frozen; Notifications scaffold unnumbered.
2026-10-09 — P2 DONE: models, migrations, score + lifecycle goldens.
2026-10-20 — P3–P9 DONE: backend, unit + integration goldens, swagger, console page; context complete for its v1 scope.
2026-10-20 — Parity design: calibration (PM-018..PM-022) and continuous feedback (FB-001..FB-006), goldens G16–G20.
2026-10-20 — Parity DONE: calibration + continuous feedback built and verified (33 ops in total).
