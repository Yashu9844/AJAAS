# Module 0 — Implementation Checklist

This checklist converts every implementation task into a trackable item. Each item must be completed and verified before Module 0 is considered production-ready.

---

## Phase 1: Database Models & Migrations

### Shared Infrastructure

- [ ] Create `internal/shared/database/base.go` — BaseModel (UUID id, created_at, updated_at, deleted_at)
- [ ] Create `internal/shared/database/base.go` — TenantBaseModel (embeds BaseModel + tenant_id)
- [ ] Create `internal/shared/database/database.go` — PostgreSQL connection initialization with pool configuration

### GORM Models

- [ ] Create `internal/identity/models/tenant.go` — Tenant model with GORM tags, indexes, relationships
- [ ] Create `internal/identity/models/tenant_settings.go` — TenantSettings model with unique(tenant_id, key) constraint
- [ ] Create `internal/identity/models/user.go` — User model with unique(tenant_id, email) constraint, soft delete
- [ ] Create `internal/identity/models/role.go` — Role model with unique(tenant_id, name) constraint, soft delete
- [ ] Create `internal/identity/models/permission.go` — Permission model with unique(resource, action) constraint
- [ ] Create `internal/identity/models/user_role.go` — UserRole join model with unique(user_id, role_id, tenant_id)
- [ ] Create `internal/identity/models/role_permission.go` — RolePermission join model with unique(role_id, permission_id, tenant_id)
- [ ] Create `internal/identity/models/session.go` — Session model with user_id, tenant_id, ip_address, user_agent, expires_at, revoked_at
- [ ] Create `internal/identity/models/refresh_token.go` — RefreshToken model with unique(token_hash)
- [ ] Create `internal/identity/models/password_reset_token.go` — PasswordResetToken model with unique(token_hash)
- [ ] Create `internal/identity/models/mfa_config.go` — MFAConfig model with unique(user_id, type)
- [ ] Create `internal/identity/models/audit_log.go` — AuditLog model (no updated_at, no deleted_at, JSONB metadata)

### SQL Migrations

- [ ] Create `migrations/000001_create_tenants.up.sql` — Enable uuid-ossp, CREATE TABLE tenants
- [ ] Create `migrations/000001_create_tenants.down.sql` — DROP TABLE tenants
- [ ] Create `migrations/000002_create_users.up.sql` — CREATE TABLE users with FK to tenants
- [ ] Create `migrations/000002_create_users.down.sql` — DROP TABLE users
- [ ] Create `migrations/000003_create_roles.up.sql` — CREATE TABLE roles with FK to tenants
- [ ] Create `migrations/000003_create_roles.down.sql` — DROP TABLE roles
- [ ] Create `migrations/000004_create_permissions.up.sql` — CREATE TABLE permissions
- [ ] Create `migrations/000004_create_permissions.down.sql` — DROP TABLE permissions
- [ ] Create `migrations/000005_create_user_roles.up.sql` — CREATE TABLE user_roles with FKs
- [ ] Create `migrations/000005_create_user_roles.down.sql` — DROP TABLE user_roles
- [ ] Create `migrations/000006_create_role_permissions.up.sql` — CREATE TABLE role_permissions with FKs
- [ ] Create `migrations/000006_create_role_permissions.down.sql` — DROP TABLE role_permissions
- [ ] Create `migrations/000007_create_sessions.up.sql` — CREATE TABLE sessions with FKs
- [ ] Create `migrations/000007_create_sessions.down.sql` — DROP TABLE sessions
- [ ] Create `migrations/000008_create_refresh_tokens.up.sql` — CREATE TABLE refresh_tokens
- [ ] Create `migrations/000008_create_refresh_tokens.down.sql` — DROP TABLE refresh_tokens
- [ ] Create `migrations/000009_create_password_reset_tokens.up.sql` — CREATE TABLE password_reset_tokens
- [ ] Create `migrations/000009_create_password_reset_tokens.down.sql` — DROP TABLE password_reset_tokens
- [ ] Create `migrations/000010_create_mfa_configs.up.sql` — CREATE TABLE mfa_configs
- [ ] Create `migrations/000010_create_mfa_configs.down.sql` — DROP TABLE mfa_configs
- [ ] Create `migrations/000011_create_tenant_settings.up.sql` — CREATE TABLE tenant_settings
- [ ] Create `migrations/000011_create_tenant_settings.down.sql` — DROP TABLE tenant_settings
- [ ] Create `migrations/000012_create_audit_logs.up.sql` — CREATE TABLE audit_logs (no soft delete columns)
- [ ] Create `migrations/000012_create_audit_logs.down.sql` — DROP TABLE audit_logs

