# HR / Attendance & Time — Design

> Part of **M1 HR Management** (bounded context `attendance`). Formerly the stand-alone "module-3-attendance" bundle; module numbers in older text were renamed to the canonical M0–M12 / HR-context names.

## Architecture

### 1. Layers (Clean + DDD, mirrors M0 Identity, HR/Organization, HR/Employee)

```text
Gin → shared RequestLogger/CORS → M0 Identity TenantResolver → M0 Identity Authenticate
→ M0 Identity RequirePermission(attendance:read|manage|approve)   (self endpoints: auth only)
→ Controllers (punch, attendance query, regularization, shift)   thin: parse → validate → service → map
→ Services (ShiftService, AssignmentService, PunchService, AttendanceQueryService, RegularizationService, OutboxRelay)
→ calc (pure functions: date attribution, totals, status)   ← no I/O, golden-tested
→ Repositories (shift, assignment, record, punch, regularization, outbox) — tenant_id-scoped, *gorm.DB tx param
→ PostgreSQL (truth) · RabbitMQ (jaas.attendance.events via outbox) · M0 Identity AuditService · HR/Employee EmployeeService
```

Package: `backend/internal/attendance/{models,dto,validators,calc,repositories,services,controllers,routes,events}` + `module.go`.

### 2. Components

| Component | Responsibility |
|---|---|
| `calc` | Pure time math: `AttendanceDate(now, shift)`, `ComputeTotals(punches, shift, date)`, `Status(totals, open, shift)`, `ExpectedMinutes`, `ParseHHMM`. Zero dependencies → exhaustive table tests (goldens G4, G5). |
| `ShiftService` | CRUD + deactivate guard (AT-012, AT-013). |
| `AssignmentService` | Create with overlap guard + auto-close (AT-011); list; `ActiveFor(employee, date)` used by punch/regularization. |
| `PunchService` | Resolve caller employee (HR/Employee) → tx { advisory lock → load last punch / open session → rules AT-003..AT-006 → upsert record → insert punch → recompute → outbox } → after commit: audit + publish. |
| `AttendanceQueryService` | Today view, self range, admin list/detail, summary (aggregate SQL). |
| `RegularizationService` | Create / list / cancel / approve / reject (AT-014..AT-018); approve rewrites record inside one tx. |
| `OutboxRelay` | Background ticker (30 s): publish unpublished rows (batch 100, attempts < 10), mark published. Started by `cmd/main.go`, stopped on shutdown context. |
| `employeeDirectory` (port) | Local interface over HR/Employee `EmployeeService` (GetByUserID, GetByID, List). Adapter wired in `module.go`. |
| `auditLogger` (port) | Local interface over M0 Identity `AuditService.Log`. |
| `txRunner` (port) | `Transaction(ctx, fn)` over `*gorm.DB`; unit tests pass a fake that calls `fn(nil)`. |

Controllers never hold `*gorm.DB` (fixes the M0 Identity, HR/Organization, HR/Employee pattern locally; D3-05). Services own the DB handle via `txRunner` and pass `tx` to repositories.

### 3. Data flow — punch (A1)

```text
POST /attendance/punch {type:"in"}
→ ctrl: bind + validate (type oneof, lat/long ranges) → svc.Punch(ctx, tenantID, userID, ip, req)
→ employees.GetByUserID(tenant,user)                       404 EMPLOYEE_NOT_FOUND / 403 EMPLOYEE_NOT_ACTIVE
→ tx BEGIN
   repo.LockEmployee(tx, tenant, emp)                      pg_advisory_xact_lock(hashtext(tenant||emp))  AT-022
   last := punches.LastForEmployee(tx)                      AT-004 debounce (60 s)
   open := punches.OpenSession(tx)                          AT-003 / AT-006 (20 h abandon window)
   shift := assignments.ActiveFor(tx, emp, date)            date via calc.AttendanceDate (AT-005)
   record := records.FindOrCreate(tx, emp, date)
   punches.Create(tx, punch{server now})                    AT-001
   ps := punches.ForRecord(tx, record) → calc.ComputeTotals → records.Update(tx)
   outbox.Create(tx, event attendance.punch.in)             FR-EV002
 COMMIT
→ audit.Log(db, "attendance.punch_in")   best-effort after commit
→ publisher.Publish → outbox.MarkPublished               failure leaves row for relay
→ 201 {punch, record}
```

### 4. Control flow — regularization approve (A11)

```text
load request (tenant) → 404 | status != pending → 409 | reviewer's employee == requester → 403
tx: lock employee → record FindOrCreate(date) → punches.SupersedeForRecord → insert in/out (source regularization)
    → recompute totals → record.is_regularized = true → request approved → outbox
after commit: audit + publish
```

