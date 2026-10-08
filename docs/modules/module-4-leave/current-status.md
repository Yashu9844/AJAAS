# Module 4 — Current Status (updated: 2026-10-09, P3 DONE, P4 CURRENT)

- P1: re-scoped from Projects to Leave (D4-01); spec frozen (20 ops, LV-001..017, 6 tables, 5 events), architecture, connections C1–C9, security LS-T1..T8, goldens G1–G15.
- P2: `calc.Days` (integer hundredths; NUMERIC(7,2) via Value/Scan; JSON number) 100%; models LeaveType, Holiday, Balance (Available()), Request, LedgerEntry, OutboxEvent with index/where/expression and column-type contracts, 100%; migrations 000031–000036 (CHECK constraints for enums, ranges, half-day single date) — 36 up / 6 down / 6 up verified on scratch Postgres.
- P3: `calc` CountDays/Accrued/CarryForward/DateOf/IsWeekend — goldens G2 (day counting incl. sandwich, holidays, half day), G3 (accrual: annual proration, monthly, December remainder, joining after asOf), G4 (carry-forward) green; `dto` requests/responses + ParsePage (G15); `validators` code/amount/adjustment/date/year/gender/notice (LV-005). All four packages 100%.
