# Module 4 — Handoff (2026-10-09, G4-9 DONE — backend CLOSED)

Completed: P1 design (re-scope Projects → Leave); P2 models + migrations 000032–000037; P3 calc/DTO/validators; P4 repositories; P5 services, events, relay, Module 3 LeaveSync; P6 HTTP edge, wiring, seed, swagger; P7 live ring; P9 DoD (checklist in current-status.md).
Not completed: P8 frontend — BLOCKED (no identity login shell).
Known issues: p95 budgets unverified on this host (D4-11); single approver (Module 6 owns chains); fixed Sat/Sun weekly off and calendar leave year (D4-04/D4-05); Projects has no module number (D4-01); Module 0 RBAC soft-deleted-role bug inherited.
Tests executed: `go vet ./...`; `go test ./internal/...` PASS; `go test -tags integration -count=1 ./tests/api/` 16/16 PASS; migrations 36 up / 6 down / 6 up; AutoMigrate↔SQL parity identical.
How to rerun live: see module-3 handoff (docker start + boot backend) then `go test -tags integration -count=1 ./tests/api/ -run Leave`.
## Phase: G4-9 → DONE; Module 4 backend CLOSED.

## Update 2026-10-09 — integrated onto Module 0–2 hardening
- Branch `integrate/m3-m4-on-hardening` (from `fix/module-0-2-hardening` + this module). Supersedes `module_3_attendance` / `module_4_leave`.
- Changes: migrations renumbered (+1, now 000026–000037); wiring moved to `internal/app/time_modules.go` (relays via `app.Options.Background`); `errors.Normalize` in controllers; integration tests in `tests/integration/{attendance,leave}_test.go`.
- Tests executed: go build/vet ./...; go test ./internal/... PASS; `JAAS_REQUIRE_INTEGRATION=1 go test ./tests/integration/` 59/59 PASS (2 full runs; one earlier run had a single perf-sample spike in the partner's TestPerformanceBudgets — 3 isolated reruns PASS at 7–28 ms p95); migrations 37 up / 12 down / 12 up; setup flow verified on a fresh DB with the platform-key tenant API.
