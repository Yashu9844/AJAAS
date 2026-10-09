# Module 4 — Decisions

- D4-01 Module 4 = Leave & Absence (masterplan), following D3-01. The Projects scaffold bundle was renamed to module-4-leave; Projects has no module number until the human assigns one (DEPENDENCY-GRAPH updated).
- D4-02 Ledger is the source of truth; `leave_balances` is a projection updated in the same tx (LV-013). No generated column (masterplan) because AutoMigrate cannot express it and dev/prod parity is enforced (D3-14 style).
- D4-03 Days are integer hundredths in Go (`calc.Days`), NUMERIC(7,2) in SQL. Reason: no float drift in balances (NFR-D002).
- D4-04 Leave year = calendar year of the UTC date; requests must lie in the current year. Reason: simple, matches masterplan `year INT`; cross-year requests are split by the employee. Fiscal-year config deferred.
- D4-05 Weekly off fixed Sat/Sun; one tenant holiday calendar; optional holidays count as working days. Reason: no weekly-off config exists yet (Module 3 has none); v1.1.
- D4-06 Lazy accrual/carry-forward on first touch instead of a scheduler. Reason: no job runner in the stack; deterministic and testable.
- D4-07 Single-tier approval by any `leave:approve` holder except the requester. Reason: Module 6 (Approvals) owns chains and manager routing; Module 1 manager data is not exposed as a service.
- D4-08 Module 3 gains exported `services.LeaveSync` (MarkLeave/ClearLeave on the caller's tx) and record source `leave` — contract C10 reserved by Module 3 at P1. Recorded in module-3 decisions.md.
- D4-09 Half-day leave does not change attendance status (attendance computes half_day from work minutes); full-day leave sets `on_leave`. Punches on a leave day win (recompute → present).
- D4-10 (P5) Accrual events (`leave.accrued`) are written to the outbox inside the materializing tx but published by the relay, not inline; request events publish inline after commit. Reason: accrual can happen inside any command (even a balance read) and must never delay or fail it.
- D4-11 (P7) Performance budgets NFR-P001/P002 were not verifiable on the dev host (Docker under memory pressure, ~70 ms/query, login 2–4 s; apply observed 881 ms). Query count per apply is bounded (lock + ≤ 10 statements + holidays). Re-measure on a normal host before production; not claimed as met.
- D4-12 (2026-10-09) Integration onto Module 0–2 hardening: migrations renumbered to 000032–000037; wiring in internal/app/time_modules.go; errors.Normalize in controllers; live goldens ported to tests/integration/leave_test.go; RBAC golden G9 strengthened with an exact-grant user (leave:approve only).
