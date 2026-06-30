# Module 0 — Coding Guidelines

These guidelines apply to all code written within the Module 0 identity module. They establish the conventions that every developer (human or AI) must follow.

---

## Folder Structure

```
backend/
├── cmd/
│   └── main.go                          # Application entry point
├── configs/
│   ├── development.yaml                 # Dev environment config
│   └── production.yaml                  # Prod environment config
├── migrations/
│   ├── 000001_create_tenants.up.sql
│   ├── 000001_create_tenants.down.sql
│   └── ...
├── internal/
│   ├── identity/                        # Module 0
│   │   ├── module.go                    # Module registration
│   │   ├── controllers/
│   │   │   ├── tenant_controller.go
│   │   │   ├── auth_controller.go
│   │   │   ├── user_controller.go
│   │   │   ├── role_controller.go
│   │   │   └── permission_controller.go
│   │   ├── dto/
│   │   │   ├── pagination_dto.go
│   │   │   ├── tenant_dto.go
│   │   │   ├── auth_dto.go
│   │   │   ├── user_dto.go
│   │   │   ├── role_dto.go
│   │   │   ├── permission_dto.go
│   │   │   └── session_dto.go
│   │   ├── events/
│   │   │   └── events.go
│   │   ├── middleware/
│   │   │   ├── tenant.go
│   │   │   ├── auth.go
│   │   │   ├── rbac.go
│   │   │   └── audit.go
│   │   ├── models/
│   │   │   ├── tenant.go
│   │   │   ├── tenant_settings.go
│   │   │   ├── user.go
│   │   │   ├── role.go
│   │   │   ├── permission.go
│   │   │   ├── user_role.go
│   │   │   ├── role_permission.go
│   │   │   ├── session.go
│   │   │   ├── refresh_token.go
│   │   │   ├── password_reset_token.go
│   │   │   ├── mfa_config.go
│   │   │   └── audit_log.go
│   │   ├── repositories/
│   │   │   ├── interfaces.go            # All repository interfaces
│   │   │   ├── tenant_repository.go
│   │   │   ├── user_repository.go
│   │   │   ├── role_repository.go
│   │   │   ├── permission_repository.go
│   │   │   ├── session_repository.go
│   │   │   ├── refresh_token_repository.go
│   │   │   ├── password_reset_token_repository.go
│   │   │   ├── mfa_config_repository.go
│   │   │   ├── tenant_settings_repository.go
│   │   │   └── audit_log_repository.go
│   │   ├── routes/
│   │   │   └── routes.go
│   │   ├── services/
│   │   │   ├── tenant_service.go
│   │   │   ├── auth_service.go
│   │   │   ├── user_service.go
│   │   │   ├── role_service.go
│   │   │   ├── permission_service.go
│   │   │   ├── session_service.go
│   │   │   ├── token_service.go
│   │   │   └── audit_service.go
│   │   └── validators/
│   │       └── validators.go            # Custom validation functions
│   ├── shared/                          # Cross-module shared code
│   │   ├── cache/
│   │   │   └── redis.go
│   │   ├── config/
│   │   │   └── config.go
│   │   ├── constants/
│   │   │   └── constants.go
│   │   ├── database/
│   │   │   ├── base.go                  # BaseModel, TenantBaseModel
│   │   │   └── database.go             # Connection setup
│   │   ├── errors/
│   │   │   └── errors.go               # Shared error types
│   │   ├── logger/
│   │   │   └── logger.go
│   │   ├── middleware/
│   │   │   ├── cors.go
│   │   │   └── rate_limiter.go
│   │   ├── queue/
│   │   │   ├── publisher.go            # Interface
│   │   │   └── rabbitmq.go             # Implementation
│   │   └── utils/
│   │       └── validator.go
│   ├── organization/                    # Module 1 (future)
│   └── employee/                        # Module 2 (future)
├── api/
│   └── swagger.yaml                     # OpenAPI spec
├── tests/
│   ├── api/                             # API integration tests
│   ├── security/                        # Security tests
│   └── performance/                     # Load test scripts
└── go.mod
```

---

## Naming Conventions

### Files

