# Module 0 — Current Goal

Goal: G0-1 Models compile — add the 11 missing identity models so `go build ./internal/identity/models/...` passes.
Why: `models/tenant.go` references User, Role, Session, AuditLog, TenantSettings which do not exist; the package does not compile and nothing downstream (migrations → repos → services → API) can start. First item in docs/PENDING_WORK.md build order.
Scope: `backend/internal/identity/models/*.go` only — tenant_settings.go, user.go, role.go, permission.go, user_role.go, role_permission.go, session.go, refresh_token.go, password_reset_token.go, mfa_config.go, audit_log.go. Fix tenant.go only if its tags diverge from database-schema.dbml.
Non-goals: migrations, DTOs, repos, services, controllers, middleware, events, frontend, bootstrap.
Success Criteria:
- [ ] `go build ./internal/identity/models/...` passes
- [ ] `go vet` clean on the package
- [ ] GORM tags match database-schema.dbml (uniques, indexes, FKs, soft-delete on tenants/users/roles only, audit_logs without updated_at/deleted_at)
- [ ] Model unit tests pass (UUID BeforeCreate + constraint sanity)
- [ ] plan.md P1 marked DONE with proof, P2 promoted to CURRENT, current-status.md + implementation-checklist.md + handoff.md updated
