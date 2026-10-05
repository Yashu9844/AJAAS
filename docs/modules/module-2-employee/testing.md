# Module 2 — Testing Strategy

## Coverage & Quality Gates
- **Service Layer Coverage:** $\ge 90\%$ statement coverage on application services.
- **Package Coverage:** $\ge 80\%$ statement coverage across the module.
- **Sensors:** `go build ./...`, `go vet ./...`, `gofmt -l`, `go test ./internal/employee/...`.
- **Negative Paths:** Duplicate code, invalid status transitions, cross-tenant leaks, unverified user attachments.
