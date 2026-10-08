# Module 4 — Architecture (FROZEN at P1 2026-10-09)

## 1. Layers (mirrors Module 3)

```text
Gin → Module 0 TenantResolver → Authenticate → RequirePermission(leave:read|manage|approve)   (self + type/holiday reads: auth only)
→ Controllers (type, holiday, balance, request)            thin: parse → validate → service → envelope
→ Services (TypeService, HolidayService, BalanceService, RequestService, OutboxRelay)
→ calc (pure: Days hundredths, CountDays w/ holidays + sandwich, Accrued(annual|monthly, joining, asOf), CarryForward)
→ Repositories (type, holiday, balance, request, ledger, outbox) — tenant-scoped, tx param, advisory lock
→ PostgreSQL · RabbitMQ (jaas.leave.events via outbox) · Module 0 AuditService · Module 2 EmployeeService · Module 3 LeaveSync
```

Package: `backend/internal/leave/{models,dto,validators,calc,repositories,services,controllers,routes,events}` + `module.go`.

## 2. Components

| Component | Responsibility |
|---|---|
| `calc` | `Days` (int64 hundredths) parse/format; `CountDays(range, half, holidays, sandwich)` → total + working dates; `Accrued(policy, joining, asOf)`; `CarryForward(prevAvailable, limit)`. Pure, golden-tested (G2–G4). |
| `TypeService` | CRUD + deactivate; code uppercase immutable; uniqueness → 409. |
| `HolidayService` | create / list by year / soft delete; one per date. |
| `BalanceService` | `ensure(tx, emp, type, year, asOf)` materializes row, carry-forward, accrual top-up (ledger + `leave.accrued` outbox); self/HR views; adjust; ledger list. |
| `RequestService` | preview; apply (LV-001..LV-009, reserve); approve/reject/cancel (LV-010/011, consume/release/reversal, Module 3 LeaveSync); lists. |
| `OutboxRelay` | same pattern as Module 3: batch publish unpublished rows, attempts < 10. |
| Ports | `EmployeeDirectory` (Module 2), `AuditLogger` (Module 0), `AttendanceSync` (Module 3 `LeaveSync`), `TxRunner`, `Clock`. |

## 3. Key flows

- Apply: resolve employee → tx { lock `leave:<emp>` → load type → rules → holidays in range → CountDays → overlap check → ensure balance → available check → insert request → ledger `reserve` → balance.reserved += d → outbox `leave.applied` } → audit + publish.
- Approve: tx { load request → lock → pending? self? → ledger `release` −d and `consume` +d → reserved −= d, used += d → AttendanceSync.MarkLeave(full-day working dates) → status approved → outbox } → audit + publish.
- Cancel approved (future): tx { ledger `reversal` −d → used −= d → AttendanceSync.ClearLeave(dates) → status cancelled → outbox }.

## 4. Ledger kinds and balance columns

| kind | sign | balance column |
|---|---|---|
| carry_forward | + | opening |
| accrual | + | accrued |
| adjustment | ± | adjusted |
| reserve / release | + / − | reserved |
| consume / reversal | + / − | used |

available = opening + accrued + adjusted − used − reserved (LV-013 invariant, live G12).

## 5. Transactions & concurrency
One tx per command; per-employee `pg_advisory_xact_lock(hashtext('leave:' || employee_id))` (LV-014). Module 3 LeaveSync takes Module 3's attendance lock for the same employee inside the same tx. Lock order is always leave → attendance; punches only take the attendance lock, so no cycle.
