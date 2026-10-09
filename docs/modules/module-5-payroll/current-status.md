# Module 5 — Current Status (updated: 2026-10-09, P5 DONE, P6 CURRENT)

- P1: design frozen — 20 ops, PY-001..PY-015, 7 tables, 4 events, goldens G1–G16, decisions D5-01..D5-09. No code yet.
- P2: calc.Money; models Structure, Component, Assignment, Run, Payslip (+Lines), PayslipLine, OutboxEvent — 100%; migrations 000038–000044 (CHECKs for enums, ranges, period, net ≥ 0) cycle-verified. P3 calc engine landed early: Breakdown, ValidateStructure, PF/ESI/PT/AnnualTax/MonthlyTDS, Compute with net cap — goldens G2–G9 green, 100%.
- P3: dto + validators (CTC range, period not in future, CSV formula-injection guard) — 100%.
- P4: repositories (structure, assignment, run, payslip, outbox) — compile-verified; SQL proven at P7.
- P5: services (Structure, Assignment, Run, Payslip, OutboxRelay) + events; Module 4 UnpaidLeave port. Goldens: September lifecycle (full month, mid-month joiner 15/30, leaver 10/30, LOP 2 → 28/30, PF/PT exact, no TDS under 87A), maker-checker, state machine, recalculation, self-visibility + privacy; warnings (missing employee, overflow, net cap); fault injection. Services 94.4%.
