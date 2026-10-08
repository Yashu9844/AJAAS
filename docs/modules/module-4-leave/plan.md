# Module 4 — Plan (updated: 2026-10-09, owner: loop — P5 DONE, P6 CURRENT)

## Phase Map

| Phase | Goal ID | Objective | Depends On | Status | Proof |
|---|---|---|---|---|---|
| P0 | G4-0 | Bundle scaffold | — | DONE | 2026-09-30 (as Projects) |
| P1 | G4-1 | Re-scope to Leave; spec, architecture, connections, goldens frozen | Module 3 closed | DONE | specification.md et al. 2026-10-09 |
| P2 | G4-2 | Models (6) + migrations 000031–000036 + model tests (incl. parity contracts) | P1 | DONE | models 100% (index/where/expression + column-type contracts), calc.Days codec 100%; migrations 36 up / 6 down / 6 up clean on scratch DB |
| P3 | G4-3 | calc (Days, CountDays, Accrued, CarryForward) goldens G2–G4; DTOs; validators | P2 | DONE | goldens G2 (11 cases), G3 (11), G4 (4), G15; calc/dto/validators 100% |
| P4 | G4-4 | Repositories (6) + advisory lock | P2 | DONE | 6 repos + interfaces compile; tenant-scoped (outbox relay cross-tenant by design); lock namespaced `leave:`; SQL proven at P7 |
| P5 | G4-5 | Services + events + relay + Module 3 LeaveSync; goldens G5–G8, G10, G11, G13; services ≥ 90% | P3+P4 | DONE | goldens G5,G6,G7,G8,G10,G11,G13 green on fakes; services 96.3%; events 100%; Module 3 LeaveSync tested (M3 services 91.9%) |
| P6 | G4-6 | Controllers, routes, module.go, main.go, seed, swagger | P5 | CURRENT | — |
| P7 | G4-7 | Live ring: goldens, migration cycle, parity diff | P6 | NEXT | — |
| P8 | G4-8 | Frontend leave slice | identity shell | BLOCKED | no identity login shell |
| P9 | G4-9 | DoD close + handoff | P7 | LATER | — |

Status vocabulary: DONE / CURRENT (one) / NEXT (one) / LATER / BLOCKED.

## Current Phase
- Goal: G4-5 services — Module 3 LeaveSync port (D4-08), ports (EmployeeDirectory, AuditLogger, AttendanceSync, TxRunner, Clock), TypeService, HolidayService, BalanceService (lazy ensure: carry-forward + accrual, adjust, views, ledger), RequestService (preview, apply, approve, reject, cancel, lists), events envelope, OutboxRelay; goldens G5–G8, G10, G11, G13 on fakes; services ≥ 90%.
- Exit criteria: unit goldens green; services ≥ 90%; Module 3 tests still green.

## Next Phase
- Goal: G4-6 HTTP edge + wiring + swagger.

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
| 2026-10-09 | P4 | repositories: interfaces (6) + GORM impls; generic first/paged helpers; LockEmployee (leave: namespace); balance FindOrCreate with created flag (on conflict do nothing); overlap query (pending/approved, ?::date); outbox relay methods | build/vet/gofmt clean | GREEN | handoff.md |
| 2026-10-09 | P5 | Module 3 LeaveSync (MarkLeave/ClearLeave + record Delete + source leave; RED caught early-return bug); leave ports/errors/helpers; ledger move + lazy ensure (carry-forward, accrual); Type/Holiday/Balance/Request services; events; outbox relay; fakes with tx rollback; goldens + rules + fault injection | vet clean; leave services 96.3%, events 100%; attendance services 91.9% | GREEN | handoff.md |
