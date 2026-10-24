# M1 HR - acceptance checklist, item by item (2026-10-24)

Source: the 100 items in `docs/ARCHITECTURE.md` section 9, which that section labels a **proposed** list rather than the owner's
canonical one. Each item is judged against what the code and tests in this repository show. If you provide the canonical list,
each of its items can be mapped onto these rows.

**Result: 45 met, 25 partial, 24 not met, 6 not applicable to M1.**

Most of the "not met" items are platform and operations concerns (CI, metrics, tracing, backups, runbooks, load tests, a secret manager)
that sit outside the HR domain code. They are open work, not claimed as done.

## Architecture and ownership

| # | Status | Evidence or gap |
|---|---|---|
| 1 | Partial | Purpose per context: `README.md`, `contexts/*/specification.md`. No named human owner per context. |
| 2 | Met | Per-context tables and the `hr` schema; `TestSchemasAreOwnedAndIsolated`. |
| 3 | Met | Contexts reach each other only through `dto`/`api` ports; `TestImportBoundaries`. |
| 4 | Met | `specification.md` section 10 and each context `design.md`; the `api` packages are the contracts. |
| 5 | Met | Import boundary test forbids cycles by construction; late-bound ports (`SetBenefits`, `SetStates`) are one-way. |
| 6 | Partial | ARCHITECTURE sections 5 and 12 map modules to services. The screen-to-API map (deliverable 12) is not built. |
| 7 | Met | No dashboard context exists in M1. |
| 8 | Partial | Routes are under `/api/v1`; events carry `event_version`. No compatibility-window policy or test. |
| 9 | Not met | HR runs in-process; no extraction or rollback plan is written. |
| 10 | Met | `decisions.md` per context plus the DHR decision log. |

## API and contracts

| # | Status | Evidence or gap |
|---|---|---|
| 11 | Met | Uniform `{data, meta}` / `{error: {code, message}}` envelopes, snake_case, UUID ids. |
| 12 | Met | `backend/api/swagger.yaml` (442 operations); `TestSwaggerMatchesRoutes`. |
| 13 | Partial | Inputs are separate types. Single-package contexts (compensation, onboarding, learning, benefits) return their models as responses. |
| 14 | Partial | Codes are stable and listed in each context spec section 4. There is no central error catalogue. |
| 15 | Met | `utils.ParsePagination` caps `per_page` at 100. |
| 16 | Met | Filters and statuses are validated against fixed sets; sorts are fixed in the stores. |
| 17 | Partial | `X-Correlation-ID` middleware and audit; events from the older contexts carry `correlation_id`, the newer ones do not. |
| 18 | Met | Server read/write/header timeouts in `cmd/main.go`; `BodyLimit` 1 MiB, including chunked bodies (`TestBodyLimitGivesChunkedBodiesALength`). |
| 19 | Not met | No breaking-change or compatibility-window policy. |
| 20 | Partial | Contract test exists and runs in the integration ring. The repository has no CI pipeline. |

## Idempotency and consistency

| # | Status | Evidence or gap |
|---|---|---|
| 21 | Partial | `Idempotency-Key` is honoured on every write route when sent (HR-010); it is not mandatory. |
| 22 | Met | Unique key is scope (`tenant:user`), method, path (which holds `/api/v1`) and key. |
| 23 | Met | `RequestHash`; `TestIdempotencyKeyReplaysAndRejectsReuse`. |
| 24 | Met | The claim is a database unique constraint. |
| 25 | Met | The stored status and body are replayed. |
| 26 | Met | `idempotency_records` table, not Redis. |
| 27 | Not met | No consumers exist yet, so no inbox is built. |
| 28 | Met | Every service writes its outbox row in the business transaction (`Event(tx)`). |
| 29 | Met | Relays retry and never republish (`TestOutboxRelay`). |
| 30 | N/A | M1 calls no external providers. |
| 31 | Partial | Hire saga state is durable (`hired_user_id`, `seat_reserved`); there is no generic saga table. |
| 32 | Partial | A retried hire reuses its reservation; relay retries are capped at 10. Saga retries are not otherwise observable. |
| 33 | Met | Hire: `POST /candidates/{id}/hire/release` gives the seat back (RP-01). |
| 34 | Partial | Row locks (`FOR UPDATE`) and advisory locks inside transactions; there are no version columns or ETags for read-modify-write over the API. |
| 35 | Met | Race tests: last seat (RG1), one live election (BG2), joiner enrolment once (LG1), and others. |

## Tenancy and security

