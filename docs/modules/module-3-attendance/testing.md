# Module 3 — Testing

Strategy: RED first per item. Unit (calc 100%, services ≥ 90%, module ≥ 80%) + unit goldens (G2–G8, G10–G12) + live goldens (G1, G6, G7, G9, G11–G13) + migration up/down/up.

## Commands
```bash
export PATH="$HOME/go-sdk/go/bin:$PATH"     # local Go 1.27.1
cd backend
go build ./... && go vet ./... && gofmt -l .
go test -count=1 ./internal/attendance/...
go test -count=1 -cover ./internal/attendance/...
grep -rn "TODO" internal/attendance/            # must be empty
# live ring (Docker PG/Redis/RabbitMQ + backend on :8080):
docker compose up -d postgres redis rabbitmq     # repo root
DATABASE_PASSWORD=postgres DATABASE_DBNAME=jaas_dev go run ./cmd/main.go &
go test -count=1 -tags integration ./tests/api/ -run Attendance -v
# migrations up/down/up (psql inside container, files 000026..000031)
```
`-race` needs cgo (unavailable on this Windows box) — reported as not run, never claimed.

## Suites
- `calc`: table tests incl. goldens G4/G5 (fixed instants, multiple time zones).
- `services`: fakes for repos, employee directory, audit spy, publisher spy, fake txRunner; goldens G2/G3/G6/G7/G8/G10/G11/G12.
- `controllers`: httptest with stub services — binding → VALIDATION_ERROR, status mapping, pagination clamp (G13), self endpoints never read employee_id.
- `models`: GORM schema parse, table names, unique index names.
- live (build tag `integration`): `attendance_flow_test.go` (punch flow G2/G11/G12, G13), `attendance_rules_test.go` (G6/G7/G8 regularization, G9 RBAC, G1 isolation), `attendance_race_test.go` (AS-T6: 10 concurrent INs → exactly one session), `attendance_helpers_test.go` (tenant+admin+member+employees bootstrap, punch backdating). Last run 2026-10-09: PASS. Not run: `go test -race` (no cgo toolchain on this Windows host).

## Integration onto Module 0–2 hardening (2026-10-09)
Merged onto `fix/module-0-2-hardening` as branch `integrate/m3-m4-on-hardening`. Migrations renumbered 000025–000036 → 000026–000037 (SQL runner is the schema authority, D-HARDEN-3); module wired in `internal/app` (`time_modules.go`); controllers normalize persistence errors (unique violation → 409); live tests ported from the removed `tests/api` suite to `tests/integration` (in-process, real Postgres + Redis) — run with `JAAS_REQUIRE_INTEGRATION=1 go test -count=1 ./tests/integration/`. Full suite 59/59 PASS; migration cycle 37 up / 12 down / 12 up clean.
