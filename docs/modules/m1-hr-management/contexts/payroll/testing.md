# HR / Payroll & Statutory — Testing

> Part of **M1 HR Management** (bounded context `payroll`). Formerly the stand-alone "module-5-payroll" bundle; module numbers in older text were renamed to the canonical M0–M12 / HR-context names.

## Golden tests (protected)

| # | Behaviour | Input | Expected | Location |
|---|---|---|---|---|
| G1 | Tenant isolation | B reads A's run/payslip/structure; A JWT on B host | 404; 403 | integration |
| G2 | Breakdown | CTC 12,00,000/yr; BASIC 40% CTC, HRA 50% basic, SPECIAL balance | basic 40,000.00, HRA 20,000.00, special 40,000.00 | calc golden |
| G3 | Overflow | fixed BASIC 1,20,000 with CTC 6,00,000 | STRUCTURE_OVERFLOW | calc golden + service |
| G4 | Proration | 30-day month, payable 15 | each earning × 15/30, paise-exact | calc golden |
| G5 | PF | earned basic 30,000 / 10,000 | 1,800 / 1,200 | calc golden |
| G6 | ESI | gross 20,000 / 21,001 | 150 / 0; 20,123 → 151 (round up) | calc golden |
| G7 | PT | gross 25,000 / 24,999.99 | 200 / 0 | calc golden |
| G8 | TDS | annual taxable 12,00,000 / 15,00,000 / 30,00,000 | annual 0 / 1,09,200 (1,05,000 + 4% cess) → monthly 9,100 / 4,99,200 → monthly 41,600 | calc golden |
| G9 | Net cap | deductions > gross | net 0 + NET_CAPPED warning | calc golden |
| G10 | LOP from HR/Leave | 2 approved unpaid days in month | lop 2, payable dim−2 | service + integration |
| G11 | Maker-checker | calculator approves | 403 SELF_APPROVAL_FORBIDDEN | service + integration |
| G12 | State machine | finalize draft / calculate approved | 409 RUN_STATE | service + integration |
| G13 | Self visibility | employee lists payslips before / after finalize | 0 / 1; other's id → 404 | integration |
| G14 | Privacy | audit + outbox after full run | no per-employee amounts | integration |
| G15 | Recalculate | change assignment, recalc | payslips replaced, totals updated | service |
| G16 | Pagination | per_page=0 etc. | 200 clamped | controllers + integration |

## Test strategy and commands

RED first. Unit: calc 100% (statutory goldens G2–G9), services ≥ 90% (G10–G12, G15 on fakes), controllers/routes 100%. Integration (in-process harness, real Postgres + Redis): G1, G10–G14, G16 + full lifecycle. Migration cycle up/down/up.

```bash
go test -count=1 -cover ./internal/payroll/...
JAAS_REQUIRE_INTEGRATION=1 go test -count=1 -run Payroll ./tests/integration/
```

## Delivery evidence (status and Definition of Done at backend close)

- P1: design frozen — 20 ops, PY-001..PY-015, 7 tables, 4 events, goldens G1–G16, decisions D5-01..D5-09. No code yet.
- P2: calc.Money; models Structure, Component, Assignment, Run, Payslip (+Lines), PayslipLine, OutboxEvent — 100%; migrations 000038–000044 (CHECKs for enums, ranges, period, net ≥ 0) cycle-verified. P3 calc engine landed early: Breakdown, ValidateStructure, PF/ESI/PT/AnnualTax/MonthlyTDS, Compute with net cap — goldens G2–G9 green, 100%.
- P3: dto + validators (CTC range, period not in future, CSV formula-injection guard) — 100%.
- P4: repositories (structure, assignment, run, payslip, outbox) — compile-verified; SQL proven at P7.
- P5: services (Structure, Assignment, Run, Payslip, OutboxRelay) + events; HR/Leave UnpaidLeave port. Goldens: September lifecycle (full month, mid-month joiner 15/30, leaver 10/30, LOP 2 → 28/30, PF/PT exact, no TDS under 87A), maker-checker, state machine, recalculation, self-visibility + privacy; warnings (missing employee, overflow, net cap); fault injection. Services 94.4%.
- P6: HTTP edge (20 ops, RBAC contract test), module.go, wired in internal/app (HR/Attendance, HR/Leave, HR/Payroll as peopleModules), permission seed payroll:read|manage|approve, swagger.
- P7: integration (in-process, real Postgres + Redis): TestPayrollLifecycle — two employees assigned, member's 2 approved unpaid days flow from HR/Leave into payable/LOP (G10), maker cannot approve own calculation (G11), finalize before approve rejected (G12), nothing visible before finalize then exactly one own payslip, another's payslip 404 (G13), payout CSV, no salary in audit/events (G14); TestPayrollRBACAndIsolation — member 403, payroll:read reads but cannot calculate, tenant B 404s, foreign JWT 403, pagination (G16), preview. Full suite 61/61. Migrations 44 up / 7 down / 7 up; AutoMigrate vs SQL columns + 19 index definitions identical.

