# Module 5 — Files (status per phase)

| Path | Purpose | Status |
|---|---|---|
| docs/modules/module-5-payroll/*.md | bundle | DONE (P1) |
| backend/internal/payroll/models/* + test; migrations 000038–000044 | schema | P2 |
| backend/internal/payroll/{calc,dto,validators}/* + goldens | money, breakdown, statutory, DTOs | P3 |
| backend/internal/payroll/repositories/* | data access | P4 |
| backend/internal/leave/services/unpaid_leave.go (+test) | Module 4 read port (D5-03) | P5 |
| backend/internal/payroll/{services,events}/* + goldens | business logic | P5 |
| backend/internal/payroll/{controllers,routes}/*, module.go; internal/app wiring; swagger | HTTP + DI | P6 |
| backend/tests/integration/payroll_test.go | integration goldens | P7 |
