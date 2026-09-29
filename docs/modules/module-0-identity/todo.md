# Module 0 — Todo (operational mirror — canonical task list remains implementation-checklist.md)

## P0 (blocks compile/boot)
- [ ] G0-1 [plan P1] Add 11 missing models (tenant_settings, user, role, permission, user_role, role_permission, session, refresh_token, password_reset_token, mfa_config, audit_log)
  - Reason: tenant.go refs do not exist; package does not compile
  - Dependencies: database-schema.dbml + business-rules.md (TN/RB/PW/SS/RT/MF/SD/AD)
  - Acceptance: `go build ./internal/identity/models/...` + `go vet` clean; tags match DBML
  - Test: model unit tests (UUID hook, constraint sanity)
- [ ] G0-2 [plan P2] 12 migration pairs up/down clean
  - Acceptance: migrate up → 12 tables; down → clean; up again, no errors
  - Test: `\dt` + index/constraint verification per acceptance-criteria AC-DB
- [ ] G0-3 [plan P3] cmd/main.go + compose PG/Redis/RabbitMQ so API boots
  - Acceptance: `go run cmd/main.go` serves; health check 200
  - Test: boot smoke + DB ping

## P1 (data + logic)
- [ ] G0-4 DTOs (7 files) + shared/utils/validator.go
- [ ] G0-5 12 repositories (interfaces + GORM, tenant_id-scoped, tx injection)
- [ ] G0-6 TenantService (TN-001..013, events TenantCreated/Activated/Suspended)
- [ ] G0-7 AuthService (AU/PW/RT rules, rotation + reuse-revokes-all)
- [ ] G0-8 UserService (invite/deactivate + session revocation, UserCreated/Invited/Deactivated)
- [ ] G0-9 Role + Permission/Session/Token/Audit services (RB rules, idempotent assigns, immutable audit)

## P2 (delivery + integration)
- [ ] G0-10 Controllers + routes (24 endpoints, envelopes, status map)
- [ ] G0-11 Middleware (CORS → RateLimit → TenantResolver → Auth → RBAC → Audit)
- [ ] G0-12 Events/queue (12 types, envelope, retry/DLQ, publish-after-commit)
- [ ] G0-13 Frontend identity vs live API (login, guards, user/role pages)
- [ ] G0-14 E2E + hardening (SC-001..SC-010, p95 budgets, coverage 80/90)

## P3 (v2 — explicitly deferred: FE-001..FE-010)
- OAuth2/SSO, API keys, feature flags, lockout, email verification, role hierarchy, ABAC, GDPR export, login history, webhooks.
