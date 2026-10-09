# Module 5 — Plan (updated: 2026-10-09, owner: loop — P4 DONE, P5 CURRENT)

## Phase Map

| Phase | Goal ID | Objective | Depends On | Status | Proof |
|---|---|---|---|---|---|
| P0 | G5-0 | Bundle scaffold | — | DONE | as module-9-payroll |
| P1 | G5-1 | Spec, architecture, connections, goldens frozen | Modules 0–4 on main | DONE | specification.md 2026-10-09 |
| P2 | G5-2 | Models (7) + migrations 000038–000044 + model tests | P1 | DONE | 7 models 100% (index/where/expression + column types); migrations 44 up / 7 down / 7 up clean |
| P3 | G5-3 | calc (Money, Breakdown, Prorate, PF/ESI/PT/TDS, Assemble) goldens G2–G9; DTOs; validators | P2 | DONE | goldens G2–G9 + full payslip; calc/dto/validators 100% |
| P4 | G5-4 | Repositories | P2 | DONE | 5 repos compile; tenant-scoped; run FOR UPDATE; DISTINCT ON eligibility; SQL proven at P7 |
| P5 | G5-5 | Services + events + relay + Module 4 UnpaidLeave port; goldens G10–G12, G15 | P3+P4 | CURRENT | — |
| P6 | G5-6 | Controllers, routes, module, app wiring, seed, swagger | P5 | NEXT | — |
| P7 | G5-7 | Integration goldens + migration cycle | P6 | LATER | — |
| P8 | G5-8 | Frontend | identity shell | BLOCKED | — |
| P9 | G5-9 | DoD close + handoff | P7 | LATER | — |

## Current Phase
- Goal: G5-5 services (structure, assignment, run, payslip), events, relay, Module 4 UnpaidLeave port; goldens G10–G12, G15.

## Next Phase
- Goal: G5-6 HTTP edge + app wiring.

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
