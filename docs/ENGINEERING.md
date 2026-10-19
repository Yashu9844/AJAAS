# JAAS Engineering Guide

How work is done in this repository. Replaces the former `MASTER_PROMPT.md`, `SERVICE_LOOP.md`, `CONTRIBUTING.md`
and the M0 `coding-guidelines.md` (merged as Appendix A). Architecture: `docs/ARCHITECTURE.md`. Security law and
code style: `docs/SECURITY.md`.

## 1. Sources of truth

Precedence when sources disagree: **code > executable tests > frozen specifications > plans/status > architecture
narrative > other docs > changelog**. If code contradicts a specification, flag it in the module's `plan.md` and
resolve it explicitly — never pick one silently. Docs never claim behaviour that is not implemented and tested.

## 2. Setup

```bash
docker compose up -d postgres redis rabbitmq          # or: docker start ajaas-postgres-1 ajaas-redis-1 ajaas-rabbitmq-1
cd backend && export PATH="$HOME/go-sdk/go/bin:$PATH"
APP_ENV=development DATABASE_PASSWORD=postgres DATABASE_DBNAME=jaas_dev go run ./cmd   # applies SQL migrations, serves :8080
cd frontend && npm install && npm run dev               # :3000, test console at /dev-test
```

Tenant APIs need the tenant subdomain host (`<slug>.localhost:8080`). Platform routes need `X-Platform-Key`.
Test-console details: `docs/TEST_CONSOLE.md`.

## 3. Units of work

- **Module** = one of M0–M12 (`docs/modules/mNN-*`). **Context** = a bounded context inside a module (M1 HR has
  several). Contexts follow the same delivery phases as modules.
- Delivery phases for a module or context:

| Phase | Output |
|---|---|
| P1 Design | frozen `specification.md` (FR/rule IDs, API, errors, data, edge cases, out of scope), `design.md`, goldens in `testing.md`, decisions; human gate |
| P2 Data | models, SQL migrations (up/down), up→down→up cycle and AutoMigrate↔SQL parity |
| P3 Engines | pure calculation/state engines, DTOs, validators (100% coverage) |
| P4 Repositories | tenant-scoped repositories, locks, constraints |
| P5 Services | use cases, transactions, outbox events, relay, ports to other contexts/modules |
| P6 HTTP | controllers, routes with permission chains, wiring, OpenAPI |
| P7 Integration | in-process integration tests on real Postgres/Redis, goldens, exact-grant RBAC, isolation |
| P8 Test console | console page + scenarios for every endpoint and golden (`docs/TEST_CONSOLE.md`) |
| P9 Done | Definition of Done (§7), docs updated, `SYSTEM_REFERENCE` regenerated |

## 4. The loop (one tick = one checklist item)

```text
ANALYZE   read the item and only the spec pages it needs
BLUEPRINT short file plan; STOP for a human gate if it touches schema, public API, frontend contract,
          several modules, secrets, or anything irreversible
RED       failing test first; confirm it fails for the right reason
GREEN     minimal code until it passes — this item only
REFACTOR  names, one-line docs on exports, size limits (§9)
REVIEW    re-read the diff as a critic against docs/SECURITY.md §14
INTEGRATE boot the backend and exercise the endpoint; update the console page against the live API
NEXT      tick the item, update docs (§8), commit, report; then the next item
```

Budget: the same error three times → mark the item BLOCKED with the evidence in `plan.md`, commit nothing, stop.
Never batch two items in one tick. Never report a test as passing without running it.

## 5. Human gates

Schema migrations, public API changes, backend↔frontend contract changes, changes spanning several modules or
contexts, production secrets, irreversible operations. Everything else proceeds on documented defaults and is
reported at the end of the tick.

## 6. Sensors (must pass before an item is done)

- Backend: `go build ./...`, `go vet ./...`, `gofmt -l` clean (strip CRLF first — the repo uses `core.autocrlf=true`),
  unit tests, coverage ≥ 80% overall and ≥ 90% services (100% engines/validators/DTOs/models), no `TODO`, migrations
  up/down/up clean, integration ring (`JAAS_REQUIRE_INTEGRATION=1`).