| Type | Convention | Example |
|------|-----------|---------|
| Models | singular, snake_case | `tenant.go`, `user_role.go` |
| Controllers | singular + `_controller`, snake_case | `tenant_controller.go` |
| Services | singular + `_service`, snake_case | `auth_service.go` |
| Repositories | singular + `_repository`, snake_case | `user_repository.go` |
| DTOs | domain + `_dto`, snake_case | `tenant_dto.go` |
| Middleware | descriptive, snake_case | `rate_limiter.go` |
| Test files | original_name + `_test`, snake_case | `tenant_service_test.go` |

### Go Identifiers

| Type | Convention | Example |
|------|-----------|---------|
| Packages | lowercase, single word | `models`, `dto`, `services` |
| Exported types | PascalCase | `TenantService`, `CreateTenantRequest` |
| Unexported types | camelCase | `tenantServiceImpl` |
| Interfaces | PascalCase, describes behaviour | `TenantRepository`, `EventPublisher` |
| Constants | PascalCase for exported | `StatusActive`, `MaxPageSize` |
| JSON fields | snake_case (via struct tags) | `tenant_id`, `created_at` |
| Database columns | snake_case | `tenant_id`, `password_hash` |
| Table names | plural, snake_case | `tenants`, `user_roles` |
| Error variables | `Err` prefix, PascalCase | `ErrTenantNotFound`, `ErrInvalidCredentials` |

### API Routes

| Convention | Example |
|-----------|---------|
| Lowercase, hyphenated | `/forgot-password`, `/reset-password` |
| Plural resources | `/users`, `/roles`, `/tenants` |
| Nested resource actions | `/users/{id}/roles`, `/roles/{id}/permissions` |
| No trailing slashes | `/api/v1/users` (not `/api/v1/users/`) |

---

## Error Handling

### Error Types

Define domain-specific error types in `internal/shared/errors/`:

```go
type AppError struct {
    Code       string `json:"code"`
    Message    string `json:"message"`
    StatusCode int    `json:"-"`
}

func (e *AppError) Error() string {
    return e.Message
}
```

### Predefined Errors

```go
var (
    ErrValidation         = &AppError{Code: "VALIDATION_ERROR", Message: "validation failed", StatusCode: 400}
    ErrUnauthorized       = &AppError{Code: "UNAUTHORIZED", Message: "unauthorized", StatusCode: 401}
    ErrInvalidCredentials = &AppError{Code: "INVALID_CREDENTIALS", Message: "invalid credentials", StatusCode: 401}
    ErrForbidden          = &AppError{Code: "FORBIDDEN", Message: "forbidden", StatusCode: 403}
    ErrNotFound           = &AppError{Code: "NOT_FOUND", Message: "resource not found", StatusCode: 404}
    ErrConflict           = &AppError{Code: "CONFLICT", Message: "resource already exists", StatusCode: 409}
    ErrRateLimited        = &AppError{Code: "RATE_LIMITED", Message: "too many requests", StatusCode: 429}
    ErrInternal           = &AppError{Code: "INTERNAL_ERROR", Message: "internal server error", StatusCode: 500}
)
```

### Error Flow

```
Repository (returns error) →
  Service (wraps in AppError) →
    Controller (calls handleError) →
      Response (consistent JSON error format)
```

### Rules

1. **Never return raw Go errors** to the client. Always wrap in `AppError`.
2. **Never expose stack traces** in API responses. Log them internally.
3. **Never log passwords** or sensitive tokens.
4. **Always use `errors.Is()` and `errors.As()`** for error comparison, not string matching.
5. **Return early** on errors — avoid deep nesting.

---

## Logging

### Logger Setup

Use a structured logger (e.g., `zerolog` or `zap`). Configure in `internal/shared/logger/`.

### Log Levels

| Level | Usage |
|-------|-------|
| **DEBUG** | Detailed diagnostic info (DB queries, cache hits/misses). Disabled in production. |
| **INFO** | Normal operational events (server started, request handled, user created). |
| **WARN** | Unexpected but recoverable events (cache miss, retry attempt, rate limit approached). |
| **ERROR** | Failures that need attention (DB connection failed, event publish failed, unhandled error). |
| **FATAL** | Unrecoverable failures (cannot connect to DB on startup). Application will exit. |

