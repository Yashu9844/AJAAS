# Module 3 — Plan (updated: 2026-10-08, owner: loop — P1 DONE, P2 CURRENT)

## Phase Map

| Phase | Goal ID | Objective | Depends On | Status | Proof |
|---|---|---|---|---|---|
| P0 | G3-0 | Bundle scaffold + dependency metadata | — | DONE | 2026-09-30 scaffold |
| P1 | G3-1 | Design: spec (FR/AT), architecture, connections, goldens, frozen REST/event/data contracts | Modules 0+2 merged | DONE | specification.md, architecture.md, connections.md, golden-tests.md (2026-10-08) |
| P2 | G3-2 | Models (6) + migrations 000025–000030 + model tests | P1 | CURRENT | — |
| P3 | G3-3 | Pure `calc` engine (date attribution, totals, status) + DTOs + validators; goldens G4/G5 | P2 | NEXT | — |
| P4 | G3-4 | Repositories (6, tenant-scoped, tx param, advisory lock) | P2 | LATER | — |
| P5 | G3-5 | Services (shift, assignment, punch, query, regularization, outbox relay) + events; goldens G2/G3/G6/G7/G8/G10/G11/G12; services ≥ 90% | P3+P4 | LATER | — |
| P6 | G3-6 | Controllers + routes + module.go + main.go wiring + permission seed + swagger (19 paths); G13 | P5 | LATER | — |
| P7 | G3-7 | Live ring: Docker PG boot, migrations up/down/up, live goldens G1/G6/G7/G9/G11/G12/G13 | P6 | LATER | — |
| P8 | G3-8 | Frontend attendance slice vs live API | P7 + identity frontend shell | LATER | — |
| P9 | G3-9 | Hardening + DoD close (§24), handoff to Module 4 | P7 | LATER | — |

Status vocabulary: DONE / CURRENT (one) / NEXT (one) / LATER / BLOCKED. Promotion = MASTER_PROMPT §24 + handoff.

## Current Phase
- Goal: G3-2 Models + migrations — six GORM models matching specification.md §8 exactly, six up/down SQL pairs producing the same tables (+ partial/lower indexes), model parse tests.
- Why now: every later layer depends on the frozen schema.
- Scope in: backend/internal/attendance/models/**, backend/migrations/000025–000030.
- Scope out: services, API.
- Exit criteria: build/vet/gofmt clean; models tests green; plan/status/todo/handoff updated.
- Risks: GORM tag vs SQL drift (mitigate: P7 compares live schema); partial indexes only in SQL (service pre-checks cover AutoMigrate dev DB).

## Next Phase
- Goal: G3-3 calc engine + DTOs + validators (goldens G4/G5).

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
