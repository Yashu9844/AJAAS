# Module 3 — Current Goal

Goal: G3-2 Models + migrations — six GORM models (shifts, shift_assignments, attendance_records, attendance_punches, attendance_regularizations, attendance_events_outbox) matching specification.md §8, six up/down SQL pairs 000025–000030, model tests.
Why: frozen schema unblocks calc/DTO (P3) and repositories (P4).
Scope: backend/internal/attendance/models/**, backend/migrations/000025–000030.
Non-goals: services, controllers, API, frontend.
Success Criteria:
- [ ] `go build ./...`, `go vet ./...`, `gofmt -l` clean
- [ ] `go test ./internal/attendance/models/...` green (schema parse, table names, unique/index names)
- [ ] SQL pairs present; down files drop in reverse order
- [ ] plan.md / current-status.md / todo.md / handoff.md / changelog.md updated
