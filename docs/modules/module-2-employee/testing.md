# Module 2 — Testing

Strategy (MASTER_PROMPT section 14): unit (services 90 percent) + integration (Docker PG) + contract/API + security (enumeration, tamper, reuse, injection) + golden (protected) + performance (p95 budgets).

Commands (activate at P2 implementation):
- go build ./... ; go vet ./... ; gofmt -l .
- go test -race ./internal/employee/...  (backend path: backend/internal/employee)
- Frontend module path: frontend/src/modules/employee
- grep -r TODO (must be empty); migrations up/down/up clean.
