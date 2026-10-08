# Module 4 — Plan (updated: 2026-10-09, owner: loop — P3 DONE, P4 CURRENT)

## Phase Map

| Phase | Goal ID | Objective | Depends On | Status | Proof |
|---|---|---|---|---|---|
| P0 | G4-0 | Bundle scaffold | — | DONE | 2026-09-30 (as Projects) |
| P1 | G4-1 | Re-scope to Leave; spec, architecture, connections, goldens frozen | Module 3 closed | DONE | specification.md et al. 2026-10-09 |
| P2 | G4-2 | Models (6) + migrations 000031–000036 + model tests (incl. parity contracts) | P1 | DONE | models 100% (index/where/expression + column-type contracts), calc.Days codec 100%; migrations 36 up / 6 down / 6 up clean on scratch DB |
| P3 | G4-3 | calc (Days, CountDays, Accrued, CarryForward) goldens G2–G4; DTOs; validators | P2 | DONE | goldens G2 (11 cases), G3 (11), G4 (4), G15; calc/dto/validators 100% |
| P4 | G4-4 | Repositories (6) + advisory lock | P2 | CURRENT | — |
| P5 | G4-5 | Services + events + relay + Module 3 LeaveSync; goldens G5–G8, G10, G11, G13; services ≥ 90% | P3+P4 | NEXT | — |
| P6 | G4-6 | Controllers, routes, module.go, main.go, seed, swagger | P5 | LATER | — |
| P7 | G4-7 | Live ring: goldens, migration cycle, parity diff | P6 | LATER | — |
| P8 | G4-8 | Frontend leave slice | identity shell | BLOCKED | no identity login shell |
| P9 | G4-9 | DoD close + handoff | P7 | LATER | — |

Status vocabulary: DONE / CURRENT (one) / NEXT (one) / LATER / BLOCKED.

## Current Phase
- Goal: G4-4 repositories — type, holiday, balance, request, ledger, outbox; tenant-scoped; tx param; LockEmployee advisory lock (`leave:` namespace, LV-014); FindOrCreate balance; overlap query; list filters; dates as ?::date strings (D3 lesson).
- Exit criteria: compile + vet clean; SQL behavior proven at P7 live.

## Next Phase
- Goal: G4-5 services + events + relay + Module 3 LeaveSync.

## Gate Log
| Date | Phase | Gate | Decision | By | Reason |
|---|---|---|---|---|---|
| 2026-10-09 | P1 | Re-scope Module 4 Projects → Leave (D4-01) | GO | user instruction "please complete module 4 as well" after Module 3 handoff naming Leave | masterplan numbering adopted in D3-01 |
| 2026-10-09 | P1→P2 | New schema (6 tables) + 20 REST ops + 5 events + Module 3 port | GO | same instruction | spec frozen |

## Progress Journal
| Date | Phase | Did | Sensors | Result | Handoff |
|---|---|---|---|---|---|
| 2026-10-09 | P1 | bundle renamed + full design (spec, architecture, connections, security, goldens, decisions) | doc review vs Modules 0/2/3 code | GREEN | handoff.md |
| 2026-10-09 | P2 | calc.Days (hundredths, SQL Value/Scan, JSON) RED→GREEN; 6 models (types, holidays, balances, requests, ledger, outbox) with parity contracts RED→GREEN; migrations 000031–000036 | vet/gofmt clean; models + calc 100%; live migration cycle 36 up / 6 down / 6 up | GREEN | handoff.md |
| 2026-10-09 | P3 | calc CountDays (weekend, holidays, sandwich, half day) + Accrued (cumulative months formula) + CarryForward + date helpers RED→GREEN (goldens G2–G4); dto requests/responses + ParsePage (G15); validators (code, amounts, dates, year, gender, notice) | vet/gofmt clean; calc/dto/validators/models 100% | GREEN | handoff.md |
