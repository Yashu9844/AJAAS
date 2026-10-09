# Module 5 — Payroll & Statutory Compliance

Purpose: salary structures, CTC assignments, monthly payroll runs (draft → calculated → approved → finalized) with Indian statutory deductions (PF, ESI, PT, TDS new regime) and loss of pay from Module 4, and self-service payslips.
Responsibilities: specification.md §3 (FR-ST, FR-AS, FR-RN, FR-PS, FR-EV) and rules PY-001..PY-015.
Inputs / Outputs: 20 REST operations (spec §5), 4 events on `jaas.payroll.events`.
Dependencies: Module 0, Module 2, Module 4 (`UnpaidLeave` port). Consumed by: Modules 7, 10, 11.
Files: files.md · Tests: testing.md · State: current-status.md · Goal: current-goal.md.
Known limitations: spec §10 (no PDFs, single PT slab, new regime only, no employer contributions).
