# Module 0 — Acceptance Criteria

Every criterion below must be satisfied before Module 0 is considered production-ready. Each criterion is measurable and verifiable.

---

## Database (AC-DB)

| ID | Criterion | Verification Method |
|----|----------|-------------------|
| AC-DB-001 | All 12 database tables exist in PostgreSQL after running migrations | Run migrations; `\dt` shows all tables |
| AC-DB-002 | All migrations are reversible — running `.down.sql` drops all tables cleanly | Run up, then down, then up again without errors |
| AC-DB-003 | UUID primary keys are used on every table | `\d+ {table}` shows `uuid` type for `id` column |
| AC-DB-004 | `tenants.slug` has a unique index | `\di` shows unique index on `slug` |
| AC-DB-005 | `tenants.domain` has a unique index | `\di` shows unique index on `domain` |
| AC-DB-006 | `users` has a unique composite index on `(tenant_id, email)` | `\di` shows composite unique index |
| AC-DB-007 | `roles` has a unique composite index on `(tenant_id, name)` | `\di` shows composite unique index |
| AC-DB-008 | `permissions` has a unique composite index on `(resource, action)` | `\di` shows composite unique index |
| AC-DB-009 | `user_roles` has a unique composite index on `(user_id, role_id, tenant_id)` | `\di` shows composite unique index |
| AC-DB-010 | `role_permissions` has a unique composite index on `(role_id, permission_id, tenant_id)` | `\di` shows composite unique index |
| AC-DB-011 | `refresh_tokens.token_hash` has a unique index | `\di` shows unique index |
| AC-DB-012 | `password_reset_tokens.token_hash` has a unique index | `\di` shows unique index |
| AC-DB-013 | `mfa_configs` has a unique composite index on `(user_id, type)` | `\di` shows composite unique index |
| AC-DB-014 | Soft delete (`deleted_at`) is present on `tenants`, `users`, `roles` | Column exists with nullable timestamp type |
| AC-DB-015 | `audit_logs` has no `updated_at` or `deleted_at` columns | `\d+ audit_logs` confirms absence |
| AC-DB-016 | All foreign keys are properly defined and cascading correctly | Attempt to insert with invalid FK → error |
| AC-DB-017 | All tenant-scoped tables have an index on `tenant_id` | `\di` shows index |

---

## API Endpoints (AC-API)

| ID | Criterion | Verification Method |
|----|----------|-------------------|
| AC-API-001 | `POST /api/v1/tenants` creates a tenant and returns 201 | API test |
| AC-API-002 | `GET /api/v1/tenants` returns paginated tenant list and 200 | API test |
| AC-API-003 | `GET /api/v1/tenants/{id}` returns a single tenant and 200 | API test |
| AC-API-004 | `PATCH /api/v1/tenants/{id}` updates tenant fields and returns 200 | API test |
| AC-API-005 | `POST /api/v1/tenants/{id}/activate` activates a suspended tenant and returns 200 | API test |
| AC-API-006 | `POST /api/v1/tenants/{id}/suspend` suspends an active tenant and returns 200 | API test |
| AC-API-007 | `POST /api/v1/auth/login` returns JWT + refresh token on valid credentials | API test |
| AC-API-008 | `POST /api/v1/auth/login` returns 401 on invalid credentials | API test |
| AC-API-009 | `POST /api/v1/auth/logout` revokes session and returns 200 | API test |
| AC-API-010 | `POST /api/v1/auth/refresh` rotates tokens and returns new pair | API test |
| AC-API-011 | `POST /api/v1/auth/forgot-password` returns 200 regardless of email existence | API test |
| AC-API-012 | `POST /api/v1/auth/reset-password` resets password with valid token | API test |
| AC-API-013 | `POST /api/v1/users` creates a user and returns 201 | API test |
| AC-API-014 | `GET /api/v1/users` returns paginated user list scoped to tenant | API test |
| AC-API-015 | `GET /api/v1/users/{id}` returns a single user and 200 | API test |
| AC-API-016 | `PATCH /api/v1/users/{id}` updates user profile and returns 200 | API test |
| AC-API-017 | `POST /api/v1/users/{id}/deactivate` deactivates user and revokes sessions | API test |
| AC-API-018 | `POST /api/v1/roles` creates a role and returns 201 | API test |
| AC-API-019 | `GET /api/v1/roles` returns paginated role list scoped to tenant | API test |
| AC-API-020 | `PATCH /api/v1/roles/{id}` updates role and returns 200 | API test |
| AC-API-021 | `DELETE /api/v1/roles/{id}` deletes non-system role and returns 200 | API test |
| AC-API-022 | `GET /api/v1/permissions` returns all permissions | API test |
| AC-API-023 | `POST /api/v1/roles/{id}/permissions` assigns permissions to role | API test |
| AC-API-024 | `POST /api/v1/users/{id}/roles` assigns roles to user | API test |
| AC-API-025 | All endpoints return consistent JSON error format | Review all error responses |
| AC-API-026 | All list endpoints support `page` and `per_page` query parameters | API test |