### Verification

- [ ] `go build ./internal/identity/models/...` compiles
- [ ] Migrations run successfully against a test PostgreSQL database
- [ ] All tables, indexes, and constraints verified in database

---

## Phase 2: DTOs & Validation

### DTOs

- [ ] Create `internal/identity/dto/pagination_dto.go` — PaginationRequest, PaginationMeta
- [ ] Create `internal/identity/dto/tenant_dto.go` — CreateTenantRequest, UpdateTenantRequest, TenantResponse, TenantListResponse
- [ ] Create `internal/identity/dto/auth_dto.go` — LoginRequest, LoginResponse, RefreshTokenRequest, RefreshTokenResponse, ForgotPasswordRequest, ResetPasswordRequest, MessageResponse
- [ ] Create `internal/identity/dto/user_dto.go` — CreateUserRequest, UpdateUserRequest, InviteUserRequest, UserResponse, UserListResponse
- [ ] Create `internal/identity/dto/role_dto.go` — CreateRoleRequest, UpdateRoleRequest, AssignRoleRequest, AssignPermissionsRequest, RoleResponse, RoleListResponse
- [ ] Create `internal/identity/dto/permission_dto.go` — PermissionResponse, PermissionListResponse
- [ ] Create `internal/identity/dto/session_dto.go` — SessionResponse

### Shared Error Types

- [ ] Create `internal/shared/errors/errors.go` — AppError, ValidationErrorDetail, predefined error codes

### Validation

- [ ] Add `validate:"..."` tags to all request DTOs (required, email, min, max, uuid, oneof)
- [ ] Create `internal/shared/utils/validator.go` — ValidateStruct helper function
- [ ] Create `internal/identity/validators/validators.go` — Custom validators (slug format, reserved slugs)

### Verification

- [ ] `go build ./internal/identity/dto/...` compiles
- [ ] All JSON tags use snake_case
- [ ] All validation tags are correct

---

## Phase 3: Repositories

### Interfaces

- [ ] Create `internal/identity/repositories/interfaces.go` — All repository interfaces (TenantRepository, UserRepository, RoleRepository, PermissionRepository, UserRoleRepository, RolePermissionRepository, SessionRepository, RefreshTokenRepository, PasswordResetTokenRepository, MFAConfigRepository, TenantSettingsRepository, AuditLogRepository)

### Implementations

- [ ] Create `internal/identity/repositories/tenant_repository.go` — GORM implementation
- [ ] Create `internal/identity/repositories/user_repository.go` — GORM implementation with tenant_id scoping
- [ ] Create `internal/identity/repositories/role_repository.go` — GORM implementation with tenant_id scoping
- [ ] Create `internal/identity/repositories/permission_repository.go` — GORM implementation (global)
- [ ] Create `internal/identity/repositories/session_repository.go` — GORM implementation
- [ ] Create `internal/identity/repositories/refresh_token_repository.go` — GORM implementation
- [ ] Create `internal/identity/repositories/password_reset_token_repository.go` — GORM implementation
- [ ] Create `internal/identity/repositories/mfa_config_repository.go` — GORM implementation
- [ ] Create `internal/identity/repositories/tenant_settings_repository.go` — GORM implementation
- [ ] Create `internal/identity/repositories/audit_log_repository.go` — GORM implementation (create-only, no update/delete)

### Verification