- `go test -race` needs cgo, which the main dev host lacks; concurrency is proven with live parallel tests instead.
- Frontend: `tsc --noEmit`, `eslint`, `next build`.
- Performance (p95): single GET < 200 ms, list < 500 ms, login < 300 ms, DB query < 50 ms.

## 7. Definition of Done

```text
[ ] Requirements cited (FR/rule IDs in code comments and tests)  [ ] Dependencies checked (design.md connections)
[ ] Implementation complete for the agreed scope                   [ ] Code limits (§9) and SECURITY.md §13
[ ] Unit ring                                                       [ ] Integration ring
[ ] Contract (routes ↔ permissions ↔ OpenAPI)                       [ ] Goldens
[ ] Security (threat → evidence table filled)                       [ ] Dependent modules/contexts green
[ ] Test console page + scenarios pass live                         [ ] Docs updated (§8), SYSTEM_REFERENCE regenerated
[ ] No unrelated changes, diff reviewed, no secrets                 [ ] ARCHITECTURE.md §9 checklist items cited
```

## 8. Documentation standard

| Bundle | Files |
|---|---|
| Module in design or built | `README.md`, `specification.md`, `design.md`, `testing.md`, `plan.md`, `decisions.md`, `model/` |
| Module not started | `README.md`, `specification.md` (proposed), `model/` |
| Context (inside a module) | `contexts/<name>/specification.md`, `design.md`, `testing.md`, `decisions.md`; status and plan live in the module's `plan.md` |

Update in the same task as the code: `plan.md` (status, step ticked, gaps), `specification.md`/`design.md` when
behaviour or structure changes, `testing.md` when tests or goldens change, `decisions.md` for every decision
(decision — reason — alternatives — trade-off) and a dated changelog line. Regenerate `docs/SYSTEM_REFERENCE.md`
Part 2 with `python scripts/gen_reference.py <repo> <out>`; never hand-edit it.

**Golden tests** are protected: never edited to make a red test pass. Changing one needs a reason, a specification
update, a recorded decision and a full re-run of dependent suites.

## 9. Code limits

Function ≤ 50 lines, file ≤ 300 lines, nesting depth ≤ 4, parameters ≤ 4 (group into a struct otherwise); no `panic`
in production code, no `TODO`, no `nolint`/`eslint-disable`, no secrets; `tenant_id` on every scoped query; one-line
doc comment on every exported identifier. Details: `docs/SECURITY.md` §13 and Appendix A.

## 10. Ownership, cross-module changes and git

- Stay inside your module/context: `backend/internal/<module>/**`, `frontend/src/modules/<module>/**`,
  `docs/modules/<mNN>/**`. `backend/internal/shared/**` changes need a decision entry and dependent test runs.
- Cross-module or cross-context needs: use the provider's published contract; if it is insufficient, change it
  backward-compatibly, update provider tests, then the consumer, run both suites, record the change in both
  `decisions.md` files. A silent behaviour change is a defect.
- Failure handling: classify before editing — implementation, test, environment (DB/Redis/queue down), dependency
  (contract changed), configuration, external. Fix the class that failed, not the nearest code.
- Commits: one module per commit, `type(scope): summary`, e.g. `feat(hr-leave): ...`, `fix(m0): ...`,
  `docs(architecture): ...`, `test(hr-payroll): ...`. Small, reviewable, no unrelated formatting. Never commit to
  `main` directly; branch per phase.

## 11. Handoff block (end of every task)

```text
Completed — what + phase/step
Files changed — paths
Tests — commands run, pass/fail counts
Cross-module impact — none | contracts changed + dependents run
Docs updated — files
Remaining issues — risks, blocked items
Next — next step and its first actions
```

---

## Appendix A — Coding guidelines (from the former M0 bundle; apply to every Go module)

> Written for M0; the patterns apply everywhere. Two refinements since: (1) events are written to an **outbox row
> inside the transaction** and published after commit by a relay (`docs/ARCHITECTURE.md` §3.7) — "publish after
> commit" below means exactly that, never a direct best-effort publish; (2) services take a `TxRunner` port instead
> of beginning transactions on a raw `*gorm.DB`. The folder tree below is M0's; HR's layout is in
> `docs/modules/m1-hr-management/design.md` §2.

