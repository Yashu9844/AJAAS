# Module 4 — Current Status (updated: 2026-10-09, backend CLOSED — P1–P7, P9 DONE; P8 BLOCKED)

- P1: re-scoped from Projects to Leave (D4-01); spec frozen (20 ops, LV-001..017, 6 tables, 5 events), architecture, connections C1–C9, security LS-T1..T8, goldens G1–G15.
- P2: `calc.Days` (integer hundredths; NUMERIC(7,2) via Value/Scan; JSON number) 100%; models LeaveType, Holiday, Balance (Available()), Request, LedgerEntry, OutboxEvent with index/where/expression and column-type contracts, 100%; migrations 000032–000037 (CHECK constraints for enums, ranges, half-day single date) — 36 up / 6 down / 6 up verified on scratch Postgres.
- P3: `calc` CountDays/Accrued/CarryForward/DateOf/IsWeekend — goldens G2 (day counting incl. sandwich, holidays, half day), G3 (accrual: annual proration, monthly, December remainder, joining after asOf), G4 (carry-forward) green; `dto` requests/responses + ParsePage (G15); `validators` code/amount/adjustment/date/year/gender/notice (LV-005). All four packages 100%.
- P4: `repositories` — Type, Holiday, Balance (advisory lock `leave:<tenant>:<employee>`, FindOrCreate reporting creation), Request (overlap on pending/approved), Ledger (append-only: Create + List only), Outbox; tenant-scoped; compile-verified, SQL proven at P7.
- P5: services (Type, Holiday, Balance, Request, OutboxRelay) + events; Module 3 LeaveSync port (D4-08/D3-17). Unit goldens G5–G8, G10, G11, G13 green; services 96.3%. Accrual events are delivered by the relay (not published inline).
- P6: controllers (100%), routes (20 ops, RBAC contract test, 100%), module.go (Ports: Module 2 EmployeeService, Module 0 AuditService, Module 3 LeaveSync; permission seed leave:read|manage|approve; relay), cmd/main.go wiring (AutoMigrate, seed, routes, relay start/stop), swagger (16 paths / 20 operations, refs validated).
- P7: live ring on Docker Postgres — tests/api/leave_flow_test.go (lifecycle: preview, apply G5, approve G8 → 2 Module 3 on_leave records, cancel future G13 → records removed, adjust, ledger invariant G12 via SQL, audit G11 + outbox privacy; overlap G7 + half days; pagination G15) and leave_rules_test.go (RBAC G9, self-approval G10, isolation G1, concurrency G14: 5 parallel applies → 2×201 + 3×409, available 1). All PASS; full tests/api suite 16/16 PASS. Migrations 36 up / 6 down / 6 up clean; AutoMigrate vs SQL: columns and all 20 index definitions identical. Perf note: this host showed ~70 ms per query under Docker memory pressure (login 2–4 s), so p95 budgets (NFR-P001/P002) were not meaningfully measurable here — apply 881 ms, preview 606 ms observed; re-measure on a normal host.

## Definition of Done (MASTER_PROMPT §24) — backend, 2026-10-09
- [x] Requirements understood — FR-LT/HD/BL/LR/EV + LV-001..LV-017 cited in code and tests
- [x] Dependencies checked — connections C1–C9 (Module 0 auth/RBAC/audit, Module 2 employees, Module 3 LeaveSync)
- [x] Implementation complete — 20 operations, 6 tables, 5 events, relay, Module 3 port
- [x] Code quality — AST check: func ≤ 50, file ≤ 300, depth ≤ 4, params ≤ 4 (D3-16), no panic/TODO/nolint
- [x] Unit pass — leave calc/controllers/dto/events/models/routes/validators 100%, services 96.3%; attendance services 91.9%
- [x] Integration pass — `go test -tags integration ./tests/api/` 16/16 (5 leave + 6 attendance + 5 org)
- [x] Contract pass — live routes ↔ swagger 20/20; router contract test
- [x] Golden pass — G1–G15 (unit and/or live; see golden-tests.md)
- [x] Security verified — security.md threat → evidence LS-T1..T8
- [x] Dependent tests pass — Module 3 unit + live, Module 1 live green
- [x] No unrelated modules modified — outside touches are integration points: Module 3 LeaveSync + record Delete + source `leave` (D4-08/D3-17), cmd/main.go, api/swagger.yaml, DEPENDENCY-GRAPH.md
- [x] Docs updated · [x] Diff reviewed · [x] No secrets (scan clean)
- Not met / not run: NFR-P001/P002 p95 not verifiable on this host (D4-11); `go test -race` not run (no cgo) — compensated by live G14 concurrency test.