- [ ] `go build ./internal/identity/repositories/...` compiles
- [ ] All queries use parameterized inputs
- [ ] All tenant-scoped queries filter by tenant_id

---

## Phase 4: Services

- [ ] Create `internal/identity/services/tenant_service.go` — Create, Get, List, Update, Activate, Suspend
- [ ] Create `internal/identity/services/auth_service.go` — Login, Logout, Refresh, ForgotPassword, ResetPassword
- [ ] Create `internal/identity/services/user_service.go` — Create, Get, List, Update, Deactivate, Invite
- [ ] Create `internal/identity/services/role_service.go` — Create, Get, List, Update, Delete, AssignPermissions, AssignRoles
- [ ] Create `internal/identity/services/permission_service.go` — List
- [ ] Create `internal/identity/services/session_service.go` — Create, Validate, Revoke, RevokeAll
- [ ] Create `internal/identity/services/token_service.go` — JWT generation/validation, refresh token ops, password reset token ops
- [ ] Create `internal/identity/services/audit_service.go` — Create audit entries

### Verification

- [ ] `go build ./internal/identity/services/...` compiles
- [ ] Services depend on interfaces, not concrete implementations
- [ ] Business rules from business-rules.md are enforced
- [ ] Transactions used for multi-table operations
- [ ] Events published after transaction commit

---

## Phase 5: Controllers

- [ ] Create `internal/identity/controllers/tenant_controller.go` — Handlers for POST, GET, GET/:id, PATCH/:id, POST/:id/activate, POST/:id/suspend
- [ ] Create `internal/identity/controllers/auth_controller.go` — Handlers for POST/login, POST/logout, POST/refresh, POST/forgot-password, POST/reset-password
- [ ] Create `internal/identity/controllers/user_controller.go` — Handlers for POST, GET, GET/:id, PATCH/:id, POST/:id/deactivate
- [ ] Create `internal/identity/controllers/role_controller.go` — Handlers for POST, GET, PATCH/:id, DELETE/:id, POST/:id/permissions
- [ ] Create `internal/identity/controllers/permission_controller.go` — Handler for GET

### Route Registration

- [ ] Create `internal/identity/routes/routes.go` — Register all routes with proper grouping and middleware

### Verification

- [ ] `go build ./internal/identity/controllers/...` compiles
- [ ] Controllers parse request body, validate DTO, call service, return response
- [ ] Controllers map service errors to HTTP status codes
- [ ] All 22 endpoints are registered

---

## Phase 6: Middleware

- [ ] Create `internal/identity/middleware/tenant.go` — Extract subdomain, resolve tenant, inject tenant_id
- [ ] Create `internal/identity/middleware/auth.go` — Extract JWT, validate, check blacklist, inject user context
- [ ] Create `internal/identity/middleware/rbac.go` — Check user permissions against required permission
- [ ] Create `internal/identity/middleware/audit.go` — Post-handler audit log creation
- [ ] Create `internal/shared/middleware/cors.go` — CORS configuration
- [ ] Create `internal/shared/middleware/rate_limiter.go` — Redis-backed rate limiter

### Verification

- [ ] `go build ./internal/identity/middleware/...` compiles
- [ ] Middleware stack order is correct (CORS → Rate Limit → Tenant → Auth → RBAC → Audit)
- [ ] Public endpoints bypass auth/RBAC middleware

---

## Phase 7: Events

- [ ] Create `internal/identity/events/events.go` — All 12 event type constants and payload structs
- [ ] Create `internal/shared/queue/publisher.go` — EventPublisher interface
- [ ] Create `internal/shared/queue/rabbitmq.go` — RabbitMQ implementation with reconnection
- [ ] Wire event publishing into TenantService (TenantCreated, TenantActivated, TenantSuspended)
- [ ] Wire event publishing into AuthService (PasswordReset, SessionRevoked)
- [ ] Wire event publishing into UserService (UserCreated, UserInvited, UserDeactivated)
- [ ] Wire event publishing into RoleService (RoleAssigned, RoleRevoked, PermissionAssigned, PermissionRevoked)

