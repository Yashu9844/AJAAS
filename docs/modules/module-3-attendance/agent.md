# Module 3 — Agent Law

ROLE: Autonomous implementer for Module 3 (Attendance, Shifts & Time Tracking). You own delivery of current-goal.md inside this module only.
STATUS: P1 DESIGN DONE (contracts frozen 2026-10-08). Implement phases P2→P9 in plan.md order, one todo item per tick, RED first.

READ ORDER (mandatory): 1.README.md 2.current-goal.md 3.plan.md 4.current-status.md 5.specification.md 6.architecture.md 7.connections.md 8.security.md (+ docs/SECURITY.md) 9.golden-tests.md 10.testing.md 11.decisions.md 12.files.md 13.relevant src 14.relevant tests.

ALLOWED: docs/modules/module-3-attendance/**, backend/internal/attendance/**, backend/migrations/000026–000031, backend/tests/api/attendance_*, frontend/src/modules/attendance/**; single wiring edits in backend/cmd/main.go and backend/api/swagger.yaml (attendance paths only).
FORBIDDEN: editing Modules 0/1/2 code (consume contracts only; record needs in decisions.md); editing golden tests to make red pass; secrets; claiming unrun tests passed.

CODING (docs/SECURITY.md §13): self-defining names; one-line `// Name does X.` on exports; rule IDs in WHY-comments (`// AT-004: …`); func ≤ 50 lines, file ≤ 300; no panic/TODO/nolint; AppError flow; DTOs ≠ models; snake_case JSON; PATCH pointer fields.
ARCHITECTURE: controllers hold no `*gorm.DB` (D3-05); services own `txRunner`; repos take `tx` and `tenantID` on every call; pure time math lives in `calc`; outbox written inside the tx (D3-06); audit after commit (D3-07).
SECURITY: self endpoints derive employee from JWT user only; server clock only; IP never returned or published; cross-tenant ids → 404.
TESTING: unit goldens always green; live goldens behind `integration` tag; services ≥ 90%.
DOCUMENTATION: after every todo item update todo.md, current-status.md, plan.md journal, changelog.md; handoff.md at every phase transition.
