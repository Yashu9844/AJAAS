# Module 2 — Todo (P1 & P2 DONE; P3 NEXT)

## P0 & P1 (done)
- [x] 17-file bundle scaffolded
- [x] specification.md (FR-EP, FR-ED, FR-EC, FR-ES, FR-DOC, FR-TL, FR-EV)
- [x] architecture.md, connections.md, golden-tests.md, security.md, testing.md, files.md, decisions.md, assumptions.md

## P2 (done — G2-2 Implementation)
- [x] models/ (Profile, EmploymentDetail, Contact, Statutory, Document, Timeline, Outbox) + models_test
- [x] migrations (000018..000024) up/down pairs
- [x] dto/ + custom validators with unit tests
- [x] repositories/ interfaces + GORM tenant-scoped implementations
- [x] services/ (Profile, Employment, Statutory, Document, Timeline, EventConsumer) with unit test suites
- [x] controllers/ + routes/ (REST endpoints with RBAC & AuditLog)
- [x] events/ (domain events + outbox pattern)
- [x] module.go DI + wiring in cmd/main.go
- [x] OpenAPI / Swagger documentation in backend/api/swagger.yaml
- [x] Sensor verification: `go build`, `go vet`, `gofmt`, `go test ./internal/employee/...` all clean

## P3 (next — G2-3 Integration & Frontend)
- [ ] E2E integration against PostgreSQL container (`tests/api/`)
- [ ] Frontend Employee profile, directory, and document views
- [ ] Handoff to Module 3 (Attendance/Leave)
