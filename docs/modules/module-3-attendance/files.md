# Module 3 — Files (target map frozen at P1; status column updated per phase)

| Path | Purpose | Status |
|---|---|---|
| docs/modules/module-3-attendance/*.md | 17-file bundle | P1 DONE |
| backend/internal/attendance/models/{constants,shift,attendance}.go + models_test.go | GORM models | DONE (P2) |
| backend/migrations/000025_create_shifts … 000030_create_attendance_events_outbox (.up/.down) | SQL schema | DONE (P2), live check P7 |
| backend/internal/attendance/calc/{time,totals}.go + calc_test.go + calc_golden_test.go | pure time engine | DONE (P3) |
| backend/internal/attendance/dto/dto.go, validators/validators.go (+tests) | request/response DTOs, field rules | DONE (P3) |
| backend/internal/attendance/repositories/{interfaces,shift,record,punch}_repository.go | tenant-scoped data access (assignment in shift_, regularization+outbox in punch_ file) | DONE (P4) |
| backend/internal/attendance/services/{ports,errors,helpers,mappers,shift_service,assignment_service,punch_service,query_service,regularization_service,regularization_review,outbox_relay}.go + fakes/harness + tests/goldens | business logic | DONE (P5) |
| backend/internal/attendance/events/events.go (+test) | event types + envelope | DONE (P5) |
| backend/internal/attendance/controllers/*.go (+tests), routes/routes.go (+test), module.go | HTTP edge + DI | DONE (P6) |
| backend/cmd/main.go (wire module, seed, relay) · backend/api/swagger.yaml (19 operations) | bootstrap/contract | DONE (P6) |
| backend/tests/api/attendance_{helpers,flow,rules}_test.go (`integration` tag) | live goldens | DONE (P7) |
| frontend/src/modules/attendance/** | UI slice | P8 (LATER) |