### Verification

- [ ] `go build ./internal/identity/events/...` compiles
- [ ] Events are published to correct exchange with correct routing keys
- [ ] Failed event publishing is logged and does not block operations

---

## Phase 8: Testing

### Unit Tests — Services

- [x] Create `internal/identity/services/tenant_service_test.go` — All tenant service test cases
- [x] Create `internal/identity/services/auth_service_test.go` — All auth service test cases
- [x] Create `internal/identity/services/user_service_test.go` — All user service test cases
- [x] Create `internal/identity/services/role_service_test.go` — All role service test cases
- [x] Create `internal/identity/services/permission_service_test.go` — Permission service test cases
- [x] Create `internal/identity/services/session_service_test.go` — Session service test cases
- [x] Create `internal/identity/services/token_service_test.go` — Token service test cases
- [x] Create `internal/identity/services/audit_service_test.go` — Audit service test cases

### Unit Tests — Controllers

- [x] Create `internal/identity/controllers/tenant_controller_test.go`
- [x] Create `internal/identity/controllers/auth_controller_test.go`
- [x] Create `internal/identity/controllers/user_controller_test.go`
- [x] Create `internal/identity/controllers/role_controller_test.go`
- [x] Create `internal/identity/controllers/permission_controller_test.go`

### Unit Tests — Middleware

- [x] Create `internal/identity/middleware/tenant_test.go`
- [x] Create `internal/identity/middleware/auth_test.go`
- [x] Create `internal/identity/middleware/rbac_test.go`
- [x] Create `internal/identity/middleware/audit_test.go`

### Integration Tests — Repositories

- [ ] Create repository integration tests with test PostgreSQL database
- [ ] Test CRUD, unique constraint violations, soft delete, pagination for each repository

### Integration Tests — API

- [ ] Create `tests/api/auth_test.go` — Full authentication flow tests
- [ ] Create `tests/api/tenant_test.go` — Tenant CRUD + lifecycle tests
- [ ] Create `tests/api/user_test.go` — User CRUD + isolation tests
- [ ] Create `tests/api/role_test.go` — Role CRUD + assignment tests
- [ ] Create `tests/api/permission_test.go` — Permission listing tests
- [ ] Create `tests/api/rbac_test.go` — RBAC enforcement tests
- [ ] Create `tests/api/tenant_isolation_test.go` — Cross-tenant access prevention tests

### Security Tests

- [ ] Create `tests/security/enumeration_test.go` — Login/forgot-password enumeration prevention
- [ ] Create `tests/security/jwt_test.go` — JWT tampering, expiry, cross-tenant
- [ ] Create `tests/security/rate_limit_test.go` — Rate limiting enforcement
- [ ] Create `tests/security/injection_test.go` — SQL injection prevention
- [ ] Create `tests/security/token_reuse_test.go` — Refresh token reuse detection

### Verification

- [ ] `go test ./internal/identity/...` passes
- [ ] `go test -race ./internal/identity/...` passes
- [ ] Service layer coverage ≥ 90%
- [ ] Overall module coverage ≥ 80%

---

## Phase 9: Documentation & Finalization

### API Documentation

- [ ] Create `api/swagger.yaml` — Complete OpenAPI 3.0 specification
- [ ] Verify swagger spec matches all implemented endpoints
- [ ] Document all request/response schemas
- [ ] Document all error responses

### Module Registration

- [ ] Create `internal/identity/module.go` — RegisterModule function
- [ ] Register all models for AutoMigrate
- [ ] Register all routes
- [ ] Wire dependency injection

### Final Checks

- [ ] No TODO comments in any file (`grep -r "TODO" internal/identity/`)
- [ ] No placeholder or fake implementations
- [ ] No hardcoded secrets or credentials
- [ ] All files have package-level documentation
- [ ] `go build ./...` compiles cleanly
- [ ] `go vet ./...` passes
- [ ] All tests pass
- [ ] Security checklist (from security.md) is complete
- [ ] All acceptance criteria (from acceptance-criteria.md) are met
