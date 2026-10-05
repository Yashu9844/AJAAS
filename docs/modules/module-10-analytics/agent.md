# Module 10 — Agent Law

ROLE: Autonomous implementer for Module 10 (Analytics Platform). You own delivery of current-goal.md inside this module only.
STATUS: DESIGN PENDING. Do NOT write production code until plan.md P1 design is DONE and contracts are frozen. Docs and scaffold tasks only.

READ ORDER (mandatory): 1.README.md 2.current-goal.md 3.plan.md 4.current-status.md 5.specification.md 6.architecture.md 7.connections.md 8.security.md (+ docs/SECURITY.md) 9.testing.md 10.files.md 11.relevant src 12.relevant tests.

ALLOWED: docs/modules/module-10-analytics/**, backend/internal/analytics/**, frontend/src/modules/analytics/**, module tests.
FORBIDDEN: editing another module without a contract-change record; editing shared/ contracts without decisions.md entry + dependent tests; any prod secret; editing golden tests to make red pass.
CODING: docs/SECURITY.md section 13 (self-defining names, one-line docs, size limits, no panic/TODO/disables, AppError flow, DTOs differ from Models, tenant_id on every scoped query).
ARCHITECTURE: mirror Module 0 Clean layers (controller thin, service owns logic, repo owns data). Consume Module 0 contracts only — never its internals.
SECURITY: Aggregation only. Tenant-scoped always; no PII in exports without explicit consent + audit. Read security.md before security-sensitive code. Generic errors, no secrets in logs or responses, audit every state change.
TESTING: RED first. Unit + integration + contract + golden + security. Never claim an unrun test passed.
HANDOFF: fill handoff.md + phase transition every task. Update plan.md map in the same task.
