# Module 3 — Handoff (2026-10-09, G3-9 DONE → Module 4 G4-1)

Completed: P1 design; P2 models+migrations; P3 calc/DTO/validators; P4 repositories; P5 services/events/relay; P6 HTTP edge + wiring + swagger; P7 live ring; P9 DoD close (checklist in current-status.md).
Not completed: P8 frontend — BLOCKED (frontend is a Next.js bootstrap with no identity login shell).
Known issues: Module 0 RequirePermission ignores soft-deleted roles (inherited); dev DBs created before D3-14 need a one-time index drop; `-race` not run on this host.
Files changed (P9): services/{helpers,punch_service}.go, events/events.go(+test), module.go, cmd/main.go (RouteDeps), services/harness_test.go (spy rename), tests/api/attendance_race_test.go; docs.
Tests executed: `go vet ./...` clean; `go test ./internal/...` PASS; `go test -tags integration -count=1 ./tests/api/` PASS (11).
Remaining risks: relay publish to a live RabbitMQ is not asserted end-to-end (outbox rows asserted; publish path unit-tested, G12).

## Module 4 entry point (Leave)
- D3-01 split: Module 3 = Attendance, Module 4 = Leave (masterplan). The existing docs/modules/module-4-* bundle is scoped to Projects and must be re-scoped before G4-1.
- Leave consumes from Module 3: `attendance_records.status = on_leave` (AT-020 summary counts it), events `attendance.*` on exchange `jaas.attendance.events`; Leave approval should write/override day records through a Module 3 service port, not by touching tables directly.
- Reuse the patterns: TxRunner + outbox in-tx, advisory lock per employee, `?::date` strings, ParsePage clamp, permission seed via ON CONFLICT DO NOTHING, live goldens behind `integration` tag with `newAttTenant`-style bootstrap.
## Phase: G3-9 → DONE; Module 3 backend CLOSED; next: Module 4 G4-1 (design).
