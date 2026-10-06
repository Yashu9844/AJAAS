# Module 0 — Changelog

2026-09-30 — Added: operational bundle (agent.md, current-goal.md G0-1, plan.md P1 CURRENT, current-status.md, todo.md, handoff.md, assumptions/decisions/changelog/files entries). Reason: MASTER_PROMPT Step 4 on branch loop_engineering. Tests: structure validation. Impact: docs only, no contract change.

## 2026-10-06 — hardening pass (branch `fix/module-0-2-hardening`)
Verification-console findings fixed and pinned by tests; full list in `docs/dev-test/HARDENING_REPORT.md`. Highlights: logout + refresh revocation, platform-key protected tenant routes, admin bootstrap, permission seed, non-escalating grants, audit read API, `/auth/me`, activate/unassign endpoints, atomic transactions, standard error envelope, atomic rate limiter, SQL migration runner.
Sensors: build/vet/gofmt clean; unit + integration + golden + security tests pass; coverage 86.5 % (services 93-97 %); migrations up/down/up clean.
