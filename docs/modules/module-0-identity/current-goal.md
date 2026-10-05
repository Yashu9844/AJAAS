# Module 0 — Current Goal

Goal: G0-14 E2E + hardening — remaining integration, security, and performance tests (API E2E, repo integration, rate-limit/injection/reuse, load budgets) plus final verification that SC-001..SC-010 hold on the merged backkendmod0 implementation.
Why: backkendmod0 landed models → migrations → DTOs → repos → services → controllers → middleware → events → bootstrap → Swagger with unit tests (services, controllers, middleware, models, validators, events). What is missing is the outer verification ring: repo integration against real PG, API E2E (login → access → refresh → logout, isolation, RBAC), security tests, and perf budgets. G0-1..G0-13 are DONE by merge.
Scope: `backend/tests/**` (new), `backend/internal/identity/**` fixes only where tests expose real bugs, `docs/modules/module-0-identity/implementation-checklist.md` Phase 8 remainder + Phase 9 final checks.
Non-goals: new endpoints, schema changes, frontend identity pages (G0-13 scope, separate goal if needed).
Success Criteria:
- [ ] `go test ./internal/identity/...` passes
- [ ] `go test -race ./internal/identity/...` passes
- [ ] Service coverage ≥90%, module ≥80%
- [ ] Repo integration tests pass against Docker PG (CRUD, uniques, soft delete, pagination)
- [ ] API E2E green: auth flow, tenant lifecycle, user isolation, role assignment, RBAC deny/allow
- [ ] Security tests green: enumeration-identical, JWT tamper/expiry/cross-tenant, rate-limit 429, injection blocked, reuse revokes all
- [ ] p95 budgets hold: single <200ms, list <500ms, login <300ms
- [ ] Phase 9 final checks: swagger matches endpoints, module.go wiring verified, no TODOs, no placeholders
- [ ] plan.md P14 marked DONE, current-status.md + implementation-checklist.md + handoff.md updated
