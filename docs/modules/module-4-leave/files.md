# Module 4 — Files (target map frozen at P1; status updated per phase)

| Path | Purpose | Status |
|---|---|---|
| docs/modules/module-4-leave/*.md | 17-file bundle | DONE (P1) |
| backend/internal/leave/models/{models,models_test}.go | GORM models (6) | DONE (P2) |
| backend/migrations/000031…000036 (.up/.down) | SQL schema | DONE (P2) |
| backend/internal/leave/calc/{days,count,accrual}.go + days_test, calc_golden_test | days, counting, accrual, carry-forward | DONE (P3) |
| backend/internal/leave/dto/{requests,responses}.go, validators/validators.go (+tests) | DTOs, field rules | DONE (P3) |
| backend/internal/leave/repositories/{interfaces,type,balance,request}_repository.go | tenant-scoped data access (holiday in type_, ledger in balance_, outbox in request_) | DONE (P4) |
| backend/internal/leave/{services,events}/*.go + tests/goldens | business logic, events, relay | DONE (P5) |
| backend/internal/attendance/services/leave_sync.go (+test), record source `leave`, module LeaveSync() | Module 3 port (D4-08) | DONE (P5/P6) |
| backend/internal/leave/{controllers,routes}/*.go + tests, module.go | HTTP edge + DI | DONE (P6) |
| backend/cmd/main.go · backend/api/swagger.yaml | wiring, contract | DONE (P6) |
| backend/tests/api/leave_{flow,rules}_test.go (`integration`) | live goldens | DONE (P7) |