| # | Status | Evidence or gap |
|---|---|---|
| 36 | Met | `tenant_id` on every HR table. |
| 37 | Met | Stores filter by tenant; foreign-tenant ids return 404 (isolation tests). |
| 38 | Partial | HR contexts use no cache. Access-cache keys in M0 were not re-audited here. |
| 39 | Met | Exports (payroll CSV) are tenant-scoped; there is no search service. |
| 40 | N/A | No event consumers exist. |
| 41 | Met | The tenant comes from the Host and the verified token only. |
| 42 | Met | Object-level checks: owner, manager and permission scopes, 404 for others (tests per context). |
| 43 | Partial | M0 owns role escalation; not re-audited in this work. |
| 44 | Met | Statutory and bank data masked; separate `*_sensitive` permissions (employee, compensation, benefits). |
| 45 | Not met | Secrets come from environment and config files (including a dev platform key); no secret manager. |
| 46 | Partial | Not audited; login failure audit stores the email. |
| 47 | N/A | HR is in-process; `identityport` is the seam for a later split. |
| 48 | Partial | Failed-login limiter (10 per 15 minutes). Expensive endpoints (payroll calculate, bulk import) have no limiter. |
| 49 | Met | `login.failed` is audited and counted by the failure limiter. |
| 50 | Not met | Audit rows are ordinary rows; no tamper-resistance. |

## Data and storage

| # | Status | Evidence or gap |
|---|---|---|
| 51 | Met | UUID primary keys. |
| 52 | Met | Multi-table writes are transactional, with rollback tests under failure injection. |
| 53 | Met | `kernel.Money` is int64 hundredths; no floats. |
| 54 | Met | Leave ledger append-only; completions immutable; elections ended, never edited. |
| 55 | Met | Explicit state machines; illegal transitions return 409 (tests per context). |
| 56 | Met | Unique and partial unique indexes for seats, elections, enrolments, offers and others. |
| 57 | Met | Foreign keys and indexes exist for the access patterns. Query plans were not reviewed (see 74). |
| 58 | Met | Migrations are owned per schema (`migrations/identity`, `migrations/hr`). |
| 59 | Partial | Up/down/up verified by script for each new range; `TestMigrationsAreIdempotentAndComplete` is in the ring. The down path is not in the ring. |
| 60 | Not met | No expand/contract policy; migrations were re-baselined. |
| 61 | Not met | No retention or deletion policy except idempotency expiry. |
| 62 | Not met | No backup monitoring. |
| 63 | Not met | No restore drills. |
| 64 | Partial | Payroll CSV is synchronous and bounded by run size; no asynchronous export. |
| 65 | N/A | Documents are metadata only (object storage arrives with M8). |

## Performance and scale

| # | Status | Evidence or gap |
|---|---|---|
| 66 | Met | `TestPerformanceBudgets` asserts single GET p95 under 200 ms on the measured endpoints. |
| 67 | Met | The same test asserts list p95 under 500 ms. |
| 68 | Partial | Login p95 is measured and reported, not asserted (bcrypt-dominated). |
| 69 | Not met | DB query p95 under 50 ms is not measured. |
| 70 | Partial | Only the endpoints in the budget test are measured. |
| 71 | Not met | No load test. |
| 72 | Not met | No scale test. |
| 73 | Not met | No systematic N+1 test (some list views do per-row lookups, for example the pending-completions list). |
| 74 | Not met | No query-plan review. |
| 75 | Met | Pagination caps; relays batch 100. |
| 76 | N/A | HR contexts use no cache to document. |
| 77 | Not met | No queue-lag or outbox-age monitoring. |
| 78 | Partial | One relay goroutine per outbox with a batch cap; no explicit backpressure. |
| 79 | Met | Pool bounded: 25 open, 10 idle, 5 minute lifetime. |
| 80 | N/A | No dashboards in M1. |

## Resilience and operations

| # | Status | Evidence or gap |
|---|---|---|
| 81 | Met | `/health/live` and `/health/ready` are distinct, with a readiness timeout. |
| 82 | Partial | Server and readiness timeouts are set; per-query context timeouts are not enforced everywhere. |
| 83 | Partial | Relay attempts are capped at 10 on a fixed 30 s tick; no exponential backoff or jitter. |
| 84 | Not met | No circuit breakers. |
| 85 | Not met | No dead-letter queue; rows past 10 attempts are left for an operator. |
| 86 | Met | `srv.Shutdown` on SIGTERM/SIGINT; background loops stop on context cancel. |
| 87 | Partial | The API is stateless, but rolling deployment is not exercised or documented. |
| 88 | Not met | No canary or feature-flag rollout (the `Flag` middleware is a permission flag). |
| 89 | Not met | No runbooks. |
| 90 | Not met | No incident metrics or error budgets. |

## Observability, testing and delivery

| # | Status | Evidence or gap |
|---|---|---|
| 91 | Met | Structured logs (zerolog) with request and correlation ids. |
| 92 | Not met | No RED metrics. |
| 93 | Not met | No distributed tracing. |
| 94 | Partial | Events and audit exclude sensitive data (leak tests); log redaction was not audited. |
| 95 | Met | Unit tests cover the domain rules in every context. |
| 96 | Partial | Integration ring runs against real Postgres. Broker behaviour is covered with a capturing publisher, not a real broker. |
| 97 | Met | Failure-injection fakes in every service test (`fail[...]` maps). |
| 98 | Met | Service layers 85-100% (for example learning 89%, benefits 90%, recruitment/plus 88%, payroll services 92%). |
| 99 | Not met | The repository has no CI pipeline; the checks run as local commands. |
| 100 | Not met | No release rollback or reconciliation plan. |
