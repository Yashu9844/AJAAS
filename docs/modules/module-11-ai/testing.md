# Module 11 — Testing

Strategy (MASTER_PROMPT section 14): unit (services 90 percent) + integration (Docker PG) + contract/API + security (enumeration, tamper, reuse, injection) + golden (protected) + performance (p95 budgets).

Commands (activate at P2 implementation):
- go build ./... ; go vet ./... ; gofmt -l .
- go test -race ./internal/ai/...  (backend path: backend/internal/ai)
- Frontend module path: frontend/src/modules/ai
- grep -r TODO (must be empty); migrations up/down/up clean.
