# Module 5 — Specification (FROZEN at P1 2026-10-09 — contract for P2+)

Scope per masterplan (D5-01): Payroll & Statutory Compliance (India). Depends on Module 0 (tenant, auth, RBAC, audit), Module 2 (employee code, name, status, joining/exit dates), Module 4 (approved unpaid leave → loss of pay). Consumers: Modules 7, 10, 11 (events).

## 1. Purpose
Define salary structures, assign an annual CTC to employees, run a monthly payroll that computes earnings, statutory deductions (PF, ESI, Professional Tax, TDS) and loss of pay, approve and finalize it, and let employees read their own payslips.

## 2. Glossary
- **Money** — rupees with 2 decimals, held as integer paise in Go (`calc.Money`), NUMERIC(14,2) in SQL. No floats (NFR-D002).
- **Monthly CTC** — `annual_ctc / 12`. v1 treats monthly CTC as monthly gross; employer contributions are not modelled (D5-04).
- **Structure** — an ordered set of components; exactly one `BASIC`.
- **Window** — the employee's employed days inside the payroll month (joining/exit clipped).
- **LOP** — loss-of-pay days = approved unpaid leave days inside the window (Module 4).
- **Payable days** — window days − LOP. **Proration** = payable / days-in-month.

## 3. Functional requirements

### Structures (FR-ST)
- FR-ST001 Create a structure {name, description?, pf_enabled, esi_enabled, pt_enabled, tds_enabled, components[]}. Component {code `^[A-Z0-9_]{2,20}$`, name, kind `earning`|`deduction`, calc `fixed`|`percent_of_ctc`|`percent_of_basic`|`balance`, value (rupees for fixed, percent 0–100 for percents; none for balance), taxable (earnings, default true)}.
- FR-ST002 Rules: exactly one `BASIC` earning with calc `fixed` or `percent_of_ctc`; at most one `balance` earning; codes unique in the structure; `percent_of_basic` cannot apply to `BASIC`; deductions may be `fixed` or `percent_of_basic` only.
- FR-ST003 List / get / update (name, description, statutory flags) / deactivate. Components are immutable after create (create a new structure to change pay design, D5-05). Inactive structures cannot be assigned.

### Assignments (FR-AS)
- FR-AS001 Assign {employee_id, structure_id, annual_ctc (> 0, ≤ 1,000,000,000), effective_from}: closes the previous current assignment the day before; earlier-than-current effective dates → 409 `ASSIGNMENT_BACKDATED`.
- FR-AS002 Preview {structure_id, annual_ctc} → full-month breakdown incl. statutory (no save). A structure whose fixed earnings exceed monthly CTC → 400 `STRUCTURE_OVERFLOW`.
- FR-AS003 List by employee (`payroll:read`); self view of own current assignment with breakdown.

### Runs (FR-RN)
- FR-RN001 Create run {month, year}: one per tenant period (409 `RUN_EXISTS`); period not in the future (400 `INVALID_PERIOD`). Status `draft`.
- FR-RN002 Calculate (`draft`|`calculated`): (re)builds payslips for every employee with an assignment effective on or before month end whose window is non-empty; status `calculated`; totals stored. Employees whose structure overflows are skipped and listed in `warnings`.
- FR-RN003 Approve (`calculated` → `approved`), maker-checker: approver ≠ the user who last calculated (403 `SELF_APPROVAL_FORBIDDEN`).
- FR-RN004 Finalize (`approved` → `finalized`): payslips become visible to employees; run is immutable.
- FR-RN005 List runs; get run with totals; list its payslips; payout CSV (`finalized` only): `employee_code,employee_name,net_pay`.
- FR-RN006 Invalid transitions → 409 `RUN_STATE`.

### Payslips (FR-PS)
- FR-PS001 Payslip: employee snapshot (code, name), structure, annual_ctc, days_in_month, payable_days, lop_days, lines [{code, name, kind, amount}], gross_earnings, total_deductions, net_pay.
- FR-PS002 Self: list/get own payslips of **finalized** runs only. Admin (`payroll:read`): any payslip by id.

