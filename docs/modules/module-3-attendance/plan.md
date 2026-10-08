# Module 3 — Plan (updated: 2026-10-09, owner: loop — P6 DONE, P7 CURRENT)

## Phase Map

| Phase | Goal ID | Objective | Depends On | Status | Proof |
|---|---|---|---|---|---|
| P0 | G3-0 | Bundle scaffold + dependency metadata | — | DONE | 2026-09-30 scaffold |
| P1 | G3-1 | Design: spec (FR/AT), architecture, connections, goldens, frozen REST/event/data contracts | Modules 0+2 merged | DONE | specification.md, architecture.md, connections.md, golden-tests.md (2026-10-08) |
| P2 | G3-2 | Models (6) + migrations 000025–000030 + model tests | P1 | DONE | models_test 100% cov; SQL pairs written (live up/down/up verified at P7) |
| P3 | G3-3 | Pure `calc` engine (date attribution, totals, status) + DTOs + validators; goldens G4/G5 | P2 | DONE | goldens G4/G5 + G13 (ParsePage) green; calc/dto/validators 100% cov |
| P4 | G3-4 | Repositories (6, tenant-scoped, tx param, advisory lock) | P2 | DONE | 6 repos + interfaces compile; tenant-scope audit clean (outbox relay cross-tenant by design); SQL verified live at P7 |
| P5 | G3-5 | Services (shift, assignment, punch, query, regularization, outbox relay) + events; goldens G2/G3/G6/G7/G8/G10/G11/G12; services ≥ 90% | P3+P4 | DONE | 7 services + events + relay; goldens G2,G3,G6,G7,G8,G10,G11,G12 green; services 91.8%, events 100% |
| P6 | G3-6 | Controllers + routes + module.go + main.go wiring + permission seed + swagger (19 paths); G13 | P5 | DONE | controllers 100%, routes 100% (19-route + RBAC contract test), module.go + seed + relay, main.go wired, swagger 16 paths/24 schemas validated |
| P7 | G3-7 | Live ring: Docker PG boot, migrations up/down/up, live goldens G1/G6/G7/G9/G11/G12/G13 | P6 | CURRENT | — |
| P8 | G3-8 | Frontend attendance slice vs live API | P7 + identity frontend shell | LATER | — |
| P9 | G3-9 | Hardening + DoD close (§24), handoff to Module 4 | P7 | NEXT | — |

Status vocabulary: DONE / CURRENT (one) / NEXT (one) / LATER / BLOCKED. Promotion = MASTER_PROMPT §24 + handoff.

## Current Phase
- Goal: G3-7 live ring — Docker PG/Redis/RabbitMQ up, backend booted on :8080, SQL migrations 000025–000030 up/down/up against a scratch DB, live goldens (build tag `integration`) G1, G6, G7, G9, G11, G12, G13 + a full punch flow, exercising every repository method against real Postgres.
- Why now: repositories have only been exercised by fakes; this is the first real-SQL proof.
- Scope in: backend/tests/api/attendance_*_test.go; fixes in backend/internal/attendance/** only where live runs expose real bugs.
- Exit criteria: live goldens PASS; migration cycle clean; schema parity AutoMigrate vs SQL noted; docs updated.
- Risks: dev DB carries Module 0–2 data from earlier org runs (mitigate: unique tenant slugs per run); RabbitMQ optional (NoOp fallback is acceptable — outbox rows then rely on the relay).

## Next Phase
- Goal: G3-9 hardening + DoD close (§24) and handoff to Module 4 (P8 frontend stays LATER: blocked on identity login shell).

## Gate Log
| Date | Phase | Gate | Decision | By | Reason |
|---|---|---|---|---|---|
| 2026-09-30 | P0 | Scaffold scope (docs only) | GO | scaffold | zero production impact |
| 2026-10-08 | P1 | Module scope (masterplan split: Attendance=3, Leave=4) | GO | loop (user instruction "implement module 3 … full architecture … start implement") | D3-01; reversible until Module 4 starts |
| 2026-10-08 | P1→P2 | New schema (6 tables, migrations 000025–000030) + 19 REST endpoints + 5 events | GO | user instruction 2026-10-08 ("plan … then start implement with tests and golden tests") | spec frozen; human asked to proceed straight to implementation |

## Progress Journal
| Date | Phase | Did | Sensors | Result | Handoff |
|---|---|---|---|---|---|
| 2026-09-30 | P0 | bundle scaffolded | structure check | GREEN | — |
| 2026-10-08 | P1 | full design bundle written; contracts frozen | doc review vs Modules 0/2 code | GREEN | handoff.md |
| 2026-10-08 | P2 | 6 models + constants; models_test (table names, 8 index contracts, append-only, UUID hooks) RED→GREEN; migrations 000025–000030 | build/vet/gofmt clean; models 100% cov | GREEN | handoff.md |
| 2026-10-09 | P3 | calc (ParseHHMM, ExpectedMinutes, Thresholds, AttendanceDate, OpenSessionStart, ComputeTotals) RED→GREEN incl. goldens G4 (12 cases) + G5 (IST night shift, NY DST); dto (requests/responses, ParsePage G13); validators (code, tz, dates, thresholds) | vet/gofmt clean; calc/dto/validators/models 100% | GREEN | handoff.md |
| 2026-10-09 | P4 | repositories: interfaces (6) + GORM impls; LockEmployee advisory lock; FindOrCreate (on conflict do nothing); CountByStatus single aggregate; dates as ?::date strings | build/vet/gofmt clean | GREEN | handoff.md |
| 2026-10-09 | P5 | ports (EmployeeDirectory, AuditLogger, TxRunner, Clock) + in-memory fakes with tx rollback; ShiftService, AssignmentService, PunchService, QueryService, RegularizationService, OutboxRelay; events envelope; resolveDay cross-midnight tests; split fakes to honor 300-line cap | vet/gofmt clean; services 91.8%; no TODO | GREEN | handoff.md |
| 2026-10-09 | P6 | controllers (bind→VALIDATION_ERROR details, opaque 500, actor from context only), routes (19, RBAC per spec), module.go (Module 2 directory adapter, permission seed, relay, punch rate limit), cmd/main.go wiring + relay shutdown, swagger paths+schemas | go build ./... + vet ./... clean; go test ./internal/... all PASS; controllers/routes 100% | GREEN | handoff.md |
