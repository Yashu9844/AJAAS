# HR / Leave & Absence — Testing

> Part of **M1 HR Management** (bounded context `leave`). Formerly the stand-alone "module-4-leave" bundle; module numbers in older text were renamed to the canonical M0–M12 / HR-context names.

## Golden tests (protected)

| # | Golden behavior | Input | Expected | Location |
|---|---|---|---|---|
| G1 | Tenant isolation | tenant B uses tenant A type/request/holiday ids; A JWT on B host | 404; 403 | live tests/api/leave_rules_test.go |
| G2 | Day counting | Mon–Fri; Fri–Mon; Fri–Mon sandwich; Sat–Sun; holiday inside; optional holiday; half day | 5; 2; 4; 0 → NO_WORKING_DAYS; one fewer; counted; 0.50 | calc/calc_golden_test.go |
| G3 | Accrual | annual 12 joined 1 Jul; monthly 18 in March; monthly 15 in December (remainder); joined next year | 6.00; 4.50; 15.00; 0 | calc/calc_golden_test.go |
| G4 | Carry-forward | prev 8 limit 5; prev −2; limit 0 | 5; 0; 0 | calc/calc_golden_test.go |
| G5 | Apply reserves | apply 2 days with available 10 | pending, reserved 2, available 8, ledger reserve +2, outbox leave.applied | services + live |
| G6 | Insufficient balance | paid available 1, apply 2; unpaid apply 2 | 409 INSUFFICIENT_BALANCE; 201 | services |
| G7 | Overlap | pending Mon–Wed then Tue; first_half + second_half same day | 409 LEAVE_OVERLAP; both 201 | services + live |
| G8 | Approve → attendance | approve 2 working days | used 2, reserved 0, ledger release −2 / consume +2, HR/Attendance records on_leave ×2 | services + live |
| G9 | RBAC | member on lists/type writes/approve; member on self endpoints | 403; 200/201 | live |
| G10 | Self-approval | approver approves own request | 403, still pending | services + live |
| G11 | Audit + privacy | apply/approve/reject/cancel/adjust | audit rows; reason never in audit metadata or outbox payload | services + live |
| G12 | Ledger invariant | after accrual, apply, approve, cancel, adjust | balance columns == ledger sums per kind | live |
| G13 | Cancel approved future | cancel approved leave starting after today | used restored, reversal row, attendance marks cleared; started leave → 409 | services + live |
| G14 | Concurrency | 5 parallel applies of 2 days with available 5 | exactly 2 succeed, available 1 | live |
| G15 | Pagination | per_page=0, page=-1, per_page=abc | 200 clamped | controllers + live |

## Test strategy and commands

Strategy: RED first per item. Unit (calc 100%, services ≥ 90%, others ≥ 80%) + unit goldens (G2–G8, G10, G11, G13) + live goldens (G1, G5, G7–G15) + migration up/down/up + AutoMigrate↔SQL parity diff.

```bash
# unit ring
go test -count=1 -cover ./internal/leave/...
# live ring (Docker PG/Redis/RabbitMQ + backend on :8080; boot steps in module-3 handoff.md)
go test -count=1 -tags integration ./tests/api/ -run Leave -v
```

### Integration onto M0 Identity, HR/Organization, HR/Employee hardening (2026-10-09)
Merged onto `fix/module-0-2-hardening` as branch `integrate/m3-m4-on-hardening`. Migrations renumbered 000025–000036 → 000026–000037 (SQL runner is the schema authority, D-HARDEN-3); module wired in `internal/app` (`time_modules.go`); controllers normalize persistence errors (unique violation → 409); live tests ported from the removed `tests/api` suite to `tests/integration` (in-process, real Postgres + Redis) — run with `JAAS_REQUIRE_INTEGRATION=1 go test -count=1 ./tests/integration/`. Full suite 59/59 PASS; migration cycle 37 up / 12 down / 12 up clean.

## Delivery evidence (status and Definition of Done at backend close)

