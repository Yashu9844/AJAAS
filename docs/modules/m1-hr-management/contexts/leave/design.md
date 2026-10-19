# HR / Leave & Absence — Design

> Part of **M1 HR Management** (bounded context `leave`). Formerly the stand-alone "module-4-leave" bundle; module numbers in older text were renamed to the canonical M0–M12 / HR-context names.

## Architecture

### 1. Layers (mirrors HR/Attendance)

```text
Gin → M0 Identity TenantResolver → Authenticate → RequirePermission(leave:read|manage|approve)   (self + type/holiday reads: auth only)
→ Controllers (type, holiday, balance, request)            thin: parse → validate → service → envelope
→ Services (TypeService, HolidayService, BalanceService, RequestService, OutboxRelay)
→ calc (pure: Days hundredths, CountDays w/ holidays + sandwich, Accrued(annual|monthly, joining, asOf), CarryForward)
→ Repositories (type, holiday, balance, request, ledger, outbox) — tenant-scoped, tx param, advisory lock
→ PostgreSQL · RabbitMQ (jaas.leave.events via outbox) · M0 Identity AuditService · HR/Employee EmployeeService · HR/Attendance LeaveSync
```

Package: `backend/internal/leave/{models,dto,validators,calc,repositories,services,controllers,routes,events}` + `module.go`.

### 2. Components

| Component | Responsibility |
|---|---|
| `calc` | `Days` (int64 hundredths) parse/format; `CountDays(range, half, holidays, sandwich)` → total + working dates; `Accrued(policy, joining, asOf)`; `CarryForward(prevAvailable, limit)`. Pure, golden-tested (G2–G4). |
| `TypeService` | CRUD + deactivate; code uppercase immutable; uniqueness → 409. |
| `HolidayService` | create / list by year / soft delete; one per date. |
| `BalanceService` | `ensure(tx, emp, type, year, asOf)` materializes row, carry-forward, accrual top-up (ledger + `leave.accrued` outbox); self/HR views; adjust; ledger list. |
| `RequestService` | preview; apply (LV-001..LV-009, reserve); approve/reject/cancel (LV-010/011, consume/release/reversal, HR/Attendance LeaveSync); lists. |
| `OutboxRelay` | same pattern as HR/Attendance: batch publish unpublished rows, attempts < 10. |
| Ports | `EmployeeDirectory` (HR/Employee), `AuditLogger` (M0 Identity), `AttendanceSync` (HR/Attendance `LeaveSync`), `TxRunner`, `Clock`. |

### 3. Key flows

- Apply: resolve employee → tx { lock `leave:<emp>` → load type → rules → holidays in range → CountDays → overlap check → ensure balance → available check → insert request → ledger `reserve` → balance.reserved += d → outbox `leave.applied` } → audit + publish.
- Approve: tx { load request → lock → pending? self? → ledger `release` −d and `consume` +d → reserved −= d, used += d → AttendanceSync.MarkLeave(full-day working dates) → status approved → outbox } → audit + publish.
- Cancel approved (future): tx { ledger `reversal` −d → used −= d → AttendanceSync.ClearLeave(dates) → status cancelled → outbox }.

### 4. Ledger kinds and balance columns

| kind | sign | balance column |
|---|---|---|
| carry_forward | + | opening |
| accrual | + | accrued |
| adjustment | ± | adjusted |
| reserve / release | + / − | reserved |
| consume / reversal | + / − | used |

available = opening + accrued + adjusted − used − reserved (LV-013 invariant, live G12).

### 5. Transactions & concurrency
One tx per command; per-employee `pg_advisory_xact_lock(hashtext('leave:' || employee_id))` (LV-014). HR/Attendance LeaveSync takes HR/Attendance's attendance lock for the same employee inside the same tx. Lock order is always leave → attendance; punches only take the attendance lock, so no cycle.

## Connections

