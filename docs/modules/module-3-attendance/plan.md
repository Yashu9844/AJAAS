# Module 3 — Plan (updated: 2026-10-09, owner: loop — P4 DONE, P5 CURRENT)

## Phase Map

| Phase | Goal ID | Objective | Depends On | Status | Proof |
|---|---|---|---|---|---|
| P0 | G3-0 | Bundle scaffold + dependency metadata | — | DONE | 2026-09-30 scaffold |
| P1 | G3-1 | Design: spec (FR/AT), architecture, connections, goldens, frozen REST/event/data contracts | Modules 0+2 merged | DONE | specification.md, architecture.md, connections.md, golden-tests.md (2026-10-08) |
| P2 | G3-2 | Models (6) + migrations 000025–000030 + model tests | P1 | DONE | models_test 100% cov; SQL pairs written (live up/down/up verified at P7) |
| P3 | G3-3 | Pure `calc` engine (date attribution, totals, status) + DTOs + validators; goldens G4/G5 | P2 | DONE | goldens G4/G5 + G13 (ParsePage) green; calc/dto/validators 100% cov |
| P4 | G3-4 | Repositories (6, tenant-scoped, tx param, advisory lock) | P2 | DONE | 6 repos + interfaces compile; tenant-scope audit clean (outbox relay cross-tenant by design); SQL verified live at P7 |
| P5 | G3-5 | Services (shift, assignment, punch, query, regularization, outbox relay) + events; goldens G2/G3/G6/G7/G8/G10/G11/G12; services ≥ 90% | P3+P4 | CURRENT | — |
| P6 | G3-6 | Controllers + routes + module.go + main.go wiring + permission seed + swagger (19 paths); G13 | P5 | NEXT | — |
| P7 | G3-7 | Live ring: Docker PG boot, migrations up/down/up, live goldens G1/G6/G7/G9/G11/G12/G13 | P6 | LATER | — |
| P8 | G3-8 | Frontend attendance slice vs live API | P7 + identity frontend shell | LATER | — |
| P9 | G3-9 | Hardening + DoD close (§24), handoff to Module 4 | P7 | LATER | — |

Status vocabulary: DONE / CURRENT (one) / NEXT (one) / LATER / BLOCKED. Promotion = MASTER_PROMPT §24 + handoff.

## Current Phase
- Goal: G3-5 services + events — ports (employeeDirectory, auditLogger, txRunner, clock), event envelope, ShiftService, AssignmentService, PunchService, QueryService, RegularizationService, OutboxRelay; unit goldens G2/G3/G6/G7/G8/G10/G11/G12 with in-memory fakes; services ≥ 90%.
- Why now: all business rules (AT-001..AT-022) live here; HTTP (P6) is a thin adapter.
- Scope in: backend/internal/attendance/{services,events}/**.
- Exit criteria: goldens green; coverage ≥ 90% services; build/vet/gofmt clean; docs updated.
- Risks: fakes diverging from real repos (Module 1 lesson: stub counted only active teams) → fakes implement the exact repo contract semantics and the same goldens run live in P7.

## Next Phase
- Goal: G3-6 controllers + routes + module.go + main.go wiring + permission seed + swagger.

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
