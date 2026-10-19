# HR / Payroll & Statutory — Design

> Part of **M1 HR Management** (bounded context `payroll`). Formerly the stand-alone "module-5-payroll" bundle; module numbers in older text were renamed to the canonical M0–M12 / HR-context names.

## Architecture

```text
Gin → M0 Identity TenantResolver → Authenticate → RequirePermission(payroll:read|manage|approve)   (self endpoints: auth only)
→ Controllers (structure, assignment, run, payslip)
→ Services (StructureService, AssignmentService, RunService, PayslipService, OutboxRelay)
→ calc (pure: Money paise, Breakdown(structure, monthlyCTC), Prorate, PF/ESI/PT/TDS, Payslip assembly)
→ Repositories (structure+components, assignment, run, payslip+lines, outbox)
→ PostgreSQL · RabbitMQ (jaas.payroll.events via outbox) · M0 Identity Audit · HR/Employee EmployeeService · HR/Leave UnpaidLeave
```

Package: `backend/internal/payroll/{models,dto,validators,calc,repositories,services,controllers,routes,events}` + `module.go`; wired in `internal/app/time_modules.go` (renamed scope: people-ops modules 3–5).

### Calculation pipeline (per employee, pure)
1. `Breakdown(spec, monthlyCTC)` → full-month component amounts (PY-001) or overflow.
2. Window + LOP (service: employee dates from HR/Employee, unpaid days from HR/Leave) → payable days (PY-003/004).
3. `Prorate(amount, payable, daysInMonth)` for earnings and fixed/percent deductions (PY-002).
4. Statutory on earned figures: PF (PY-005), ESI (PY-006, eligibility from full-month gross), PT (PY-007); TDS from full-month taxable projection (PY-008).
5. Net with cap (PY-009). Output: lines + totals.

### Run state machine
draft → calculate → calculated → (calculate again) → approve (≠ calculator) → approved → finalize → finalized. One transaction per transition; calculate deletes and re-inserts payslips (PY-011).

### Concurrency
Run transitions lock the run row (`SELECT … FOR UPDATE`); assignment writes take a per-employee advisory lock `payroll:<tenant>:<employee>`.

## Connections

| # | Source → Destination | Contract | Failure | Tests |
|---|---|---|---|---|
| C1 | Client → HR/Payroll API (20 ops) | spec §5 + swagger; JWT + tenant; `payroll:*` RBAC | 400/401/403/404/409 | controllers, routes, integration |
| C2 | HR/Payroll → HR/Employee EmployeeService | `GetByID`, `GetByUserID` (code, names, status, joining/exit dates) | 404 / skip with warning | fakes + integration |
| C3 | HR/Payroll → HR/Leave `services.UnpaidLeave` | `UnpaidDays(ctx, tenant, employee, from, to) (calc.Days, error)` — approved requests of `is_paid=false` types, clipped to range | fail calculation | HR/Leave unit + payroll integration |
| C4 | HR/Payroll → M0 Identity Audit / middleware | `Log(...)` after commit; TenantResolver, Authenticate, RequirePermission(db, "payroll", action, …) | best-effort / 401/403 | spy + integration |
| C5 | HR/Payroll → PostgreSQL | migrations 000038–000044 | rollback | integration + cycle |
| C6 | HR/Payroll → RabbitMQ `jaas.payroll.events` | envelope as HR/Attendance, HR/Leave; totals only | outbox + relay | unit |
| C7 | HR/Payroll boot → M0 Identity `permissions` | `payroll:read`, `payroll:manage`, `payroll:approve` | boot fails | integration |
| C8 | M10 Notifications, M6 Reports & Analytics, M9 AI Assistant → HR/Payroll | events | — | consumers |

## Security

Law: docs/SECURITY.md + M0 Identity security.md.

### Permissions
`payroll:read` (structures, assignments, runs, any payslip, payout CSV), `payroll:manage` (structures, assignments, create/calculate runs), `payroll:approve` (approve, finalize). Self endpoints (own assignment, own finalized payslips) need authentication only.

### Data sensitivity
Salary is confidential: amounts appear only in API responses to `payroll:read` holders or the owning employee. Audit metadata holds ids, period and counts; events hold run totals and counts only (NFR-SEC003).

### Threats
| ID | Threat | Control |
|---|---|---|
| PS-T1 | Reading another employee's payslip | self endpoints resolve the employee from JWT; foreign ids → 404 (PY-013) |
| PS-T2 | Seeing unfinalized pay | self lists only finalized runs |
| PS-T3 | One person pushing a run through | maker-checker: approver ≠ calculator (PY-012) |
| PS-T4 | Tampering after approval | approved/finalized runs reject calculate (RUN_STATE); finalized immutable |
| PS-T5 | Cross-tenant IDOR | tenant-scoped repos, 404 |
| PS-T6 | Salary leakage via logs/events | whitelisted audit/event payloads; tests assert no per-employee amounts |
| PS-T7 | Rounding drift / float errors | integer paise, golden-tested statutory math |
| PS-T8 | CSV injection in payout export | cells beginning with `= + - @` are prefixed with `'` |

### Threat → evidence (verified 2026-10-09)
| Threat | Evidence |
|---|---|
| PS-T1 | integration: member → another's payslip id 404; services TestGolden_SelfVisibilityAndPrivacy |
| PS-T2 | integration + unit: 0 payslips before finalize, unfinalized own payslip 404 |
| PS-T3 | integration + unit G11: calculator approving → 403 SELF_APPROVAL_FORBIDDEN |
| PS-T4 | unit G12: calculate after approve / finalize twice → RUN_STATE; run row locked (FOR UPDATE) |
| PS-T5 | integration G1: tenant B 404 on A's run/structure, foreign structure on assign 404, foreign JWT 403 |
| PS-T6 | integration G14: no ctc/net/amounts in audit metadata; events never name employees |
| PS-T7 | calc goldens G2–G9 (integer paise, exact ESI ceiling, rupee rounding) |
| PS-T8 | validators TestCSVSafe + services CSV test (`=HYPERLINK` neutralized) |