| # | Source → Destination | Protocol | Contract | Failure | Tests |
|---|---|---|---|---|---|
| C1 | Client → HR/Leave API (20 ops) | HTTP | spec §5 + swagger; JWT + tenant; `leave:*` RBAC | 400/401/403/404/409 | controllers + routes + live |
| C2 | HR/Leave → HR/Employee EmployeeService | in-process | `GetByUserID`, `GetByID` (status, gender, joining date) | 404 / fail closed | fakes + live |
| C3 | HR/Leave → M0 Identity AuditService | in-process | `Log(...)` after commit | best-effort | spy + live G11 |
| C4 | HR/Leave → M0 Identity middleware | Gin | TenantResolver, Authenticate, RequirePermission(db, "leave", action, …) | 401/403 | live G9 |
| C5 | HR/Leave → PostgreSQL | SQL | migrations 000032–000037 | rollback | live + migration cycle |
| C6 | HR/Leave → RabbitMQ `jaas.leave.events` | AMQP | FR-EV001 envelope | outbox + relay | unit + live outbox rows |
| C7 | HR/Leave boot → M0 Identity `permissions` | SQL idempotent insert | `leave:read`, `leave:manage`, `leave:approve` | boot fails loudly | boot + G9 |
| C8 | HR/Leave → HR/Attendance `services.LeaveSync` | in-process, caller's tx | `MarkLeave(ctx, tx, tenant, employee, dates)`, `ClearLeave(ctx, tx, tenant, employee, dates)` | rollback whole approve/cancel | HR/Attendance unit + live G8 |
| C9 | HR/Payroll, M6 Reports & Analytics, M9 AI Assistant → HR/Leave | events (+ ledger reads later) | FR-EV001 payloads | — | consumers' P1 |

Trust boundary as HR/Attendance: employee for self endpoints from JWT user only; every id resolved in the caller's tenant.

## Security

Law: docs/SECURITY.md + M0 Identity security.md (STRIDE T1–T10). Module specifics below.

### Permissions
`leave:read` (lists, balances of others, ledger), `leave:manage` (types, holidays, adjustments), `leave:approve` (approve/reject). Self endpoints and type/holiday reads need authentication only. Seeded at boot (C7).

### PII
`reason` and `review_comment` are free text (may contain health data, e.g. sick leave) → never in events (FR-EV003) or audit metadata; returned only to the owner and holders of `leave:read`.

### Threats
| ID | Threat | Control |
|---|---|---|
| LS-T1 | Applying for another employee | employee from JWT only; no employee_id on self endpoints |
| LS-T2 | Cross-tenant IDOR | tenant-scoped repos; foreign ids → 404 |
| LS-T3 | Self-approval | LV-010 |
| LS-T4 | Balance race / overdraw | LV-014 advisory lock + available check in the same tx |
| LS-T5 | Silent balance tampering | append-only ledger with actor + kind (LV-013, LV-016); adjustments need `leave:manage` + reason |
| LS-T6 | Sensitive reason leakage | FR-EV003 whitelist payloads; audit metadata ids/dates/days only |
| LS-T7 | Mass assignment | DTO whitelists; status/total_days/balances server-computed |
| LS-T8 | Backdating abuse | LV-005 window (30 days, only types with min_notice 0) |

### Error hygiene
Validation → VALIDATION_ERROR with field details; unexpected → opaque INTERNAL_ERROR.

### Threat → evidence (verified 2026-10-09, P9)
| Threat | Evidence |
|---|---|
| LS-T1 | controllers TestEndpoints_SelfIgnoresClientIdentity (apply/balances ignore client ids); ApplyLeaveRequest has no employee field |
| LS-T2 | live TestLeaveLive_G1_Isolation: 5 foreign ids → 404, foreign type on apply → 404, foreign employee balance → 404, foreign JWT → 403 |
| LS-T3 | unit G10 + live G10 (admin self-approval → 403, stays pending) |
| LS-T4 | live G14: 5 parallel applies on balance 5 → exactly 2 succeed, available 1 (advisory lock `leave:`) |
| LS-T5 | live G12 ledger invariant (projection == ledger sums per kind); ledger repo exposes Create/List only; adjustments need leave:manage (G9) |
| LS-T6 | unit G11 + live G11: no reason/comment/note text in audit metadata or outbox payloads |
| LS-T7 | DTO whitelists; status/total_days/balances server-computed (unit SelfIgnoresClientIdentity sends status/total_days, ignored) |
| LS-T8 | unit TestApply_Rules (backdate window, notice) + validators NoticeOK |

Pre-production checklist (docs/SECURITY.md §12): HR/Leave items — tenant-scoped queries, DTO validation, parameterized queries, no panic, structured logs — verified at P9; JWT/bcrypt/MFA/CORS/TLS are M0 Identity / deploy scope.
