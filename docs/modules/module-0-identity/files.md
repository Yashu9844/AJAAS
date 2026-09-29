# Module 0 — Files (operational map; spec files live alongside)

Docs (spec base, existing): README, requirements, api-contracts, business-rules, database-schema.dbml, events, security, edge-cases, acceptance-criteria, implementation-plan/roadmap/checklist, coding-guidelines, testing-strategy, sequence-diagrams.mmd.
Docs (operational, new): agent.md (behavior law), current-goal.md (G0-1), plan.md (P1 CURRENT), current-status.md, todo.md (mirror), handoff.md, assumptions.md, decisions.md, changelog.md, files.md (this map).

Backend (real): internal/shared/{config, database/base+database, errors, logger, constants} (+3 tests) — DONE. internal/identity/models/tenant.go — BROKEN (G0-1). internal/identity/{controllers, dto, events, middleware, repositories, routes, services, validators}/ — MISSING (G0-4..G0-12). internal/shared/{cache, queue, middleware, utils}/ — MISSING. cmd/main.go, migrations/, api/swagger.yaml, tests/{api,security,performance}/ — MISSING (G0-2/G0-3/G0-14).
Frontend: src/modules/identity/{api, components, pages, hooks, services, types}/ — MISSING (G0-13).
