# M2 Projects & Tasks - testing

Golden tests `PG1`-`PG12` are defined in `specification.md` section 10. They run as tests on a real PostgreSQL (no repository
fakes), as integration tests through the production router, and as console scenarios.

## Inventory (2026-10-28)

| Layer | Where | Count | Coverage (statements) |
|---|---|---|---|
| Kernel (ranks, cursors, errors, HTTP helpers, dates) | `internal/projects/kernel` | 8 tests | 88% |
| API contract (roles, `Require`, actor) | `internal/projects/api` | 4 tests | 100% |
| Project context | `internal/projects/project` | 19 tests incl. statement-level fault injection | 95% services (HTTP handlers are covered by the integration ring) |
| Task context | `internal/projects/task` | 48 tests + ~35 sub-tests (fault injection per table) | 95% |
| Collaboration | `internal/projects/collab` | 37 tests incl. a real S3 round trip | 92% |
| Planning and views | `internal/projects/planning` | 45 tests | 93% |
| Boundary | `internal/projects/boundary_test.go` | M2 imports only shared packages, `identity/access` and itself | - |
| Integration ring | `tests/integration/projects*_test.go`, `inbox_test.go` | 12 tests | production router, real Postgres and Redis |
| Console scenarios | `frontend/src/modules/devtest/scenariosPm*.ts` | 11 scenarios on five pages | 11/11 pass live in headless Chrome (296 steps, 2026-10-28) |

## Golden tests

| ID | Proven by |
|---|---|
| PG1 idempotent create | `TestProjectsContext` (project), `projects_tasks_test.go` (task replay with `Idempotency-Key`) |
| PG2 concurrent updates, none lost | task package concurrency tests (goroutines per version); `go test -race` clean |
| PG3 dependency cycles incl. concurrent inserts | task package relation tests, one advisory lock per tenant |
| PG4 isolation: non-members, private projects, foreign tenant ids -> 404 | every context; `TestProjectsContext`, `TestAccessRules` |
| PG5 gapless unique task numbers under concurrency | task package create tests |
| PG6 one active sprint, one running timer under races | planning and collab race tests (partial unique indexes) |
| PG7 large board pagination | `planning/perf_test.go` (`JAAS_PERF=1`): 50 000 tasks, board walk of a 30 000-task column in 300 pages, p95 111 ms against a 500 ms budget (full table in `notes-planning.md`) |
| PG8 attachment ACL, presigned expiry, wrong size/type | collab attachment tests, real S3 round trip |
| PG9 tenant-isolated search | planning search tests |
| PG10 event replay idempotent, outbox never loses a committed event | `TestDeactivationAndInbox`, `TestInboxClaimIsOncePerConsumer`, `TestOutboxRelay` |
| PG11 mentions never leak a task | collab mention tests, `TestTaskDescriptionMentionsAndAutoWatchers` |
| PG12 events and audit hold ids only | per-context payload tests |

## Commands

```bash
cd backend && export PATH="$HOME/go-sdk/go/bin:$PATH"
JAAS_REQUIRE_INTEGRATION=1 go test ./internal/projects/... -count=1 -cover      # needs Postgres and (collab) the dev S3 service
JAAS_REQUIRE_INTEGRATION=1 go test ./tests/integration/ -count=1                 # the whole ring
JAAS_PERF=1 go test ./internal/projects/planning/ -run Perf -count=1             # PG7 seed (~15 s)
```

Parallel integration runs must use different `JAAS_TEST_DB_NAME` and `JAAS_TEST_REDIS_DB` values.

## Method notes

- Service tests run against a throw-away database migrated from `backend/migrations/projects` (`internal/projects/testdb`).
- `project/faults_test.go` and the task fault tests fail every SQL statement of the main flows in turn: each must return an error and
  leave no partial change. They found seven silently-ignored database errors during the build (DPJ-24).
- Code limits are checked by an AST lint (function <= 50 lines, file <= 300, <= 4 parameters not counting `context.Context`, doc
  comments on exported methods, no TODO/panic/nolint) - clean on `internal/projects`.
