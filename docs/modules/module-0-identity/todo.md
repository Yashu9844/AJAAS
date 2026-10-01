# Module 0 — Todo (merged backkendmod0 2026-10-01; G0-1..G0-12 DONE by merge, G0-14 CURRENT)

## P0 (done by merge 3af3d09)
- [x] G0-1 Models (12 models + models_test) — merge origin/backkendmod0
- [x] G0-2 12 migration pairs — merge
- [x] G0-3 cmd/main.go + module.go + compose PG/Redis/RabbitMQ — merge
- [x] G0-4 DTOs (7) + validators (shared + identity) — merge
- [x] G0-5 12 repositories — merge
- [x] G0-6..G0-9 Services (tenant, auth, user, role, permission, session, token, audit) + unit suites — merge
- [x] G0-10 Controllers + routes — merge
- [x] G0-11 Middleware stack — merge
- [x] G0-12 Events/queue — merge

## P0 (remaining — G0-14 CURRENT, needs toolchain: go + docker)
- [ ] Run `go build ./...`, `go vet ./...`, `gofmt -l` — BLOCKED here (no go toolchain)
  - Acceptance: clean output
- [ ] Run `go test ./internal/identity/...` + `-race` + coverage (≥90% services, ≥80% module)
- [ ] Repo integration tests (Docker PG): CRUD, uniques incl. soft-deleted, soft delete, pagination per repo
  - Acceptance: AC-DB-001..017 hold
- [ ] API E2E: auth flow (login → access → refresh → logout), tenant lifecycle, user isolation, role assignment, RBAC allow/deny, pagination + envelopes
- [ ] Security tests: enumeration-identical (login + forgot), JWT tamper/expiry/cross-tenant, rate-limit 429s, SQLi payloads, refresh reuse revokes all
- [ ] Perf: p95 single <200ms, list <500ms, login <300ms (k6/wrk)
- [ ] Phase 9 final: swagger ↔ endpoints verified, module.go wiring verified live, `grep -r TODO` empty, no placeholders

## P1 (Module 1 unblocked once G0-14 verdict lands)
- [ ] G1-1 Organization design (spec, architecture, contracts, goldens) — see module-1 plan.md

## P3 (v2 — explicitly deferred: FE-001..FE-010)
- OAuth2/SSO, API keys, feature flags, lockout, email verification, role hierarchy, ABAC, GDPR export, login history, webhooks.
