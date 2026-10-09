# Module 5 — Plan (updated: 2026-10-09, owner: loop — backend CLOSED; P8 BLOCKED)

## Phase Map

| Phase | Goal ID | Objective | Depends On | Status | Proof |
|---|---|---|---|---|---|
| P0 | G5-0 | Bundle scaffold | — | DONE | as module-9-payroll |
| P1 | G5-1 | Spec, architecture, connections, goldens frozen | Modules 0–4 on main | DONE | specification.md 2026-10-09 |
| P2 | G5-2 | Models (7) + migrations 000038–000044 + model tests | P1 | DONE | 7 models 100% (index/where/expression + column types); migrations 44 up / 7 down / 7 up clean |
| P3 | G5-3 | calc (Money, Breakdown, Prorate, PF/ESI/PT/TDS, Assemble) goldens G2–G9; DTOs; validators | P2 | DONE | goldens G2–G9 + full payslip; calc/dto/validators 100% |
| P4 | G5-4 | Repositories | P2 | DONE | 5 repos compile; tenant-scoped; run FOR UPDATE; DISTINCT ON eligibility; SQL proven at P7 |
| P5 | G5-5 | Services + events + relay + Module 4 UnpaidLeave port; goldens G10–G12, G15 | P3+P4 | DONE | lifecycle goldens (proration, window, LOP G10, statutory), G11, G12, G15, self-visibility/privacy; services 94.4%; Module 4 UnpaidLeave port tested |
| P6 | G5-6 | Controllers, routes, module, app wiring, seed, swagger | P5 | DONE | controllers 99.4%, routes 100% (20-route RBAC contract), module + app wiring (peopleModules) + seed + relay, swagger 16 paths/20 ops validated |
| P7 | G5-7 | Integration goldens + migration cycle | P6 | DONE | integration: lifecycle (real Module 4 LOP G10, maker-checker G11, G12, self visibility G13, privacy G14, CSV) + RBAC/isolation/G16; full suite 61/61; cycle 44/7/7; AutoMigrate↔SQL identical (19 indexes) |
| P8 | G5-8 | Frontend | identity shell | BLOCKED | — |
| P9 | G5-9 | DoD close + handoff | P7 | DONE | DoD ticked with evidence; AST clean; 20/20 routes ↔ swagger |

## Current Phase
- None: Module 5 backend closed at P9 (2026-10-09).

## Next Phase
- G5-8 frontend payroll slice (BLOCKED until a frontend shell for these modules exists).

## Gate Log
| Date | Phase | Gate | Decision | By | Reason |
|---|---|---|---|---|---|
| 2026-10-09 | P1 | Module 5 = Payroll (D5-01), new schema + 20 ops + Module 4 read port | GO | user: "start working on module 5 and module 6" after masterplan numbering proposal | masterplan adopted for 3/4 |

## Progress Journal
| Date | Phase | Did | Sensors | Result | Handoff |
|---|---|---|---|---|---|
| 2026-10-09 | P1 | bundle moved from module-9-payroll; full design | review vs Modules 0–4 code | GREEN | handoff.md |
| 2026-10-09 | P2 | calc.Money (paise codec) + 7 models with parity contracts RED→GREEN; migrations 000038–000044 | models/calc 100%; cycle 44 up / 7 down / 7 up | GREEN | handoff.md |
| 2026-10-09 | P3 | dto (requests\/responses, ParsePage) + validators (date, CTC, period, CSVSafe PS-T8) | all payroll packages 100% | GREEN | handoff.md |
| 2026-10-09 | P4 | repositories: structure (+components), assignment (advisory lock, current, DISTINCT ON eligibility), run (FOR UPDATE), payslip (+lines, finalized-only self list), outbox | build/vet clean | GREEN | handoff.md |
| 2026-10-09 | P5 | Module 4 UnpaidLeave port (approved unpaid, clipped, half day, holidays) RED→GREEN; payroll ports\/errors\/helpers\/mappers; Structure, Assignment, Run (state machine + maker-checker), calculate (window, LOP, Compute, warnings), Payslip (self finalized-only, CSV), events, relay; fakes + goldens + fault injection | vet clean; payroll services 94.4%, leave services 96.5% | GREEN | handoff.md |
| 2026-10-09 | P6 | controllers (setup, run) + routes (20 ops) + module.go + internal\/app peopleModules wiring (Module 4 UnpaidLeave) + swagger | go vet .\/... clean; go test .\/internal\/... PASS | GREEN | handoff.md |
| 2026-10-09 | P7+P9 | tests\/integration\/payroll_test.go (2 tests); parity diff; DoD walk | integration 61\/61 PASS; AST clean | GREEN | handoff.md |