### 5. Date & time model (WHY)

- All instants stored UTC (`timestamptz`); dates stored as `date`.
- Shift times stored as minutes-from-midnight ints (`start_minute`, `end_minute`) in the shift's IANA `timezone`. Avoids Postgres `TIME` ↔ Go driver ambiguity and makes night-shift math integer arithmetic. API speaks `HH:MM`.
- Attendance date and lateness are evaluated in the shift timezone, so a 09:00 IST shift is late at 09:16 IST regardless of server zone.
- Night shift attribution window = `end + 4h` (AT-005) covers late check-ins without a separate roster engine.

### 6. Failure paths

DB down → 500 envelope, tx rolled back, nothing published. Lock wait bounded by request context timeout. Broker down → row stays in outbox, relay retries (never blocks API). Audit failure → logged, never fails the request (runs after commit so it cannot abort the tx). HR/Employee lookup failure → error propagates (fail closed: no punch without a verified employee).

### 7. Concurrency

Advisory xact lock per employee serializes punch and approval for that employee (EC-15). Unique (tenant, employee, date) on records backs FindOrCreate (insert … on conflict do nothing, then select). Unique partial index on pending regularizations backs AT-015 against races. Shift name/code races → unique index → 409.

### 8. Persistence & indexes

See specification.md §8. Query paths: open session = latest non-superseded `in` without later `out` for employee (index employee+punch_time DESC); summary = one `GROUP BY status` over (tenant, date) + HR/Employee totals.

### 9. Security

Self endpoints take no employee/user id input (NFR-SEC002). Coordinates/device/IP stored for audit but IP never returned and never put in events. Every repo method takes `tenantID`. See security.md.

### 10. WHY (key decisions — details in decisions.md)

- Separate attendance tables keyed by employee profile (not user): attendance is employment data; deactivated users keep history.
- Outbox written in-tx (vs HR/Organization, HR/Employee publish-then-fallback): guarantees no ghost or lost events; relay closes the loop.
- Pure `calc` package: the riskiest logic (time zones, night shifts, thresholds) is testable without DB and frozen as goldens.
- Supersede instead of delete on regularization: audit trail of what was punched vs what was approved.

## Connections

Depends On: M0 Identity (Tenant, Auth, RBAC, Audit), HR/Employee (Employee profiles). HR/Organization not required in v1 (manager-scoped approvals are v1.1).
Consumed By: HR/Leave (writes `on_leave` via contract C10), HR/Payroll (reads records), M6 Reports & Analytics, M9 AI Assistant, IoT (dropped) (future punch source).

| # | Source → Destination | Protocol | Contract | Auth | Format | Failure | Owner | Tests |
|---|---|---|---|---|---|---|---|---|
| C1 | Client → HR/Attendance API (19 endpoints) | HTTPS (Kong) → HTTP | specification.md §5 + swagger | JWT (M0 Identity Authenticate) + tenant subdomain; `attendance:read/manage/approve` via M0 Identity RequirePermission | JSON envelope | 400/401/403/404/409 per spec §6 | HR/Attendance | controller units + tests/api/attendance_*_test.go |
| C2 | HR/Attendance → HR/Employee EmployeeService | In-process Go | `GetByUserID`, `GetByID`, `List(filter{Status})` | service-level, tenant passed explicitly | Go DTOs | not found → 404; error → fail closed | HR/Employee provider | service units (fake directory) + live goldens |
| C3 | HR/Attendance → M0 Identity AuditService | In-process Go | `Log(ctx, db, tenant, user, action, resource, id, meta, ip, ua)` | service-level | Go | best-effort after commit | M0 Identity | service units (spy) + G11 live |
| C4 | HR/Attendance → M0 Identity middleware | In-process Gin | `TenantResolver`, `Authenticate`, `RequirePermission(db,"attendance",action,…)` | JWT | Gin context `tenant_id`, `user_id` | 401/403 | M0 Identity | G9 live |
| C5 | HR/Attendance → PostgreSQL | TCP/5432 (shared pool) | migrations 000026–000031 | env creds | SQL | rollback, 500 | HR/Attendance | repo via live goldens; migration up/down/up |
| C6 | HR/Attendance → RabbitMQ `jaas.attendance.events` | AMQP topic | spec FR-EV001 envelope v1.0 | env creds | JSON | outbox + relay; never blocks API | HR/Attendance | outbox unit + G12 live |
| C7 | HR/Attendance boot → M0 Identity `permissions` table | SQL (idempotent insert) | rows `attendance:read`, `attendance:manage`, `attendance:approve` | DB | — | boot fails loudly | HR/Attendance (seed) / M0 Identity (table) | boot + G9 |
| C8 | HR/Attendance → shared queue/logger/errors | In-process | `queue.EventPublisher`, `errors.AppError` | — | — | — | shared | — |
| C9 | HR/Payroll, M6 Reports & Analytics, M9 AI Assistant → HR/Attendance reads | In-process Go (v1) | `AttendanceQueryService` list methods + records table schema (read-only) | service-level | DTOs | — | HR/Attendance provider | consumer contract tests (their P1) |
| C10 | HR/Leave → HR/Attendance | In-process Go (v1, HR/Leave P2) | `MarkLeave(ctx, tenant, employee, date, leaveRequestID)` → sets status `on_leave` (reserved, not built until HR/Leave) | service-level | — | — | HR/Attendance provider | HR/Leave P2 |

