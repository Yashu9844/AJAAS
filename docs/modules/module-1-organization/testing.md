# Module 1 — Testing (P1: exact commands + suites for P2/P3)

Strategy: unit (services ≥90%, overall ≥80%) + integration (Docker PG) + contract/API + security + golden (protected, see golden-tests.md) + performance (NFR-P001..P004).

## Commands

```bash
cd backend
go build ./internal/organization/...
go vet ./internal/organization/...
gofmt -l internal/organization/
go test ./internal/organization/...                 # unit (RED-first, mocked Module 0 UserService)
go test -race ./internal/organization/...
go test -cover ./internal/organization/...          # ≥90% services, ≥80% module
grep -r "TODO" internal/organization/               # must be empty
# migrations (000013.. per P2): up → tables exist; down → clean; up again
# API E2E (needs live PG/Redis/RabbitMQ): go test ./tests/api/ -run TestOrg
# security: go test ./tests/security/ -run TestOrg
# perf: k6 run tests/performance/org_chart.js  (NFR-P003: p95 <800ms uncached, <100ms cached)
cd ../frontend && npx tsc --noEmit && npm run lint && npm run build   # P3 frontend slice
```

## Suites (P2 writes, P3 runs)

- Unit: department/team/designation/mapping/chart services (cycle/depth/guard logic), validators (code format, reserved), event payload builders.
- Integration: repos vs Docker PG — CRUD, uniques (incl. soft-deleted reservation), soft delete, pagination, children/report traversal, primary-swap atomicity.
- Contract/API: 20 endpoints (§4 frozen routes), envelopes, status map, pagination, filters.
- Security: isolation (G1), RBAC (G4), leak scan (G10).
- Golden: G1..G10 (protected).
- Regression: full Module 0 identity suite still green after org wiring (Module 0 tests re-run at P3).