These guidelines apply to all code written within the M0 Identity identity module. They establish the conventions that every developer (human or AI) must follow.

---

### Folder Structure

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
│   ├── identity/                        # M0 Identity
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

### Naming Conventions

#### Files

| Type | Convention | Example |
|------|-----------|---------|
| Models | singular, snake_case | `tenant.go`, `user_role.go` |
| Controllers | singular + `_controller`, snake_case | `tenant_controller.go` |
| Services | singular + `_service`, snake_case | `auth_service.go` |
| Repositories | singular + `_repository`, snake_case | `user_repository.go` |
| DTOs | domain + `_dto`, snake_case | `tenant_dto.go` |
| Middleware | descriptive, snake_case | `rate_limiter.go` |
| Test files | original_name + `_test`, snake_case | `tenant_service_test.go` |

#### Go Identifiers

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

#### API Routes

| Convention | Example |
|-----------|---------|
| Lowercase, hyphenated | `/forgot-password`, `/reset-password` |
| Plural resources | `/users`, `/roles`, `/tenants` |
| Nested resource actions | `/users/{id}/roles`, `/roles/{id}/permissions` |
| No trailing slashes | `/api/v1/users` (not `/api/v1/users/`) |

---

### Error Handling

#### Error Types

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

#### Predefined Errors

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

#### Error Flow

```
Repository (returns error) →
  Service (wraps in AppError) →
    Controller (calls handleError) →
      Response (consistent JSON error format)
```

#### Rules

1. **Never return raw Go errors** to the client. Always wrap in `AppError`.
2. **Never expose stack traces** in API responses. Log them internally.
3. **Never log passwords** or sensitive tokens.
4. **Always use `errors.Is()` and `errors.As()`** for error comparison, not string matching.
5. **Return early** on errors — avoid deep nesting.

---

### Logging

#### Logger Setup

Use a structured logger (e.g., `zerolog` or `zap`). Configure in `internal/shared/logger/`.

#### Log Levels

| Level | Usage |
|-------|-------|
| **DEBUG** | Detailed diagnostic info (DB queries, cache hits/misses). Disabled in production. |
| **INFO** | Normal operational events (server started, request handled, user created). |
| **WARN** | Unexpected but recoverable events (cache miss, retry attempt, rate limit approached). |
| **ERROR** | Failures that need attention (DB connection failed, event publish failed, unhandled error). |
| **FATAL** | Unrecoverable failures (cannot connect to DB on startup). Application will exit. |

#### Required Log Fields

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

#### Rules

1. **Never log passwords, tokens, or secrets.**
2. **Always include `request_id`** for request tracing.
3. **Log at the appropriate level** — do not use ERROR for expected failures like validation errors.
4. **Structured logging only** — no `fmt.Println` or `log.Println`.
5. **Log error details at the service layer**, not at the controller or repository layer.

---

### Transactions

#### When to Use Transactions

Use database transactions when a single logical operation writes to multiple tables:

| Operation | Tables Affected |
|-----------|----------------|
| Create user with roles | `users`, `user_roles` |
| Delete role | `roles`, `user_roles`, `role_permissions` |
| Login | `sessions`, `refresh_tokens`, `users` (last_login_at) |
| Reset password | `users` (password_hash), `password_reset_tokens`, `sessions`, `refresh_tokens` |
| Deactivate user | `users`, `sessions`, `refresh_tokens` |

#### Transaction Pattern

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

#### Rules

1. **Publish events only after transaction commits**, never inside the transaction.
2. **Always defer rollback** as a safety net.
3. **Keep transactions short** — do not perform I/O (HTTP calls, queue operations) inside a transaction.
4. **Pass the `tx` (transaction handle)** to repository methods, not the raw `db`.

---

### Dependency Injection

#### Pattern

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

#### Wiring

All dependencies are wired in the module's `module.go` file or the application bootstrap. No global state. No service locator pattern. No `init()` functions for dependency setup.

#### Rules

