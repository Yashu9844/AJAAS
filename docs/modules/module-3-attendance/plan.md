# Module 3 — Plan (updated: 2026-10-09, owner: loop — P7 DONE, P9 CURRENT)

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
| P7 | G3-7 | Live ring: Docker PG boot, migrations up/down/up, live goldens G1/G6/G7/G9/G11/G12/G13 | P6 | DONE | live G1,G2,G6–G9,G11–G13 PASS; full tests/api suite PASS; migrations up/down/up clean; AutoMigrate↔SQL columns+indexes identical |
| P8 | G3-8 | Frontend attendance slice vs live API | P7 + identity frontend shell | LATER | — |
| P9 | G3-9 | Hardening + DoD close (§24), handoff to Module 4 | P7 | CURRENT | — |

Status vocabulary: DONE / CURRENT (one) / NEXT (one) / LATER / BLOCKED. Promotion = MASTER_PROMPT §24 + handoff.

## Current Phase
- Goal: G3-9 DoD close (MASTER_PROMPT §24) — walk the checklist against evidence (spec FR/AT coverage map, security.md threats → tests, swagger ↔ routes, 300-line cap, no TODOs, docs coherent), update DEPENDENCY-GRAPH, write the Module 4 handoff (re-scope bundle from Projects to Leave per D3-01).
- Why now: P7 proved the module end-to-end on real Postgres; P8 (frontend) is blocked on the identity login shell and stays LATER.
- Scope in: docs only, plus small fixes the checklist exposes.
- Exit criteria: §24 checklist all ticked with evidence links; handoff.md names Module 4 entry point.
- Risks: none material.

## Next Phase
- Goal: Module 4 (Leave) G4-1 design — re-scope docs/modules/module-4-* from Projects to Leave, full spec/architecture/goldens, then implement in phases like Module 3.

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
| 2026-10-09 | P7 | infra up; backend boot (19 routes, seed rows); live goldens G1,G2,G6–G9,G11–G13 RED→GREEN (fixes: test precedence D3-13, login limiter reset D3-15); migrations 30 up / 6 down / 6 up; schema parity diff → model tags (integer, partial + expression indexes) RED→GREEN (D3-14) | go test -tags integration ./tests/api PASS (10 tests); go test ./internal/... PASS; vet/gofmt clean | GREEN | handoff.md |