Trust boundary: everything outside the handler is untrusted. Employee for self endpoints is derived from JWT `user_id` → HR/Employee profile; never from request input. Every id parameter is resolved inside the caller's tenant; foreign ids return 404.

## Security

### Data classification
- Attendance times, coordinates, device ids: personal data (Medium). IP address: Medium, stored for audit only, never returned or published.
- No credentials, tokens or statutory data in HR/Attendance tables.

### Permissions (seeded at boot, idempotent — connections C7)
| Permission | Grants |
|---|---|
| `attendance:read` | admin record list/detail, summary, all regularizations list, shifts list/get, assignment list |
| `attendance:manage` | shift create/update/deactivate, shift assignments |
| `attendance:approve` | approve / reject regularizations |
| (none — authenticated employee) | punch, own today/records, own regularization create/list/cancel |

`tenant_admin` bypass from M0 Identity RequirePermission applies unchanged.

### Threats & mitigations
| ID | Threat | Mitigation |
|---|---|---|
| AS-T1 | Buddy punching / backdating via client clock | AT-001 server clock only; no time field in PunchRequest |
| AS-T2 | Punching for another employee | NFR-SEC002: employee resolved from JWT user only |
| AS-T3 | Cross-tenant reads (IDOR) | every repo call tenant-scoped; foreign id → 404 (G1) |
| AS-T4 | Self-approval of corrections | AT-016 → 403 SELF_APPROVAL_FORBIDDEN (G7) |
| AS-T5 | Punch flooding | AT-004 60 s debounce + Redis IP rate limit 30/min on A1 |
| AS-T6 | Race producing duplicate sessions | AT-022 advisory lock (EC-15) |
| AS-T7 | Location/IP leakage via events | FR-EV003 payload whitelist |
| AS-T8 | Repudiation of corrections | supersede not delete (AT-019) + audit rows for every state change |
| AS-T9 | Mass assignment | DTOs whitelist fields; `is_night_shift`, `status`, totals never client-set |

### Audit actions (M0 Identity AuditService, after commit)
`attendance.punch_in`, `attendance.punch_out`, `attendance.regularization_requested`, `attendance.regularization_cancelled`, `attendance.regularization_approved`, `attendance.regularization_rejected`, `shift.created`, `shift.updated`, `shift.deactivated`, `shift.assigned`. Metadata: ids, dates, statuses — no coordinates, no IP in metadata (IP/UA go in the audit columns).

### Error hygiene
Bind/validation errors return `VALIDATION_ERROR` with field details (no raw Go error text). Unexpected errors → `INTERNAL_ERROR` only.

### Threat → evidence (verified 2026-10-09, P9)
| Threat | Evidence |
|---|---|
| AS-T1 | unit G3 (services/punch_golden_test.go); live attendance_race_test.go (client punch_time ignored); controllers SelfEndpointsIgnoreClientIDs |
| AS-T2 | controllers SelfEndpointsIgnoreClientIDs; routes self endpoints carry no id params; live G9 member /me sees only own rows |
| AS-T3 | live G1 (attendance_rules_test.go): 5 foreign ids → 404, foreign JWT → 403, list → 0 rows |
| AS-T4 | unit + live G7 |
| AS-T5 | live AT-004 debounce (attendance_flow_test.go); IP limiter wired in module.go (30/min, shared RateLimiter) |
| AS-T6 | live attendance_race_test.go: 10 concurrent INs → 1×201, 9×409, 1 punch row, 1 record |
| AS-T7 | unit G12 + live (outbox payload has no latitude/device/ip) |
| AS-T8 | unit G6 (supersede) + live G6 (regularization punches, superseded kept) + G11 audit rows |
| AS-T9 | DTO whitelist (dto.go); controllers ignore unknown fields; status/totals/is_night_shift server-computed |

Pre-production checklist: docs/SECURITY.md §12 — HR/Attendance items (tenant-scoped queries, DTO validation, parameterized queries, no panic, structured logs) verified at P9; JWT/bcrypt/MFA/CORS/TLS items are M0 Identity / deploy scope.
