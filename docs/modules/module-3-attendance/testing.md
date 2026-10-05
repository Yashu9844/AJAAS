# Module 3 — Testing

Strategy (MASTER_PROMPT section 14): unit (services 90 percent) + integration (Docker PG) + contract/API + security (enumeration, tamper, reuse, injection) + golden (protected) + performance (p95 budgets).

Commands (activate at P2 implementation):
- go build ./... ; go vet ./... ; gofmt -l .
- go test -race ./internal/attendance/...  (backend path: backend/internal/attendance)
- Frontend module path: frontend/src/modules/attendance
- grep -r TODO (must be empty); migrations up/down/up clean.
