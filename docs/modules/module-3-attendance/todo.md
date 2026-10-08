# Module 3 — Todo (P1–P7 DONE; P9 CURRENT)

## P1 (done — G3-1)
- [x] specification.md frozen (FR + AT rules + API + data + errors + edges + out-of-scope)
- [x] architecture.md, connections.md, security.md, golden-tests.md, testing.md, decisions.md, assumptions.md, files.md

## P2 — G3-2 Models + migrations
- [x] T3-01 models: shift, shift_assignment, attendance_record, attendance_punch, regularization, outbox (+ constants for statuses/types)
- [x] T3-02 models_test: schema parse, table names, index names
- [x] T3-03 migrations 000025..000030 up/down (live verification in T3-18)

## P3 — G3-3 calc + DTOs + validators
- [x] T3-04 calc: ParseHHMM/FormatHHMM, ExpectedMinutes, AttendanceDate (AT-005), ComputeTotals (AT-006..AT-010)
- [x] T3-05 goldens G4 (totals table) + G5 (night shift, tz)
- [x] T3-06 DTOs (requests/responses/pagination clamp, G13 unit) + validators (code, tz, dates, thresholds)

## P4 — G3-4 Repositories
- [x] T3-07 interfaces + 6 GORM impls (tenant_id on every query, tx param, LockEmployee advisory lock, FindOrCreate record)

## P5 — G3-5 Services + events
- [x] T3-08 ports (employeeDirectory, auditLogger, txRunner, clock) + events envelope
- [x] T3-09 ShiftService + AssignmentService (G10)
- [x] T3-10 PunchService (G2, G3, G11, G12)
- [x] T3-11 AttendanceQueryService (today, me, list, detail, summary)
- [x] T3-12 RegularizationService (G6, G7, G8)
- [x] T3-13 OutboxRelay
- [x] T3-14 coverage ≥ 90% services (91.8%)

## P6 — G3-6 HTTP + wiring
- [x] T3-15 controllers + error mapping + pagination clamp (G13) + tests
- [x] T3-16 routes + module.go + permission seed + cmd/main.go (AutoMigrate, relay)
- [x] T3-17 swagger.yaml 19 operations (16 paths) + 24 schemas, validated

## P7 — G3-7 Live ring
- [x] T3-18 docker infra up, backend boot (seed verified), migrations 30 up → 6 down → 6 up on scratch DB, AutoMigrate↔SQL parity identical (D3-14)
- [x] T3-19 live goldens G1, G2, G6, G7, G8, G9, G11, G12, G13 PASS (tests/api/attendance_*_test.go, `-tags integration`)

## P8/P9
- [ ] T3-20 frontend slice (blocked on identity shell)
- [ ] T3-21 DoD close + handoff to Module 4
