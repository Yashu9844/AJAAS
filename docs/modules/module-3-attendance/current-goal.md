# Module 3 — Current Goal

Goal: G3-6 HTTP edge + wiring — controllers (punch, attendance, regularization, shift) with field-level VALIDATION_ERROR and pagination clamp; routes behind Module 0 TenantResolver + Authenticate + RequirePermission(attendance:read|manage|approve) and a punch rate limit; module.go DI (Module 2 directory adapter, Module 0 audit adapter, permission seed, outbox relay); cmd/main.go wiring; swagger 19 paths.
Why: exposes the frozen spec §5 contract over the finished services.
Scope: backend/internal/attendance/{controllers,routes}/**, backend/internal/attendance/module.go, attendance block in backend/cmd/main.go, attendance paths in backend/api/swagger.yaml.
Non-goals: business-rule changes, live DB runs (P7), frontend.
Success Criteria:
- [ ] controller tests: status/envelope mapping, binding → VALIDATION_ERROR details, G13 pagination clamp, self endpoints ignore client ids
- [ ] router test proves 19 routes resolve without conflicts
- [ ] `go build ./...` (incl. cmd) + vet + gofmt clean; controllers ≥ 80%
- [ ] plan/status/todo/handoff/changelog/files updated
