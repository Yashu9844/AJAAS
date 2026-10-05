# Module 1 — Files (P1: frozen contract map; code lands at P2)

Docs: README, agent, current-goal (G1-1), plan (P1 CURRENT), current-status, todo, specification (FROZEN), architecture (FROZEN), connections (FROZEN), security, testing, golden-tests (FROZEN candidates), assumptions, decisions, changelog, handoff, files (this map).

Backend (P2 creates — exact files):
- `backend/internal/organization/models/` — department.go, team.go, designation.go, mapping.go, models_test.go
- `backend/migrations/` — 000013_create_departments, 000014_create_teams, 000015_create_designations, 000016_create_mappings (+ org_events_outbox), each .up.sql/.down.sql
- `backend/internal/organization/dto/` — department_dto.go, team_dto.go, designation_dto.go, mapping_dto.go, org_chart_dto.go, pagination reuse from identity
- `backend/internal/organization/repositories/` — interfaces.go + department/team/designation/mapping_repository.go
- `backend/internal/organization/services/` — department/team/designation/mapping/org_chart/event_consumer services + *_test.go
- `backend/internal/organization/controllers/` — department/team/designation/mapping/org_chart_controller.go + *_test.go + response reuse
- `backend/internal/organization/routes/routes.go`, `module.go` (DI + RegisterModels + RegisterRoutes), `events/events.go` + consumer, `validators/validators.go`
- `backend/api/swagger.yaml` — append org paths (P2) / verify (P3)
- `backend/tests/api/org_*.go`, `backend/tests/security/org_*.go`, `backend/tests/performance/org_chart.js` (P3)

Frontend (P3 slice): `frontend/src/modules/organization/{api,components,pages,hooks,services,types}/` — dept/team tree views, mapping UI, chart page vs live API.

Shared (owned centrally, do not edit casually): backend/internal/shared/ (config, database, errors, logger, constants, cache, queue, middleware, utils). New permissions `organization:read/update` seeded via Module 0 permission seed (cross-module record required at P2).
