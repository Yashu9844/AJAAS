# Module 1 — Todo (P1 & P2 DONE; P3 NEXT)

## P0 (done)
- [x] 17-file bundle present, plan.md one CURRENT + one NEXT

## P1 (done — G1-1 design, docs only, no code)
- [x] specification.md — FR-D001..008, FR-T001..007, FR-DG001..004, FR-H001..005, FR-M001..007, FR-O001..004, FR-E001..005 + NFRs + DTOs + validation + errors + edges + out-of-scope
- [x] architecture.md — layers, components, flows, failure paths, caching, persistence, concurrency, WHY
- [x] connections.md — edge table C1..C9 with owner + tests
- [x] golden-tests.md — G1..G10 frozen candidates
- [x] security.md / testing.md / files.md / decisions.md / assumptions.md updated for P1
- [x] contracts frozen (spec §4 DTOs + §2 REST shapes + FR-E005 events); agent.md DESIGN bar + goal/status/handoff/changelog updated

## P2 (done — G1-2 implementation)
- [x] models/ (department, team, designation, mapping, outbox) + models_test
- [x] migrations 000013..000017 up/down pairs
- [x] dto/ (6 files) + validators/ with unit tests
- [x] repositories/ interfaces + 4 tenant-scoped impls
- [x] services/ (department, team, designation, mapping, org_chart, event_consumer) with cycle/depth guards and unit test suites
- [x] controllers/ + routes/ (20 endpoints per frozen shapes) + controller unit suites
- [x] events/events.go (7 produced) + outbox pattern + event consumer integration (FR-E001..E004)
- [x] module.go DI + RegisterModels + RegisterRoutes wired in cmd/main.go with user deactivation converger
- [x] OpenAPI / Swagger documentation updated in backend/api/swagger.yaml for all 20 organization endpoints
- [x] Sensors verified: `go build ./...`, `go vet ./...`, `gofmt -l`, `go test ./internal/...` all clean

## P3 (next — G1-3 integration & frontend)
- [ ] Repo integration (Docker PG), API E2E, security (G1/G4/G10), goldens G1..G10, perf NFR-P001..P004
- [ ] Frontend Organization slice (Department tree, Team views, Org chart visualizer)
- [ ] Handoff to Module 2 (Employee)
