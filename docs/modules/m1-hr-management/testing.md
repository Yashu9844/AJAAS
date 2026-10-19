# M1 HR Management — Testing

> Service-level strategy and index. Each context's protected goldens, commands and delivery evidence are in
> `contexts/<context>/testing.md`.

## 1. Rings

| Ring | What | Where | Gate |
|---|---|---|---|
| Unit | pure engines (calc, score, lifecycle, pipeline graphs), validators, DTOs, services with fakes (snapshot rollback, failure injection), controllers, route ↔ permission contract tests | `backend/internal/<context>/**/_test.go` (target `backend/internal/hr/<context>/...`) | 100% engines/validators/DTOs/models; services ≥ 90%; overall ≥ 80% |
| Integration | in-process production router against real Postgres + Redis; each test provisions its own tenant through the platform API | `backend/tests/integration/` | all green with `JAAS_REQUIRE_INTEGRATION=1` |
| Golden | protected behaviours per context (below); never edited to make a red test pass | unit and/or integration | all green |
| Schema | migration up → down → up for the context's range; AutoMigrate ↔ SQL parity (columns + index definitions) | `schema_test.go` + migration check script | identical |
| Contract | every route has the documented permission chain; swagger ↔ routes | route contract tests + YAML ref check | exact |
| Security | exact-grant RBAC (user holding only the named permission), tenant isolation (foreign id 404, foreign JWT 403), payload/audit/event scans for forbidden fields | `security_test.go`, context integration tests | all green |
| Performance | p95 budgets (`docs/ARCHITECTURE.md` §3.9) | `perf_test.go` | measured on a normal host; flaky under host load (known) |
| Manual | test-console scenarios per context (`docs/TEST_CONSOLE.md`) | frontend `/dev-test` | every scenario passes against the live backend |

## 2. Commands

```bash
# infra
docker start ajaas-postgres-1 ajaas-redis-1 ajaas-rabbitmq-1
cd backend
export PATH="$HOME/go-sdk/go/bin:$PATH"
go build ./... && go vet ./...
go test -count=1 ./internal/...                                             # unit ring
JAAS_REQUIRE_INTEGRATION=1 go test -count=1 -timeout 20m ./tests/integration/  # integration ring (DB jaas_itest, Redis DB 15)
go test -cover ./internal/<context>/...                                      # coverage per context
```

`go test -race` needs cgo, which this host lacks; concurrency is covered by live tests (parallel requests against
real Postgres) instead.

## 3. Golden index

| Context | Goldens | Highlights |
|---|---|---|
| organization | G1–G10 | isolation, acyclic hierarchy, depth cap, single primary mapping, user-deactivation convergence |
| employee | G1–G10 | atomic onboarding, PII masked by default, sensitive-access tiers, status state machine |
| attendance | G1–G13 | punch precedence, totals, regularization lifecycle, audit + outbox privacy |
| leave | G1–G15 | day counting (sandwich, holidays, half day), accrual, carry-forward, overlap, self-approval block, ledger invariant, concurrent applies |
| payroll | G1–G16 | statutory math (PF, ESI, PT, TDS under 87A), proration, LOP from leave, maker-checker, state machine, self-visibility |
| recruitment | G1–G14 | pipeline graph, stage history, interview access, offer rules, hire saga idempotency, no PII in events |
| performance | G1–G15 | weight cap, lifecycle, access policy, review launch, 9-box (designed; G2, G3 green) |
| core, onboarding, compensation, learning, benefits | candidates in `specification.md` §4–8 | frozen at each context's design phase |

## 4. Integration suite (now)

`identity_test.go` (20), `org_test.go` (8), `employee_test.go` (7), `attendance_test.go` (6), `leave_test.go` (5),
`payroll_test.go` (2), `recruitment_test.go` (2), `golden_test.go` (6), `security_test.go` (3), `schema_test.go` (2),
`perf_test.go` (1), `smoke_test.go` (1) — 63 tests, all green on `main` (2026-10-09). Harness helpers:
`newTenant`, `newTimeTenant`, `limitedUser(perms…)`, `call`, `expect`, `expectErr`, `sqlStr`.

## 5. Restructure safety net (Phase C)

The code move must be behaviour-preserving. Before the move, the full unit and integration rings are run and their
results recorded; after each step the same rings must be green with only path changes in tests (route prefix
`/api/v1/hr`). Additional checks added in Phase C:

- import-boundary test: no HR package imports `internal/identity/...`; no context imports another context except its
  `api` package;
- schema test: every HR table is in schema `hr`, no foreign key leaves the schema;
- idempotency tests per retryable command (same key replay, same key different payload, concurrent duplicates);
- event envelope test: every produced event has `event_version`, `correlation_id`, `causation_id` and no forbidden
  fields.

## 6. Test console scenarios

Each context gets a console page listing its endpoints and its goldens as scenarios (`docs/TEST_CONSOLE.md`).
The organization and employee pages exist (built for the M0–HR verification on 2026-10-07); the other contexts are
added in restructure Phase D, and every planned context ships its page with its backend.