- P1: re-scoped from Projects to Leave (D4-01); spec frozen (20 ops, LV-001..017, 6 tables, 5 events), architecture, connections C1–C9, security LS-T1..T8, goldens G1–G15.
- P2: `calc.Days` (integer hundredths; NUMERIC(7,2) via Value/Scan; JSON number) 100%; models LeaveType, Holiday, Balance (Available()), Request, LedgerEntry, OutboxEvent with index/where/expression and column-type contracts, 100%; migrations 000032–000037 (CHECK constraints for enums, ranges, half-day single date) — 36 up / 6 down / 6 up verified on scratch Postgres.
- P3: `calc` CountDays/Accrued/CarryForward/DateOf/IsWeekend — goldens G2 (day counting incl. sandwich, holidays, half day), G3 (accrual: annual proration, monthly, December remainder, joining after asOf), G4 (carry-forward) green; `dto` requests/responses + ParsePage (G15); `validators` code/amount/adjustment/date/year/gender/notice (LV-005). All four packages 100%.
- P4: `repositories` — Type, Holiday, Balance (advisory lock `leave:<tenant>:<employee>`, FindOrCreate reporting creation), Request (overlap on pending/approved), Ledger (append-only: Create + List only), Outbox; tenant-scoped; compile-verified, SQL proven at P7.
- P5: services (Type, Holiday, Balance, Request, OutboxRelay) + events; HR/Attendance LeaveSync port (D4-08/D3-17). Unit goldens G5–G8, G10, G11, G13 green; services 96.3%. Accrual events are delivered by the relay (not published inline).
- P6: controllers (100%), routes (20 ops, RBAC contract test, 100%), module.go (Ports: HR/Employee EmployeeService, M0 Identity AuditService, HR/Attendance LeaveSync; permission seed leave:read|manage|approve; relay), cmd/main.go wiring (AutoMigrate, seed, routes, relay start/stop), swagger (16 paths / 20 operations, refs validated).
- P7: live ring on Docker Postgres — tests/api/leave_flow_test.go (lifecycle: preview, apply G5, approve G8 → 2 HR/Attendance on_leave records, cancel future G13 → records removed, adjust, ledger invariant G12 via SQL, audit G11 + outbox privacy; overlap G7 + half days; pagination G15) and leave_rules_test.go (RBAC G9, self-approval G10, isolation G1, concurrency G14: 5 parallel applies → 2×201 + 3×409, available 1). All PASS; full tests/api suite 16/16 PASS. Migrations 36 up / 6 down / 6 up clean; AutoMigrate vs SQL: columns and all 20 index definitions identical. Perf note: this host showed ~70 ms per query under Docker memory pressure (login 2–4 s), so p95 budgets (NFR-P001/P002) were not meaningfully measurable here — apply 881 ms, preview 606 ms observed; re-measure on a normal host.

### Definition of Done (docs/ENGINEERING.md, Definition of Done) — backend, 2026-10-09
- [x] Requirements understood — FR-LT/HD/BL/LR/EV + LV-001..LV-017 cited in code and tests
- [x] Dependencies checked — connections C1–C9 (M0 Identity auth/RBAC/audit, HR/Employee employees, HR/Attendance LeaveSync)
- [x] Implementation complete — 20 operations, 6 tables, 5 events, relay, HR/Attendance port
- [x] Code quality — AST check: func ≤ 50, file ≤ 300, depth ≤ 4, params ≤ 4 (D3-16), no panic/TODO/nolint
- [x] Unit pass — leave calc/controllers/dto/events/models/routes/validators 100%, services 96.3%; attendance services 91.9%
- [x] Integration pass — `go test -tags integration ./tests/api/` 16/16 (5 leave + 6 attendance + 5 org)
- [x] Contract pass — live routes ↔ swagger 20/20; router contract test
- [x] Golden pass — G1–G15 (unit and/or live; see golden-tests.md)
- [x] Security verified — security.md threat → evidence LS-T1..T8
- [x] Dependent tests pass — HR/Attendance unit + live, HR/Organization live green
- [x] No unrelated modules modified — outside touches are integration points: HR/Attendance LeaveSync + record Delete + source `leave` (D4-08/D3-17), cmd/main.go, api/swagger.yaml, DEPENDENCY-GRAPH.md
- [x] Docs updated · [x] Diff reviewed · [x] No secrets (scan clean)
- Not met / not run: NFR-P001/P002 p95 not verifiable on this host (D4-11); `go test -race` not run (no cgo) — compensated by live G14 concurrency test.

## Manual verification checklist

From the partner frontend test guide (2026-10-09). Paths are the pre-restructure ones; after Phase C they are prefixed `/api/v1/hr`. These become the context's test-console scenarios in Phase D.

### 4. HR/Leave — Leave API

| # | Method & path | Who | Notes |
|---|---|---|---|
| L1 | POST `/leave/types` | `leave:manage` | `{name, code (2–10, upper-cased, immutable), is_paid=true, annual_allowance (0–365), accrual "annual"\|"monthly", carry_forward_limit, max_consecutive_days?, min_notice_days=0, allow_half_day=true, sandwich_rule=false, applicable_gender "all"\|"male"\|"female"}` |
| L2/L3 | GET `/leave/types`, `/leave/types/{id}` | any logged-in user | employees need these to apply |
| L4 | PATCH `/leave/types/{id}` | `leave:manage` | `max_consecutive_days: 0` clears the limit |
| L5 | POST `/leave/types/{id}/deactivate` | `leave:manage` | no new requests; existing ones still approvable |
| H1–H3 | POST `/leave/holidays` `{date, name, is_optional}`, GET `/leave/holidays?year`, DELETE `/leave/holidays/{id}` | manage / any / manage | one holiday per date; optional holidays count as working days |
| B1 | GET `/leave/balances/me` | self | one row per active type for the current year: `{opening, accrued, adjusted, used, reserved, available, is_paid}` |
| B2 | GET `/leave/balances?employee_id` | `leave:read` | |
| B3 | POST `/leave/balances/adjust` `{employee_id, leave_type_id, days (±, non-zero), reason}` | `leave:manage` | |
| B4 | GET `/leave/ledger?employee_id&leave_type_id` | `leave:read` | rows of kind `accrual`, `carry_forward`, `adjustment`, `reserve`, `release`, `consume`, `reversal` |
| R1 | POST `/leave/requests/preview` | self | same body as R2; returns `{total_days, available, available_after, working_dates[]}` and saves nothing |
| R2 | POST `/leave/requests` `{leave_type_id, start_date, end_date, half_day?:"first_half"\|"second_half", reason}` | self | reserves the days; status `pending` |
| R3 | GET `/leave/requests/me?status` | self | |
| R4 | POST `/leave/requests/{id}/cancel` | owner | pending at any time; approved only if it starts after today |
| R5/R6 | GET `/leave/requests?status&employee_id&from&to`, `/leave/requests/{id}` | `leave:read` | |
| R7 | POST `/leave/requests/{id}/approve` `{comment?}` | `leave:approve` | reserved becomes used; **attendance days become `on_leave`** |
| R8 | POST `/leave/requests/{id}/reject` `{comment}` | `leave:approve` | comment required |

