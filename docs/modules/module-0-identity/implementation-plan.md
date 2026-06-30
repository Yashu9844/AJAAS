# Module 0 — Implementation Plan

Implementation is divided into 8 phases. Each phase is self-contained and builds on the previous. Phases must be completed in order.

---

## Phase 1: Database

**Goal**: Create all GORM models and SQL migration files. Verify schema correctness.

### Tasks

1. **Shared base models** — Create `BaseModel` (UUID id, created_at, updated_at, deleted_at) and `TenantBaseModel` (adds tenant_id) in `internal/shared/database/`.

2. **Tenant model** — `internal/identity/models/tenant.go`
   - UUID PK, name, slug (unique), domain (unique, nullable), status (enum), plan
   - Soft delete support
   - HasMany: Users, Roles, TenantSettings, Sessions, AuditLogs

3. **TenantSettings model** — `internal/identity/models/tenant_settings.go`
   - UUID PK, tenant_id FK, key, value
   - Unique constraint: (tenant_id, key)

4. **User model** — `internal/identity/models/user.go`
   - UUID PK, tenant_id FK, email, password_hash, first_name, last_name, phone, avatar_url, status, email_verified_at, last_login_at
   - Soft delete support
   - Unique constraint: (tenant_id, email)
   - HasMany: UserRoles, Sessions, RefreshTokens

5. **Role model** — `internal/identity/models/role.go`
   - UUID PK, tenant_id FK, name, description, is_system
   - Soft delete support
   - Unique constraint: (tenant_id, name)

6. **Permission model** — `internal/identity/models/permission.go`
   - UUID PK, resource, action, description
   - No soft delete, no tenant_id (global)
   - Unique constraint: (resource, action)

7. **UserRole model** — `internal/identity/models/user_role.go`
   - UUID PK, user_id FK, role_id FK, tenant_id FK, assigned_at, assigned_by
   - Unique constraint: (user_id, role_id, tenant_id)

8. **RolePermission model** — `internal/identity/models/role_permission.go`
   - UUID PK, role_id FK, permission_id FK, tenant_id FK
   - Unique constraint: (role_id, permission_id, tenant_id)

9. **Session model** — `internal/identity/models/session.go`
   - UUID PK, user_id FK, tenant_id FK, ip_address, user_agent, expires_at, revoked_at

10. **RefreshToken model** — `internal/identity/models/refresh_token.go`
    - UUID PK, user_id FK, tenant_id FK, token_hash (unique), expires_at, revoked_at

11. **PasswordResetToken model** — `internal/identity/models/password_reset_token.go`
    - UUID PK, user_id FK, tenant_id FK, token_hash (unique), expires_at, used_at

12. **MFAConfig model** — `internal/identity/models/mfa_config.go`
    - UUID PK, user_id FK, tenant_id FK, type, secret, is_enabled, verified_at
    - Unique constraint: (user_id, type)

13. **AuditLog model** — `internal/identity/models/audit_log.go`
    - UUID PK, tenant_id FK, user_id FK (nullable), action, resource, resource_id, metadata (JSONB), ip_address, user_agent, created_at
    - No updated_at, no deleted_at

14. **SQL migrations** — Create 12 migration file pairs (`.up.sql` / `.down.sql`) in `backend/migrations/`
    - Enable uuid-ossp extension
    - Create all tables with proper types, constraints, indexes, and foreign keys
    - Down migrations drop tables in reverse dependency order

### Verification

- `go build ./internal/identity/models/...` compiles
- Run migrations against a test PostgreSQL database
- Verify all tables, indexes, and constraints exist via `\d+` in psql

### Dependencies

None. This is the first phase.

---

## Phase 2: DTOs

**Goal**: Create all request/response DTOs with JSON tags and validation tags.

### Tasks

1. **Pagination DTO** — `internal/identity/dto/pagination_dto.go`
   - PaginationRequest (page, per_page with defaults and validation)
   - PaginationMeta (page, per_page, total_items, total_pages)

2. **Tenant DTOs** — `internal/identity/dto/tenant_dto.go`
   - CreateTenantRequest, UpdateTenantRequest, TenantResponse, TenantListResponse

3. **Auth DTOs** — `internal/identity/dto/auth_dto.go`
   - LoginRequest, LoginResponse, RefreshTokenRequest, RefreshTokenResponse
   - ForgotPasswordRequest, ResetPasswordRequest, MessageResponse

4. **User DTOs** — `internal/identity/dto/user_dto.go`
   - CreateUserRequest, UpdateUserRequest, InviteUserRequest, UserResponse, UserListResponse