### Required Log Fields

| Field | Source |
|-------|--------|
| `request_id` | X-Request-ID header or generated UUID |
| `tenant_id` | From context |
| `user_id` | From context |
| `method` | HTTP method |
| `path` | Request path |
| `status` | Response status code |
| `duration_ms` | Request processing time |
| `ip` | Client IP |

### Rules

1. **Never log passwords, tokens, or secrets.**
2. **Always include `request_id`** for request tracing.
3. **Log at the appropriate level** — do not use ERROR for expected failures like validation errors.
4. **Structured logging only** — no `fmt.Println` or `log.Println`.
5. **Log error details at the service layer**, not at the controller or repository layer.

---

## Transactions

### When to Use Transactions

Use database transactions when a single logical operation writes to multiple tables:

| Operation | Tables Affected |
|-----------|----------------|
| Create user with roles | `users`, `user_roles` |
| Delete role | `roles`, `user_roles`, `role_permissions` |
| Login | `sessions`, `refresh_tokens`, `users` (last_login_at) |
| Reset password | `users` (password_hash), `password_reset_tokens`, `sessions`, `refresh_tokens` |
| Deactivate user | `users`, `sessions`, `refresh_tokens` |

### Transaction Pattern

```go
func (s *userService) CreateUser(ctx context.Context, req dto.CreateUserRequest) (*dto.UserResponse, error) {
    tx := s.db.WithContext(ctx).Begin()
    defer func() {
        if r := recover(); r != nil {
            tx.Rollback()
        }
    }()

    // ... perform operations using tx ...

    if err := tx.Commit().Error; err != nil {
        return nil, err
    }

    // Publish events AFTER commit
    s.publisher.Publish(ctx, event)

    return response, nil
}
```

### Rules

1. **Publish events only after transaction commits**, never inside the transaction.
2. **Always defer rollback** as a safety net.
3. **Keep transactions short** — do not perform I/O (HTTP calls, queue operations) inside a transaction.
4. **Pass the `tx` (transaction handle)** to repository methods, not the raw `db`.

---

## Dependency Injection

### Pattern

Use **constructor injection**. Each service and controller receives its dependencies via its constructor function.

```go
// Service interface
type TenantService interface {
    Create(ctx context.Context, req dto.CreateTenantRequest) (*dto.TenantResponse, error)
    // ...
}

// Implementation
type tenantServiceImpl struct {
    tenantRepo  repositories.TenantRepository
    auditSvc    AuditService
    publisher   queue.EventPublisher
    logger      *zerolog.Logger
}

// Constructor
func NewTenantService(
    tenantRepo repositories.TenantRepository,
    auditSvc AuditService,
    publisher queue.EventPublisher,
    logger *zerolog.Logger,
) TenantService {
    return &tenantServiceImpl{
        tenantRepo: tenantRepo,
        auditSvc:   auditSvc,
        publisher:  publisher,
        logger:     logger,
    }
}
```

### Wiring

All dependencies are wired in the module's `module.go` file or the application bootstrap. No global state. No service locator pattern. No `init()` functions for dependency setup.

### Rules

1. **Depend on interfaces**, not concrete implementations.
2. **No circular dependencies** between services.
3. **No package-level mutable state** — all state lives in struct fields.
4. **No `init()` functions** for anything other than package-level constants or test setup.

---

## Repository Pattern

### Interface Definition

All repository interfaces are defined in `internal/identity/repositories/interfaces.go`:

```go
type TenantRepository interface {
    Create(ctx context.Context, tenant *models.Tenant) error
    FindByID(ctx context.Context, id uuid.UUID) (*models.Tenant, error)
    FindBySlug(ctx context.Context, slug string) (*models.Tenant, error)
    FindAll(ctx context.Context, page, perPage int) ([]models.Tenant, int64, error)
    Update(ctx context.Context, tenant *models.Tenant) error
    Delete(ctx context.Context, id uuid.UUID) error
}
```

### Implementation Rules

