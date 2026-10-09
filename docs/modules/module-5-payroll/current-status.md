# Module 5 — Current Status (updated: 2026-10-09, backend CLOSED — P1–P7, P9 DONE; P8 BLOCKED)

- P1: design frozen — 20 ops, PY-001..PY-015, 7 tables, 4 events, goldens G1–G16, decisions D5-01..D5-09. No code yet.
- P2: calc.Money; models Structure, Component, Assignment, Run, Payslip (+Lines), PayslipLine, OutboxEvent — 100%; migrations 000038–000044 (CHECKs for enums, ranges, period, net ≥ 0) cycle-verified. P3 calc engine landed early: Breakdown, ValidateStructure, PF/ESI/PT/AnnualTax/MonthlyTDS, Compute with net cap — goldens G2–G9 green, 100%.
- P3: dto + validators (CTC range, period not in future, CSV formula-injection guard) — 100%.
- P4: repositories (structure, assignment, run, payslip, outbox) — compile-verified; SQL proven at P7.
- P5: services (Structure, Assignment, Run, Payslip, OutboxRelay) + events; Module 4 UnpaidLeave port. Goldens: September lifecycle (full month, mid-month joiner 15/30, leaver 10/30, LOP 2 → 28/30, PF/PT exact, no TDS under 87A), maker-checker, state machine, recalculation, self-visibility + privacy; warnings (missing employee, overflow, net cap); fault injection. Services 94.4%.
- P6: HTTP edge (20 ops, RBAC contract test), module.go, wired in internal/app (Modules 3–5 as peopleModules), permission seed payroll:read|manage|approve, swagger.
- P7: integration (in-process, real Postgres + Redis): TestPayrollLifecycle — two employees assigned, member's 2 approved unpaid days flow from Module 4 into payable/LOP (G10), maker cannot approve own calculation (G11), finalize before approve rejected (G12), nothing visible before finalize then exactly one own payslip, another's payslip 404 (G13), payout CSV, no salary in audit/events (G14); TestPayrollRBACAndIsolation — member 403, payroll:read reads but cannot calculate, tenant B 404s, foreign JWT 403, pagination (G16), preview. Full suite 61/61. Migrations 44 up / 7 down / 7 up; AutoMigrate vs SQL columns + 19 index definitions identical.

## Definition of Done (§24) — backend, 2026-10-09
- [x] Requirements — FR-ST/AS/RN/PS/EV + PY-001..PY-015 cited in code/tests
- [x] Dependencies — C1–C8 (Module 0 auth/RBAC/audit, Module 2 employees, Module 4 UnpaidLeave)
- [x] Implementation — 20 ops, 7 tables, 4 events, relay, statutory engine
- [x] Code quality — AST check clean (func/file/depth/params/panic/TODO)
- [x] Unit — calc/dto/events/models/validators/routes 100%, controllers 99.4%, services 94.4%
- [x] Integration — 61/61 (2 payroll + Modules 0–4)
- [x] Contract — 20 routes ↔ 20 swagger ops (router contract test + YAML ref check)
- [x] Golden — G1–G16 (calc, services, integration)
- [x] Security — PS-T1..T8 evidence in security.md
- [x] Dependent tests — Module 4 (UnpaidLeave port) and full suite green
- [x] No unrelated changes — outside touches: Module 4 read port + accessor (D4-13), internal/app wiring, swagger, DEPENDENCY-GRAPH, bundle moves (D5-01)
- [x] Docs · [x] Diff reviewed · [x] No secrets
- Not run / not met: `go test -race` (no cgo); NFR-P001 (500-employee calculate < 10 s) not load-tested; mid-month CTC split not modelled (D5-10). Follow-up for Module 0–2 owner: `internal/app.Build` is 77 lines (> 50, pre-existing).
