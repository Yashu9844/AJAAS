# Module 5 — Files (status per phase)

| Path | Purpose | Status |
|---|---|---|
| docs/modules/module-5-payroll/*.md | bundle | DONE (P1) |
| backend/internal/payroll/models/* + test; migrations 000038–000044 | schema | DONE (P2) |
| backend/internal/payroll/{calc,dto,validators}/* + goldens | money, breakdown, statutory, DTOs | DONE (P3) |
| backend/internal/payroll/repositories/* | data access | DONE (P4) |
| backend/internal/leave/services/unpaid_leave.go (+test) | Module 4 read port (D5-03) | DONE (P5) |
| backend/internal/payroll/{services,events}/* + goldens | business logic | DONE (P5) |
| backend/internal/payroll/{controllers,routes}/*, module.go; internal/app wiring; swagger | HTTP + DI | DONE (P6) |
| backend/tests/integration/payroll_test.go | integration goldens | DONE (P7) |