### Events (FR-EV)
- FR-EV001 Exchange `jaas.payroll.events`: `payroll.run_initiated`, `payroll.calculated`, `payroll.approved`, `payroll.finalized`; Module 3/4 envelope; outbox in-tx; relay.
- FR-EV002 Payloads carry run id, period, employee count and run totals only — never per-employee amounts.

## 4. Business rules (PY)

| ID | Rule |
|---|---|
| PY-001 | Component amount (full month): fixed = value; percent_of_ctc = monthly CTC × p%; percent_of_basic = BASIC × p%; balance = monthly CTC − Σ other earnings (< 0 → `STRUCTURE_OVERFLOW`). Rounded to paise, half away from zero. |
| PY-002 | Earned amount = full-month amount × payable_days / days_in_month, rounded to paise. Applies to earnings and fixed deductions. |
| PY-003 | Window = [max(month start, joining), min(month end, exit)]; empty window → no payslip. |
| PY-004 | LOP = approved unpaid leave days inside the window (half day 0.5), from Module 4; payable = window days − LOP, never below 0. |
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

## 5. REST API (base `/api/v1`, JWT + tenant subdomain)

| # | Method & path | Permission | Notes |
|---|---|---|---|
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

Money fields are JSON numbers with 2 decimals (e.g. `41666.67`). Pagination as Modules 3/4.

## 6. Errors
400 VALIDATION_ERROR, INVALID_PERIOD, STRUCTURE_OVERFLOW · 403 FORBIDDEN, SELF_APPROVAL_FORBIDDEN · 404 NOT_FOUND, EMPLOYEE_NOT_FOUND · 409 CONFLICT, RUN_EXISTS, RUN_STATE, STRUCTURE_INACTIVE, ASSIGNMENT_BACKDATED · 500 INTERNAL_ERROR (opaque).

## 7. Non-functional
NFR-P001 calculate for 500 employees < 10 s (one tx, batched inserts). NFR-SEC001 tenant-scoped everything, foreign ids 404. NFR-SEC002 self endpoints from JWT only. NFR-SEC003 salary amounts never in audit metadata or events (counts/totals only). NFR-D001 run state + payslips + outbox in one tx. NFR-D002 integer paise.

## 8. Data (migrations 000038–000044)
`payroll_structures` (tenant, name unique lower among non-deleted, description, pf/esi/pt/tds flags, status, timestamps, deleted_at) · `payroll_components` (structure_id, code unique per structure, name, kind, calc, value NUMERIC(14,2), taxable, position) · `payroll_assignments` (tenant, employee_profile_id, structure_id, annual_ctc, effective_from, effective_to?, created_by_user_id) · `payroll_runs` (tenant, month, year unique per tenant, status, employee_count, gross_total, deduction_total, net_total, warnings JSONB, calculated_by_user_id?, approved_by_user_id?, timestamps) · `payroll_payslips` (tenant, run_id, employee_profile_id unique per run, employee_code, employee_name, structure_id, annual_ctc, days_in_month, payable_days NUMERIC(5,2), lop_days NUMERIC(5,2), gross, deductions, net) · `payroll_payslip_lines` (payslip_id, code, name, kind, amount, position) · `payroll_events_outbox`.

## 9. Edge cases
EC-01 joiner on the 16th of a 30-day month → payable 15, earnings × 15/30. EC-02 exit on the 10th → payable 10. EC-03 3 days unpaid leave → LOP 3. EC-04 half-day unpaid → LOP 0.5. EC-05 gross 20,000 → ESI 150; gross 21,001 → ESI 0. EC-06 basic 30,000 → PF 1,800 (ceiling). EC-07 annual taxable 12,00,000 → TDS 0 (87A); 15,00,000 → see golden. EC-08 recalculate after a new assignment → new figures. EC-09 approve own calculation → 403. EC-10 finalize before approve → 409. EC-11 employee reads someone else's payslip → 404. EC-12 run for next month → 400. EC-13 structure with fixed BASIC above monthly CTC → overflow. EC-14 cross-tenant ids → 404.

## 10. Out of scope (v1)
Payslip PDF rendering (frontend/Module 10), bank-specific payout formats, employer PF/ESI contributions, old tax regime, investment declarations, arrears/bonuses/reimbursements, state-wise PT slabs and PT ceilings, multi-currency, payroll reversal after finalization.
