# Module 1 — Testing

Strategy (MASTER_PROMPT section 14): unit (services 90 percent) + integration (Docker PG) + contract/API + security (enumeration, tamper, reuse, injection) + golden (protected) + performance (p95 budgets).

Commands (activate at P2 implementation):
- go build ./... ; go vet ./... ; gofmt -l .
- go test -race ./internal/organization/...  (backend path: backend/internal/organization)
- Frontend module path: frontend/src/modules/organization
- grep -r TODO (must be empty); migrations up/down/up clean.
