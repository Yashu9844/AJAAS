# Module 4 — Agent Law

ROLE: Autonomous implementer for Module 4 (Leave). Own delivery of current-goal.md inside this module only.
STATUS: P1 DESIGN DONE (contracts frozen 2026-10-09). Implement P2→P9 in plan.md order, RED first.

READ ORDER: README → current-goal → plan → current-status → specification → architecture → connections → security (+ docs/SECURITY.md) → golden-tests → testing → decisions → files → src → tests.

ALLOWED: docs/modules/module-4-leave/**, backend/internal/leave/**, backend/migrations/000032–000037, backend/tests/api/leave_*; wiring edits in backend/cmd/main.go and backend/api/swagger.yaml (leave paths only); Module 3 `LeaveSync` port + record source `leave` (D4-08, recorded in module-3 decisions).
FORBIDDEN: other edits to Modules 0–3 code; editing goldens to make red pass; secrets; claiming unrun tests passed.

CODING: as Module 3 agent.md — self-defining names, one-line doc comments on exports, rule IDs in WHY comments, func ≤ 50, file ≤ 300, depth ≤ 4, params ≤ 4 excluding ctx/tx (D3-16), no panic/TODO/nolint, AppError flow, DTO ≠ model, snake_case, PATCH pointers.
ARCHITECTURE: controllers hold no `*gorm.DB`; services own TxRunner; repos take `tx` + `tenantID`; day math in pure `calc` with integer hundredths; ledger rows with every balance change (LV-013); outbox in-tx; audit after commit.
SECURITY: self endpoints derive employee from JWT; reason/comment never in events or audit metadata; cross-tenant ids → 404.
DOCUMENTATION: after every phase update todo, current-status, plan journal, changelog, handoff.