1. **Repositories accept and return domain models**, not DTOs.
2. **Repositories do not contain business logic** — only data access.
3. **All tenant-scoped queries must filter by tenant_id.**
4. **Use GORM's `.Preload()` for eager loading** relationships when needed.
5. **Support transaction injection** — methods should accept a `*gorm.DB` that can be either the main connection or a transaction.
6. **Return `nil, nil`** when a record is not found (not an error). The service layer decides if "not found" is an error.

Alternative: Return a sentinel error `ErrRecordNotFound` when not found. Choose one pattern and apply consistently.

---

## Service Layer

### Responsibilities

1. **Orchestrate business logic** — validate rules, coordinate repositories, publish events.
2. **Accept DTOs as input** and return DTOs as output.
3. **Manage transactions** when multiple repository calls are needed.
4. **Publish domain events** after successful state changes.
5. **Create audit log entries** for state-changing operations.

### Rules

1. **Services never access `*gin.Context`** — they receive plain Go `context.Context` and DTOs.
2. **Services never return HTTP status codes** — they return domain errors. The controller maps errors to HTTP responses.
3. **Services never format JSON responses** — that's the controller's job.
4. **Each service method performs one use case** — no god methods.

---

## DTO Usage

### Rules

1. **Request DTOs** are separate from **Response DTOs** — never reuse the same struct for both.
2. **DTOs are in the `dto` package** — never in `models`, `services`, or `controllers`.
3. **DTOs use `json:"field_name"` tags** with snake_case field names.
4. **Request DTOs use `validate:"..."` tags** for validation rules.
5. **Response DTOs never include sensitive fields** (password_hash, token_hash, etc.).
6. **Optional update fields use pointers** (`*string`, `*int`) to distinguish between "not provided" and "set to zero value."
7. **Mapping between models and DTOs** happens in the service layer or a dedicated mapper.

---

## Validation Standards

### Validation Tags

Use `go-playground/validator/v10` tags on request DTO structs:

| Tag | Usage |
|-----|-------|
| `required` | Field must be present and non-zero |
| `email` | Valid email format |
| `min=N` | Minimum string length or numeric value |
| `max=N` | Maximum string length or numeric value |
| `uuid` | Valid UUID format |
| `oneof=a b c` | Value must be one of the listed options |
| `omitempty` | Skip validation if field is empty/nil |
| `eqfield=FieldName` | Must equal another field (e.g., confirm_password) |

### Validation Flow

```
Request Body →
  Gin ShouldBindJSON (parses JSON) →
    Validator.ValidateStruct (validates tags) →
      If error: return 400 with field-level error details →
      If valid: pass DTO to service
```

### Custom Validators

Define custom validators in `internal/identity/validators/` for rules that cannot be expressed with tags:
- Tenant slug format (`^[a-z][a-z0-9-]{1,62}[a-z0-9]$`)
- Reserved slug check
- Password complexity (if added in v2)

---

## Code Review Checklist

Before approving any code for Module 0, verify:

### Architecture

- [ ] Code follows Clean Architecture — no layer violations
- [ ] Controller does not contain business logic
- [ ] Repository does not contain business logic
- [ ] Service does not access Gin context or HTTP concerns
- [ ] No circular dependencies between packages

### Security

- [ ] No plaintext passwords in code, logs, or responses
- [ ] All tenant-scoped queries filter by tenant_id
- [ ] No SQL string concatenation (use parameterized queries)
- [ ] Error responses do not leak internal details
- [ ] JWT claims are validated, not trusted blindly

### Data

- [ ] Transactions are used for multi-table writes
- [ ] Events are published after transaction commit
- [ ] Audit logs are created for state-changing operations
- [ ] Soft deletes are used where required
- [ ] Pagination is supported on all list endpoints

### Code Quality

- [ ] All exported symbols have doc comments
- [ ] Error handling follows the `AppError` pattern
- [ ] No `fmt.Println` or `log.Println` — use structured logger
- [ ] No `panic()` in production code paths
- [ ] No hardcoded secrets or configuration values
- [ ] Tests exist and pass
- [ ] No TODO comments

### Naming

- [ ] Files follow naming conventions
- [ ] Go identifiers follow naming conventions
- [ ] JSON fields use snake_case
- [ ] API routes follow conventions
- [ ] Error variables use `Err` prefix