5. **Role DTOs** — `internal/identity/dto/role_dto.go`
   - CreateRoleRequest, UpdateRoleRequest, AssignRoleRequest, AssignPermissionsRequest
   - RoleResponse, RoleListResponse

6. **Permission DTOs** — `internal/identity/dto/permission_dto.go`
   - PermissionResponse, PermissionListResponse

7. **Session DTOs** — `internal/identity/dto/session_dto.go`
   - SessionResponse

8. **Error DTOs** — `internal/shared/errors/errors.go`
   - ErrorResponse, ValidationErrorDetail
   - Predefined error codes (VALIDATION_ERROR, UNAUTHORIZED, FORBIDDEN, NOT_FOUND, CONFLICT, RATE_LIMITED, INTERNAL_ERROR)

9. **Validation helper** — `internal/shared/utils/validator.go`
   - ValidateStruct function returning structured validation errors

### Verification

- `go build ./internal/identity/dto/...` compiles
- All DTOs have proper JSON tags (snake_case)
- All request DTOs have proper validation tags

### Dependencies

Phase 1 (models are referenced for type alignment)

---

## Phase 3: Repositories

**Goal**: Create repository interfaces and GORM implementations for all domain entities.

### Tasks

1. **Repository interfaces** — Define interfaces in `internal/identity/repositories/`
   - TenantRepository
   - UserRepository
   - RoleRepository
   - PermissionRepository
   - UserRoleRepository
   - RolePermissionRepository
   - SessionRepository
   - RefreshTokenRepository
   - PasswordResetTokenRepository
   - MFAConfigRepository
   - TenantSettingsRepository
   - AuditLogRepository

2. **GORM implementations** — Implement each interface using GORM
   - All queries scoped by tenant_id (except permissions and global tenant queries)
   - Proper use of GORM preloading for relationships
   - Transaction support via `*gorm.DB` injection
   - Pagination support for list methods
   - Soft delete handled by GORM automatically

3. **Database connection** — `internal/shared/database/database.go`
   - Connection initialization with pool configuration
   - AutoMigrate registration

### Verification

- `go build ./internal/identity/repositories/...` compiles
- Repository tests with test database (see Phase 8)

### Dependencies

Phase 1 (models), Phase 2 (DTOs for pagination)

---

## Phase 4: Services

**Goal**: Implement all business logic as service layer use cases.

### Tasks

1. **TenantService** — Create, Get, List, Update, Activate, Suspend
   - Enforce business rules (TN-001 through TN-013)
   - Publish events (TenantCreated, TenantActivated, TenantSuspended)
   - Audit logging

2. **AuthService** — Login, Logout, Refresh, ForgotPassword, ResetPassword
   - Enforce authentication rules (AU-001 through AU-014)
   - Enforce password rules (PW-001 through PW-010)
   - Enforce refresh token rules (RT-001 through RT-007)
   - JWT generation and validation
   - Refresh token rotation with reuse detection

3. **UserService** — Create, Get, List, Update, Deactivate, Invite
   - Enforce user management rules
   - Role assignment during creation
   - Session/token revocation on deactivation
   - Publish events (UserCreated, UserInvited, UserDeactivated)

4. **RoleService** — Create, Get, List, Update, Delete, AssignPermissions, AssignRoles
   - Enforce RBAC rules (RB-001 through RB-012)
   - System role protection
   - Idempotent assignment operations
   - Publish events (RoleAssigned, RoleRevoked, PermissionAssigned, PermissionRevoked)

5. **PermissionService** — List
   - Read-only service

6. **SessionService** — Create, Validate, Revoke, RevokeAll
   - Redis caching integration
   - Enforce session rules (SS-001 through SS-008)

7. **TokenService** — Refresh token and password reset token generation, validation, rotation
   - Cryptographic random generation
   - SHA-256 hashing
   - Enforce token rules

8. **AuditService** — Create audit log entries
   - Accept structured audit entries from all services
   - Best-effort (non-blocking)

### Verification

- `go build ./internal/identity/services/...` compiles
- Unit tests for each service with mocked repositories

### Dependencies

Phase 3 (repositories), Phase 2 (DTOs)

---

## Phase 5: Controllers

**Goal**: Implement HTTP controllers that parse requests, call services, and return responses.

### Tasks

1. **TenantController** — Handlers for all tenant endpoints
   - Parse request body → DTO
   - Validate DTO
   - Call TenantService
   - Map result to response DTO
   - Return appropriate HTTP status code

