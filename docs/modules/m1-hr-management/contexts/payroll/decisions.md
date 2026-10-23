# HR / Payroll & Statutory — Decisions

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

## Changelog

2026-10-09 — P1 DONE: HR/Payroll = Payroll (D5-01); bundle moved from module-9-payroll; design frozen.
2026-10-09 — P2 DONE: money codec, 7 models, migrations 000038–000044; P3 calc engine goldens G2–G9 green.
2026-10-09 — P3 DONE: DTOs, validators.
2026-10-09 — P4 DONE: repositories.
2026-10-09 — P5 DONE: payroll services, events, relay; HR/Leave UnpaidLeave port; goldens green.
2026-10-09 — P6 DONE: HTTP edge, app wiring, swagger.
2026-10-09 — P7+P9 DONE (backend CLOSED): integration goldens, parity, DoD.

## Parity decisions (2026-10-23)

- **DP-01** Employer contributions are payslip lines of kind `employer`. They are outside gross, deductions and net, so
  every existing payslip, CSV and total keeps its meaning; `employer_cost` and the run `employer_total` carry the
  cost. The migration widens the line-kind check; the down migration deletes employer lines before narrowing it.
- **DP-02** A payslip is always computed from segments (`calc.ComputeSegments`). One segment equals the original
  `Compute` (a golden proves it), so mid-month CTC changes need no second code path.
- **DP-03** State professional tax: tenant slabs override built-in defaults per state; a worker with no state keeps the
  PY-007 single slab. The built-in KA, MH, WB, GJ, AP and TS slabs are configuration data that must be verified against
  current state law before production use; half-yearly states (TN, KL) are not built in and are set by the tenant.
- **DP-04** Statutory deductions are not recomputed on arrears in v1. Arrears are a taxable earning added after the
  statutory base is fixed. Recomputing PF/ESI/PT/TDS across past months is a larger design (supplementary payslips) and
  is deferred to P2.
- **DP-05** A payslip stores the segments it was priced with. Arrears are only considered when the current assignment
  history differs from the stored segments, so unchanged history can never produce a spurious difference, even when LOP
  fell in one segment only. Where it did change, LOP is spread over the new segments by calendar days.
- **DP-06** Each source payslip is settled by the amount computed at that time and recorded per run; later runs subtract
  everything other runs settled, so arrears are paid exactly once and recalculation replaces only its own rows.
- **DP-07** Benefits plug in through `BenefitDeductions` behind a switch installed after construction, because
  HR/Benefits is built after payroll. Without it nothing is deducted.
- **DP-08** Final settlement excludes the last month salary, which the normal run pays with the window clipped at the
  exit date (PY-003). Gratuity uses the last full-month BASIC (15/26 per year, ₹20 lakh cap, five completed years,
  six-month remainder rounds up). Notice recovery is skipped when the resignation date is missing or after the exit
  date, and is capped at the settlement earnings, with the rest reported as `recovery_shortfall`.
- **DP-09** Approving a settlement needs the exit clearance from HR/Onboarding, which is also what HR/Onboarding publishes
  for exactly this purpose (onboarding OB-07). Paying marks the worker approved encashments paid in HR/Leave in the
  same transaction, so an encashment can never be paid twice.