### Definition of Done (§24) — backend, 2026-10-09
- [x] Requirements — FR-ST/AS/RN/PS/EV + PY-001..PY-015 cited in code/tests
- [x] Dependencies — C1–C8 (M0 Identity auth/RBAC/audit, HR/Employee employees, HR/Leave UnpaidLeave)
- [x] Implementation — 20 ops, 7 tables, 4 events, relay, statutory engine
- [x] Code quality — AST check clean (func/file/depth/params/panic/TODO)
- [x] Unit — calc/dto/events/models/validators/routes 100%, controllers 99.4%, services 94.4%
- [x] Integration — 61/61 (2 payroll + M0 Identity, HR/Organization, HR/Employee, HR/Attendance, HR/Leave)
- [x] Contract — 20 routes ↔ 20 swagger ops (router contract test + YAML ref check)
- [x] Golden — G1–G16 (calc, services, integration)
- [x] Security — PS-T1..T8 evidence in security.md
- [x] Dependent tests — HR/Leave (UnpaidLeave port) and full suite green
- [x] No unrelated changes — outside touches: HR/Leave read port + accessor (D4-13), internal/app wiring, swagger, DEPENDENCY-GRAPH, bundle moves (D5-01)
- [x] Docs · [x] Diff reviewed · [x] No secrets
- Not run / not met: `go test -race` (no cgo); NFR-P001 (500-employee calculate < 10 s) not load-tested; mid-month CTC split not modelled (D5-10). Follow-up for M0 Identity, HR/Organization, HR/Employee owner: `internal/app.Build` is 77 lines (> 50, pre-existing).

## Manual verification checklist

From the partner frontend test guide (2026-10-09). Paths are the pre-restructure ones; after Phase C they are prefixed `/api/v1/hr`. These become the context's test-console scenarios in Phase D.

### 2. HR/Payroll — Payroll API (`/api/v1/payroll`)

Money is a JSON number in rupees with at most 2 decimals (for example `1200000` or `41666.67`).

| # | Method & path | Who | Notes |
|---|---|---|---|
| P1 | POST `/structures` | manage | `{name, description?, pf_enabled, esi_enabled, pt_enabled, tds_enabled, components:[{code,name,kind:earning\|deduction,calc:fixed\|percent_of_ctc\|percent_of_basic\|balance,value?}]}`. Exactly one `BASIC`; at most one `balance`. |
| P2–P5 | GET list / GET one / PATCH / POST `/{id}/deactivate` | read / manage | Components are immutable once a structure is assigned. |
| P6 | POST `/assignments` | manage | `{employee_id, structure_id, annual_ctc, effective_from}`. The previous assignment closes automatically. |
| P7 | GET `/assignments?employee_id` | read | History. |
| P8 | GET `/assignments/me` | self | Own CTC + monthly breakdown; `404` if none. |
| P9 | POST `/assignments/preview` | manage | `{structure_id, annual_ctc}` returns a breakdown with no save. Good for a live calculator UI. |
| P10 | POST `/runs` | manage | `{month, year}`; not in the future; one per period (`409 RUN_EXISTS`). |
| P11–P12 | GET list / GET one | read | Totals, warnings (`NET_CAPPED`, `STRUCTURE_OVERFLOW`, `NO_ASSIGNMENT`). |
| P13 | POST `/runs/{id}/calculate` | manage | Recalculating replaces the payslips. |
| P14 | POST `/runs/{id}/approve` | **approve** | The approver must differ from the last calculator (`403 SELF_APPROVAL_FORBIDDEN`). |
| P15 | POST `/runs/{id}/finalize` | approve | After approve only (`409 RUN_STATE`). |
| P16–P18 | GET `/runs/{id}/payslips`, `/runs/{id}/payout.csv`, `/payslips/{id}` | read | CSV only for finalized runs. |
| P19–P20 | GET `/payslips/me`, `/payslips/me/{id}` | self | Finalized runs only; someone else's payslip returns `404`. |

#### Payroll checklist
- [ ] Run flow: create, calculate, approve (as a **different** user with `payroll:approve`), finalize. Approving as the calculator shows the maker-checker error.
- [ ] A member sees no payslip before finalize and exactly their own after it.
- [ ] Two approved unpaid-leave days in the month reduce payable days (for example 28/30) and gross pro-rata.
- [ ] Statutory values match the preview: PF = 12% × min(Basic, ₹15,000); ESI only when the monthly gross is ≤ ₹21,000; PT ₹200 when gross is ≥ ₹25,000; TDS 0 at or below ₹12 L taxable.
- [ ] The CSV downloads with the header `employee_code,employee_name,net_pay,...`.
- [ ] Exact-grant RBAC: `payroll:read` can view but not calculate; a member gets `403` on admin endpoints.

---
