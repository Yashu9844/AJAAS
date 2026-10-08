# Module 3 — Handoff (2026-10-09, G3-7 DONE → G3-9 CURRENT)

Completed: P2 models+migrations; P3 calc/DTO/validators; P4 repositories; P5 services/events/relay; P6 HTTP edge + wiring + swagger; P7 live ring (goldens G1, G2, G6–G9, G11–G13 on real Postgres, migration cycle, schema parity).
Not completed: P8 frontend (blocked: no identity login shell), P9 DoD close.
Known issues: Module 4 re-scope pending (D3-01); Module 0 RequirePermission soft-deleted-role bug inherited; dev DBs booted before D3-14 need a one-time index drop.
Files changed (P7): backend/tests/api/attendance_{helpers,flow,rules}_test.go (new), tests/api/{db,helpers}_test.go (login limiter reset, D3-15), backend/internal/attendance/models/{shift,attendance,models_test}.go (parity tags + tests); docs: spec AT-004 note, decisions D3-13..15, golden-tests locations, plan/todo/status.
Tests executed: `go vet ./...` clean; `go test ./internal/...` PASS; `go test -tags integration ./tests/api/` PASS (5 attendance + 5 org); migrations 000001–000030 up, 000030–000025 down, 000025–000030 up on scratch DB `jaas_migcheck` (dropped after); column+index diff AutoMigrate vs SQL: identical.
How to rerun live: `docker start ajaas-postgres-1 ajaas-redis-1 ajaas-rabbitmq-1`; boot backend (`APP_ENV=development DATABASE_PASSWORD=postgres DATABASE_DBNAME=jaas_dev go run ./cmd`); `go test -tags integration -count=1 ./tests/api/`.
Remaining risks: relay publish to a live RabbitMQ not asserted (outbox rows asserted; publish covered by unit golden G12).
Required follow-up (G3-9): §24 checklist, DEPENDENCY-GRAPH, Module 4 handoff.
## Phase: P7 → DONE, P9 → CURRENT, Module 4 G4-1 → NEXT.