2. **AuthController** — Handlers for all auth endpoints
   - Login, Logout, Refresh, ForgotPassword, ResetPassword

3. **UserController** — Handlers for all user endpoints
   - Create, List, Get, Update, Deactivate

4. **RoleController** — Handlers for all role endpoints
   - Create, List, Update, Delete, AssignPermissions, AssignRoles

5. **PermissionController** — Handler for permission listing

6. **Route registration** — `internal/identity/routes/routes.go`
   - Register all routes with proper grouping
   - Apply middleware per route group

### Verification

- `go build ./internal/identity/controllers/...` compiles
- API tests against running server (see Phase 8)

### Dependencies

Phase 4 (services), Phase 2 (DTOs)

---

## Phase 6: Middleware

**Goal**: Implement the middleware stack for authentication, authorization, tenant resolution, and audit logging.

### Tasks

1. **Tenant Resolver middleware** — `internal/identity/middleware/tenant.go`
   - Extract subdomain from `Host` header
   - Resolve tenant from slug
   - Set tenant_id in Gin context
   - Return 404 if tenant not found, 403 if suspended

2. **JWT Auth middleware** — `internal/identity/middleware/auth.go`
   - Extract JWT from `Authorization: Bearer` header
   - Validate signature, expiry, claims
   - Check JTI blacklist in Redis
   - Set user_id, tenant_id, roles in Gin context
   - Verify JWT tenant_id matches resolved tenant_id

3. **RBAC middleware** — `internal/identity/middleware/rbac.go`
   - Extract required permission from route configuration
   - Check user's permissions (from cache or database)
   - Return 403 if permission not found

4. **Rate limiter middleware** — `internal/shared/middleware/rate_limiter.go`
   - Redis-backed sliding window counter
   - Configurable per endpoint

5. **Audit middleware** — `internal/identity/middleware/audit.go`
   - Post-handler audit log creation
   - Capture request/response metadata

6. **CORS middleware** — `internal/shared/middleware/cors.go`
   - Configurable allowed origins

### Verification

- `go build ./internal/identity/middleware/...` compiles
- Integration tests with middleware stack

### Dependencies

Phase 4 (services for tenant/user resolution), Phase 5 (controllers to attach middleware)

---

## Phase 7: Events

**Goal**: Implement event publishing infrastructure and wire up domain events.

### Tasks

1. **Event publisher interface** — `internal/shared/queue/publisher.go`
   - Define interface for publishing events
   - RabbitMQ implementation

2. **Event definitions** — `internal/identity/events/events.go`
   - All 12 event types with payload structs
   - Event envelope struct

3. **RabbitMQ connection** — `internal/shared/queue/rabbitmq.go`
   - Connection management with reconnection logic
   - Exchange declaration
   - Publishing with confirmation

4. **Failed event persistence** — Fallback table for events that fail to publish
   - Background worker for retry

5. **Wire events into services** — Inject event publisher into all services that publish events

### Verification

- `go build ./internal/identity/events/...` compiles
- Integration test: service action → event published → event received by test consumer

### Dependencies

Phase 4 (services), Phase 1 (models for fallback table)

---

## Phase 8: Testing

**Goal**: Comprehensive test coverage across all layers.

### Tasks

1. **Unit tests — Services** (highest priority)
   - Mock repositories
   - Test every business rule
   - Test every edge case
   - Target: 90% coverage

2. **Unit tests — Controllers**
   - Mock services
   - Test request parsing and validation
   - Test response formatting
   - Test error handling

3. **Unit tests — Middleware**
   - Test JWT validation
   - Test tenant resolution
   - Test RBAC enforcement
   - Test rate limiting

4. **Repository integration tests**
   - Use test PostgreSQL database (Docker)
   - Test CRUD operations
   - Test unique constraint violations
   - Test soft delete behaviour
   - Test pagination

5. **API integration tests**
   - Full HTTP request/response cycle
   - Test authentication flow end-to-end
   - Test authorization enforcement
   - Test tenant isolation

6. **Security tests**
   - Test information leakage prevention
   - Test rate limiting behaviour
   - Test token reuse detection
   - Test cross-tenant access prevention

7. **Performance tests** (optional for initial release)
   - Load test login endpoint
   - Load test permission check

### Verification

- `go test ./internal/identity/...` passes
- Coverage report ≥ 80% overall, ≥ 90% for services
- No race conditions (`go test -race`)

### Dependencies

All previous phases
