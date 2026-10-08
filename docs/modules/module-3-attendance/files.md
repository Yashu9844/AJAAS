# Module 3 — Files (target map frozen at P1; status column updated per phase)

| Path | Purpose | Status |
|---|---|---|
| docs/modules/module-3-attendance/*.md | 17-file bundle | P1 DONE |
| backend/internal/attendance/models/{shift,shift_assignment,attendance_record,attendance_punch,regularization,outbox}.go + models_test.go | GORM models | P2 |
| backend/migrations/000025_create_shifts … 000030_create_attendance_events_outbox (.up/.down) | SQL schema | P2 |
| backend/internal/attendance/calc/{time,totals}.go + calc_test.go + calc_golden_test.go | pure time engine | P3 |
| backend/internal/attendance/dto/*.go, validators/validators.go (+tests) | request/response DTOs, field rules | P3 |
| backend/internal/attendance/repositories/{interfaces,shift,assignment,record,punch,regularization,outbox}_repository.go | tenant-scoped data access | P4 |
| backend/internal/attendance/services/{ports,shift,assignment,punch,query,regularization,outbox}_service.go + tests/goldens | business logic | P5 |
| backend/internal/attendance/events/events.go (+test) | event types + envelope | P5 |
| backend/internal/attendance/controllers/*.go (+tests), routes/routes.go, module.go | HTTP edge + DI | P6 |
| backend/cmd/main.go (wire module, seed, relay) · backend/api/swagger.yaml (19 paths) | bootstrap/contract | P6 |
| backend/tests/api/attendance_*_test.go (`integration` tag) | live goldens | P7 |
| frontend/src/modules/attendance/** | UI slice | P8 (LATER) |