---

## Authentication (AC-AUTH)

| ID | Criterion | Verification Method |
|----|----------|-------------------|
| AC-AUTH-001 | Login with valid credentials returns JWT access token with correct claims | Decode JWT and verify claims |
| AC-AUTH-002 | JWT expires after 15 minutes | Attempt API call with token after 15 min → 401 |
| AC-AUTH-003 | Login with wrong password returns 401 with generic message | API test |
| AC-AUTH-004 | Login with non-existent email returns 401 with same generic message | API test |
| AC-AUTH-005 | Login against a suspended tenant returns 403 | API test |
| AC-AUTH-006 | Login with an inactive user returns 403 | API test |
| AC-AUTH-007 | Logout revokes session and refresh token | Verify in database |
| AC-AUTH-008 | Refresh token rotation works — new token pair issued, old revoked | API test + DB check |
| AC-AUTH-009 | Refresh token reuse triggers security response — all tokens/sessions revoked | API test |
| AC-AUTH-010 | Forgot-password creates token and publishes event | DB check + event verification |
| AC-AUTH-011 | Reset-password with valid token updates password hash | DB check |
| AC-AUTH-012 | Reset-password revokes all sessions and refresh tokens | DB check |
| AC-AUTH-013 | Expired password reset token is rejected | API test |
| AC-AUTH-014 | Used password reset token is rejected | API test |
| AC-AUTH-015 | `last_login_at` is updated on successful login | DB check |

---

## Authorization (AC-AUTHZ)

| ID | Criterion | Verification Method |
|----|----------|-------------------|
| AC-AUTHZ-001 | Unauthenticated requests to protected endpoints return 401 | API test |
| AC-AUTHZ-002 | Authenticated user without required permission gets 403 | API test |
| AC-AUTHZ-003 | Authenticated user with required permission gets 200 | API test |
| AC-AUTHZ-004 | System roles cannot be deleted | API test → 403 |
| AC-AUTHZ-005 | System role names cannot be changed | API test → 403 |
| AC-AUTHZ-006 | Permission changes take effect immediately (not cached beyond 5 min) | Assign permission → verify access within 5 min |

---

## Tenant Isolation (AC-ISO)

| ID | Criterion | Verification Method |
|----|----------|-------------------|
| AC-ISO-001 | User in Tenant A cannot see users in Tenant B | API test with two tenants |
| AC-ISO-002 | User in Tenant A cannot see roles in Tenant B | API test |
| AC-ISO-003 | User in Tenant A cannot assign roles from Tenant B | API test |
| AC-ISO-004 | JWT with Tenant A's tenant_id cannot access Tenant B's subdomain | API test → 403 |
| AC-ISO-005 | Audit logs are filtered by tenant_id | DB query verification |

---

## Events (AC-EVT)

| ID | Criterion | Verification Method |
|----|----------|-------------------|
| AC-EVT-001 | `TenantCreated` event is published after tenant creation | RabbitMQ consumer test |
| AC-EVT-002 | `TenantActivated` event is published after activation | RabbitMQ consumer test |
| AC-EVT-003 | `TenantSuspended` event is published after suspension | RabbitMQ consumer test |
| AC-EVT-004 | `UserCreated` event is published after user creation | RabbitMQ consumer test |
| AC-EVT-005 | `UserInvited` event is published after invitation | RabbitMQ consumer test |
| AC-EVT-006 | `UserDeactivated` event is published after deactivation | RabbitMQ consumer test |
| AC-EVT-007 | `RoleAssigned` event is published after role assignment | RabbitMQ consumer test |
| AC-EVT-008 | `PermissionAssigned` event is published after permission assignment | RabbitMQ consumer test |
| AC-EVT-009 | `PasswordReset` event is published after password reset | RabbitMQ consumer test |
| AC-EVT-010 | `SessionRevoked` event is published after logout | RabbitMQ consumer test |
| AC-EVT-011 | All events contain correct envelope fields (event_id, type, occurred_at, tenant_id) | Payload inspection |
| AC-EVT-012 | Events are published after database transaction commits, not before | Test with transaction rollback |

