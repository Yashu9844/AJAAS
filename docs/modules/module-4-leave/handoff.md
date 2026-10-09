# Module 4 — Handoff (2026-10-09, G4-9 DONE — backend CLOSED)

Completed: P1 design (re-scope Projects → Leave); P2 models + migrations 000031–000036; P3 calc/DTO/validators; P4 repositories; P5 services, events, relay, Module 3 LeaveSync; P6 HTTP edge, wiring, seed, swagger; P7 live ring; P9 DoD (checklist in current-status.md).
Not completed: P8 frontend — BLOCKED (no identity login shell).
Known issues: p95 budgets unverified on this host (D4-11); single approver (Module 6 owns chains); fixed Sat/Sun weekly off and calendar leave year (D4-04/D4-05); Projects has no module number (D4-01); Module 0 RBAC soft-deleted-role bug inherited.
Tests executed: `go vet ./...`; `go test ./internal/...` PASS; `go test -tags integration -count=1 ./tests/api/` 16/16 PASS; migrations 36 up / 6 down / 6 up; AutoMigrate↔SQL parity identical.
How to rerun live: see module-3 handoff (docker start + boot backend) then `go test -tags integration -count=1 ./tests/api/ -run Leave`.
## Phase: G4-9 → DONE; Module 4 backend CLOSED.
