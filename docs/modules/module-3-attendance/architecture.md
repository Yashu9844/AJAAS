# Module 3 — Architecture (FROZEN at P1 2026-10-08)

## 1. Layers (Clean + DDD, mirrors Modules 0–2)

```text
Gin → shared RequestLogger/CORS → Module 0 TenantResolver → Module 0 Authenticate
→ Module 0 RequirePermission(attendance:read|manage|approve)   (self endpoints: auth only)
→ Controllers (punch, attendance query, regularization, shift)   thin: parse → validate → service → map
→ Services (ShiftService, AssignmentService, PunchService, AttendanceQueryService, RegularizationService, OutboxRelay)
→ calc (pure functions: date attribution, totals, status)   ← no I/O, golden-tested
→ Repositories (shift, assignment, record, punch, regularization, outbox) — tenant_id-scoped, *gorm.DB tx param
→ PostgreSQL (truth) · RabbitMQ (jaas.attendance.events via outbox) · Module 0 AuditService · Module 2 EmployeeService
```

Package: `backend/internal/attendance/{models,dto,validators,calc,repositories,services,controllers,routes,events}` + `module.go`.

## 2. Components

| Component | Responsibility |
|---|---|
| `calc` | Pure time math: `AttendanceDate(now, shift)`, `ComputeTotals(punches, shift, date)`, `Status(totals, open, shift)`, `ExpectedMinutes`, `ParseHHMM`. Zero dependencies → exhaustive table tests (goldens G4, G5). |
| `ShiftService` | CRUD + deactivate guard (AT-012, AT-013). |
| `AssignmentService` | Create with overlap guard + auto-close (AT-011); list; `ActiveFor(employee, date)` used by punch/regularization. |
| `PunchService` | Resolve caller employee (Module 2) → tx { advisory lock → load last punch / open session → rules AT-003..AT-006 → upsert record → insert punch → recompute → outbox } → after commit: audit + publish. |
| `AttendanceQueryService` | Today view, self range, admin list/detail, summary (aggregate SQL). |
| `RegularizationService` | Create / list / cancel / approve / reject (AT-014..AT-018); approve rewrites record inside one tx. |
| `OutboxRelay` | Background ticker (30 s): publish unpublished rows (batch 100, attempts < 10), mark published. Started by `cmd/main.go`, stopped on shutdown context. |
| `employeeDirectory` (port) | Local interface over Module 2 `EmployeeService` (GetByUserID, GetByID, List). Adapter wired in `module.go`. |
| `auditLogger` (port) | Local interface over Module 0 `AuditService.Log`. |
| `txRunner` (port) | `Transaction(ctx, fn)` over `*gorm.DB`; unit tests pass a fake that calls `fn(nil)`. |

Controllers never hold `*gorm.DB` (fixes the Module 0–2 pattern locally; D3-05). Services own the DB handle via `txRunner` and pass `tx` to repositories.

## 3. Data flow — punch (A1)

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

## 4. Control flow — regularization approve (A11)

```text
load request (tenant) → 404 | status != pending → 409 | reviewer's employee == requester → 403
tx: lock employee → record FindOrCreate(date) → punches.SupersedeForRecord → insert in/out (source regularization)
    → recompute totals → record.is_regularized = true → request approved → outbox
after commit: audit + publish
```

## 5. Date & time model (WHY)

- All instants stored UTC (`timestamptz`); dates stored as `date`.
- Shift times stored as minutes-from-midnight ints (`start_minute`, `end_minute`) in the shift's IANA `timezone`. Avoids Postgres `TIME` ↔ Go driver ambiguity and makes night-shift math integer arithmetic. API speaks `HH:MM`.
- Attendance date and lateness are evaluated in the shift timezone, so a 09:00 IST shift is late at 09:16 IST regardless of server zone.
- Night shift attribution window = `end + 4h` (AT-005) covers late check-ins without a separate roster engine.

## 6. Failure paths

DB down → 500 envelope, tx rolled back, nothing published. Lock wait bounded by request context timeout. Broker down → row stays in outbox, relay retries (never blocks API). Audit failure → logged, never fails the request (runs after commit so it cannot abort the tx). Module 2 lookup failure → error propagates (fail closed: no punch without a verified employee).

## 7. Concurrency

Advisory xact lock per employee serializes punch and approval for that employee (EC-15). Unique (tenant, employee, date) on records backs FindOrCreate (insert … on conflict do nothing, then select). Unique partial index on pending regularizations backs AT-015 against races. Shift name/code races → unique index → 409.

## 8. Persistence & indexes

See specification.md §8. Query paths: open session = latest non-superseded `in` without later `out` for employee (index employee+punch_time DESC); summary = one `GROUP BY status` over (tenant, date) + Module 2 totals.

## 9. Security

Self endpoints take no employee/user id input (NFR-SEC002). Coordinates/device/IP stored for audit but IP never returned and never put in events. Every repo method takes `tenantID`. See security.md.

## 10. WHY (key decisions — details in decisions.md)

- Separate attendance tables keyed by employee profile (not user): attendance is employment data; deactivated users keep history.
- Outbox written in-tx (vs Module 1/2 publish-then-fallback): guarantees no ghost or lost events; relay closes the loop.
- Pure `calc` package: the riskiest logic (time zones, night shifts, thresholds) is testable without DB and frozen as goldens.
- Supersede instead of delete on regularization: audit trail of what was punched vs what was approved.