1. **Depend on interfaces**, not concrete implementations.
2. **No circular dependencies** between services.
3. **No package-level mutable state** — all state lives in struct fields.
4. **No `init()` functions** for anything other than package-level constants or test setup.

---

### Repository Pattern

#### Interface Definition

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

#### Implementation Rules

1. **Repositories accept and return domain models**, not DTOs.
2. **Repositories do not contain business logic** — only data access.
3. **All tenant-scoped queries must filter by tenant_id.**
4. **Use GORM's `.Preload()` for eager loading** relationships when needed.
5. **Support transaction injection** — methods should accept a `*gorm.DB` that can be either the main connection or a transaction.
6. **Return `nil, nil`** when a record is not found (not an error). The service layer decides if "not found" is an error.

Alternative: Return a sentinel error `ErrRecordNotFound` when not found. Choose one pattern and apply consistently.

---

### Service Layer

#### Responsibilities

1. **Orchestrate business logic** — validate rules, coordinate repositories, publish events.
2. **Accept DTOs as input** and return DTOs as output.
3. **Manage transactions** when multiple repository calls are needed.
4. **Publish domain events** after successful state changes.
5. **Create audit log entries** for state-changing operations.

#### Rules

1. **Services never access `*gin.Context`** — they receive plain Go `context.Context` and DTOs.
2. **Services never return HTTP status codes** — they return domain errors. The controller maps errors to HTTP responses.
3. **Services never format JSON responses** — that's the controller's job.
4. **Each service method performs one use case** — no god methods.

---

### DTO Usage

#### Rules

1. **Request DTOs** are separate from **Response DTOs** — never reuse the same struct for both.
2. **DTOs are in the `dto` package** — never in `models`, `services`, or `controllers`.
3. **DTOs use `json:"field_name"` tags** with snake_case field names.
4. **Request DTOs use `validate:"..."` tags** for validation rules.
5. **Response DTOs never include sensitive fields** (password_hash, token_hash, etc.).
6. **Optional update fields use pointers** (`*string`, `*int`) to distinguish between "not provided" and "set to zero value."
7. **Mapping between models and DTOs** happens in the service layer or a dedicated mapper.

---

### Validation Standards

#### Validation Tags

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

#### Validation Flow

```
Request Body →
  Gin ShouldBindJSON (parses JSON) →
    Validator.ValidateStruct (validates tags) →
      If error: return 400 with field-level error details →
      If valid: pass DTO to service
```

#### Custom Validators

Define custom validators in `internal/identity/validators/` for rules that cannot be expressed with tags:
- Tenant slug format (`^[a-z][a-z0-9-]{1,62}[a-z0-9]$`)
- Reserved slug check
- Password complexity (if added in v2)

---

### Code Review Checklist

Before approving any code for M0 Identity, verify:

#### Architecture

- [ ] Code follows Clean Architecture — no layer violations
- [ ] Controller does not contain business logic
- [ ] Repository does not contain business logic
- [ ] Service does not access Gin context or HTTP concerns
- [ ] No circular dependencies between packages

#### Security

- [ ] No plaintext passwords in code, logs, or responses
- [ ] All tenant-scoped queries filter by tenant_id
- [ ] No SQL string concatenation (use parameterized queries)
- [ ] Error responses do not leak internal details
- [ ] JWT claims are validated, not trusted blindly

#### Data

- [ ] Transactions are used for multi-table writes
- [ ] Events are published after transaction commit
- [ ] Audit logs are created for state-changing operations
- [ ] Soft deletes are used where required
- [ ] Pagination is supported on all list endpoints

#### Code Quality

- [ ] All exported symbols have doc comments
- [ ] Error handling follows the `AppError` pattern
- [ ] No `fmt.Println` or `log.Println` — use structured logger
- [ ] No `panic()` in production code paths
- [ ] No hardcoded secrets or configuration values
- [ ] Tests exist and pass
- [ ] No TODO comments

#### Naming

- [ ] Files follow naming conventions
- [ ] Go identifiers follow naming conventions
- [ ] JSON fields use snake_case
- [ ] API routes follow conventions
- [ ] Error variables use `Err` prefix