**Days** are JSON numbers with at most 2 decimals (`1.5`); 3 decimals are rejected.
**Day counting:** Monday–Friday minus non-optional holidays; a half day is 0.5. With the sandwich rule, weekends and holidays *between* leave days are counted too.
**Balance:** `available = opening + accrued + adjusted − used − reserved`. Annual accrual is granted up front, prorated by joining month; monthly accrual is allowance/12 per month so far. Carry-forward is `min(last year's available, limit)`.
**Error codes to assert:** `INVALID_LEAVE_RANGE` (start after end, or outside the current year), `LEAVE_NOTICE`, `NO_WORKING_DAYS`, `LEAVE_TOO_LONG`, `LEAVE_TYPE_INACTIVE`, `LEAVE_TYPE_NOT_APPLICABLE` (gender), `LEAVE_OVERLAP`, `INSUFFICIENT_BALANCE` (paid types only; unpaid types skip the check), `LEAVE_NOT_PENDING`, `LEAVE_ALREADY_STARTED`, `LEAVE_NOT_CANCELLABLE`, `SELF_APPROVAL_FORBIDDEN`, `EMPLOYEE_NOT_FOUND`, `EMPLOYEE_NOT_ACTIVE`, `CONFLICT`.

---

#### Leave
- [ ] **Leave types admin:** create (code shown upper-cased), edit, deactivate; a duplicate name or code gives `CONFLICT`.
- [ ] **Holidays admin:** add, list by year, delete; a duplicate date gives `CONFLICT`.
- [ ] **Apply form + preview:** preview shows days and available-after; Fri–Mon counts 2, or 4 with the sandwich rule; a weekend-only range gives `NO_WORKING_DAYS`; a holiday inside the range is excluded. *(leave_test.go TestLeaveLifecycle, calc golden G2)*
- [ ] **Apply:** the balance card shows `reserved` up and `available` down. *(G5)*
- [ ] **Approve:** used +, reserved −, and the employee's attendance for those dates shows `on_leave`. *(G8, verified live in the attendance tables)*
- [ ] **Reject:** without a comment gives 400; with a comment the reservation is released.
- [ ] **Cancel:** a pending request is released; an approved future request is reversed and the attendance `on_leave` marks are removed. *(G13)*
- [ ] **Overlap:** an overlapping range gives `LEAVE_OVERLAP`; first half and second half of the same day are both allowed. *(G7)*
- [ ] **Insufficient balance** on a paid type gives 409; an unpaid (LOP) type lets the balance go negative.
- [ ] **Adjust + ledger:** an HR adjustment shows in the ledger with its note; the ledger rows sum to the balance. *(G12)*
- [ ] **Self-approval** gives 403. **Member** gets 403 on type/holiday writes, the request list, others' balances, the ledger, adjust and approve, but self endpoints work. *(G9/G10)*
- [ ] **Isolation:** with two tenants, ids from one return 404 in the other. *(G1)*
- [ ] **Pagination:** `per_page=0` and `page=-1` render page 1 with 20 rows and no error. *(G15)*

---

## Parity evidence (2026-10-22)

| # | Behaviour | Location |
|---|---|---|
| LG1 | A non-calendar leave year labels dates, accrues exactly and locks once balances exist | `calc/parity_test.go`, `plus` unit, `TestLeaveParity` |
| LG2 | A location holiday and week-off pattern change pricing for that location only | `calc/parity_test.go`, `plus` unit, `TestLeaveParity` |
| LG3 | The inbox shows only reports; strangers cannot decide; the calendar hides reasons | `plus` unit, `TestLeaveParity` |
| LG4 | Comp-off needs real work on a non-working day, credits once, expires without going negative | `plus` unit, `TestLeaveParity` |
| LG5 | Encashment respects the yearly cap and minimum balance and debits exactly once; events carry no reasons | `plus` unit, `TestLeaveParity` |

- Package `internal/hr/leave/plus` (migrations hr/000073–000076, up/down/up verified), 87% unit coverage; the gorm store
  is covered by integration.
- The existing leave goldens pass unchanged after the hooks.
- Contract: 27 new operations in `backend/api/swagger.yaml` (tag Leave operations).
- Console: the leave page lists 47 operations; scenario `hr.leave.plus`.