---

## Audit Logging (AC-AUD)

| ID | Criterion | Verification Method |
|----|----------|-------------------|
| AC-AUD-001 | Login success creates audit log entry | DB query |
| AC-AUD-002 | Login failure creates audit log entry | DB query |
| AC-AUD-003 | User creation creates audit log entry | DB query |
| AC-AUD-004 | User deactivation creates audit log entry | DB query |
| AC-AUD-005 | Role assignment creates audit log entry | DB query |
| AC-AUD-006 | Tenant suspension creates audit log entry | DB query |
| AC-AUD-007 | Audit log entries cannot be updated | Attempt UPDATE → verify failure or no-op |
| AC-AUD-008 | Audit log entries cannot be deleted | Attempt DELETE → verify failure or no-op |
| AC-AUD-009 | Audit logs include IP address and user agent | DB query inspection |
| AC-AUD-010 | Failed login audit logs do not contain passwords | DB query inspection |

---

## Security (AC-SEC)

| ID | Criterion | Verification Method |
|----|----------|-------------------|
| AC-SEC-001 | Passwords are stored as bcrypt hashes | DB query: `password_hash` starts with `$2a$12$` |
| AC-SEC-002 | No API response includes password hash | Inspect all user-related API responses |
| AC-SEC-003 | Rate limiting blocks excessive login attempts | Send 11 requests in 15 min → 429 on 11th |
| AC-SEC-004 | Rate limiting blocks excessive forgot-password requests | Send 6 requests in 1 hour → 429 on 6th |
| AC-SEC-005 | Forgot-password returns same response for existent and non-existent emails | Compare responses → identical |
| AC-SEC-006 | Login returns same error for wrong email and wrong password | Compare error messages → identical |
| AC-SEC-007 | JWT signature tampering is detected | Modify JWT payload → 401 |
| AC-SEC-008 | Expired JWT is rejected | Wait 15 min → 401 |
| AC-SEC-009 | Cross-tenant JWT usage is rejected | JWT for Tenant A used on Tenant B subdomain → 403 |
| AC-SEC-010 | SQL injection via input fields is prevented | Attempt injection payload → no SQL execution |

---

## Performance (AC-PERF)

| ID | Criterion | Verification Method |
|----|----------|-------------------|
| AC-PERF-001 | Login endpoint responds in < 300ms (p95) | Load test with k6 or wrk |
| AC-PERF-002 | Single-resource GET responds in < 200ms (p95) | Load test |
| AC-PERF-003 | List endpoint with 100 items responds in < 500ms (p95) | Load test |
| AC-PERF-004 | System handles 100 concurrent login requests without errors | Load test |

---

## Code Quality (AC-CQ)

| ID | Criterion | Verification Method |
|----|----------|-------------------|
| AC-CQ-001 | `go build ./...` compiles without errors | CI build |
| AC-CQ-002 | `go vet ./...` passes without warnings | CI build |
| AC-CQ-003 | `go test ./internal/identity/...` passes | CI test |
| AC-CQ-004 | `go test -race ./internal/identity/...` passes | CI test with race detector |
| AC-CQ-005 | Service layer test coverage ≥ 90% | `go test -cover` report |
| AC-CQ-006 | Overall module test coverage ≥ 80% | `go test -cover` report |
| AC-CQ-007 | No TODO comments in production code | `grep -r "TODO" internal/identity/` returns empty |
| AC-CQ-008 | All exported functions have documentation comments | `golint` or `go doc` check |
| AC-CQ-009 | Consistent error handling — no raw `panic()` calls | Code review |
| AC-CQ-010 | All database queries use parameterized inputs (no string concatenation) | Code review |
