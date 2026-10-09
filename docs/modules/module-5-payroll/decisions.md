# Module 5 — Decisions

- D5-01 Module 5 = Payroll & Statutory Compliance (masterplan). Bundle moved from `module-9-payroll`; the Meetings scaffold that sat at 5 is now `module-x-meetings` (unnumbered, like Projects D4-01). Remaining graph numbers 6–12 still diverge from the masterplan and are fixed as each module starts.
- D5-02 Money as integer paise (`calc.Money`), NUMERIC(14,2); rounding half away from zero at paise; statutory amounts rounded to the rupee as the statutes do.
- D5-03 Loss of pay comes from Module 4 approved unpaid leave via a read-only exported port `leave/services.UnpaidLeave` (no writes into Module 4). Attendance absences are not LOP in v1 (they may be unregularized).
- D5-04 Monthly CTC is treated as monthly gross; employer PF/ESI contributions are not modelled in v1.
- D5-05 Structure components are immutable once created; new pay designs = new structure. Keeps finalized payslips explainable.
- D5-06 Professional Tax: one default slab (≥ ₹25,000 → ₹200). State-wise slabs need a tenant statutory profile (v1.1).
- D5-07 TDS: new regime FY 2025-26, full-month projection × 12, standard deduction ₹75,000, 87A rebate up to ₹12,00,000 without marginal relief, 4% cess. Approximate by design; investment declarations/old regime out of scope.
- D5-08 Maker-checker on approve (approver ≠ last calculator); finalize also needs `payroll:approve`.
- D5-09 Payslips store an employee snapshot (code, name) so later Module 2 edits never rewrite history.
