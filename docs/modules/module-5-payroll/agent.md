# Module 5 — Agent Law

ROLE: Autonomous implementer for Module 5 (Payroll). STATUS: P1 design frozen 2026-10-09; implement P2→P9 RED first.
READ ORDER: README → current-goal → plan → current-status → specification → architecture → connections → security → golden-tests → testing → decisions → files → src → tests.
ALLOWED: docs/modules/module-5-payroll/**, backend/internal/payroll/**, backend/migrations/000038–000044, backend/tests/integration/payroll_*; wiring in backend/internal/app; swagger payroll paths; Module 4 read-only `UnpaidLeave` port (D5-03).
FORBIDDEN: other edits to Modules 0–4; editing goldens to pass; secrets; claiming unrun tests passed.
CODING: as Modules 3/4 (func ≤ 50, file ≤ 300, depth ≤ 4, params ≤ 4 excl. ctx/tx, no panic/TODO). Money only as `calc.Money` (paise); statutory math only in pure `calc`.
SECURITY: salary amounts never in audit metadata or events; self endpoints from JWT; maker-checker on approve.
DOCS: update todo, status, plan journal, changelog, handoff every phase.
