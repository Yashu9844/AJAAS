# Module 0 — System Architecture

## Component Overview

Module 0 is a self-contained identity platform composed of the following layers and components:

```
┌─────────────────────────────────────────────────────────────────────┐
│                          API Gateway (Kong)                         │
│                     TLS Termination + Routing                       │
└────────────────────────────────┬────────────────────────────────────┘
                                 │
                                 ▼
┌─────────────────────────────────────────────────────────────────────┐
│                        Go Application (Gin)                         │
│                                                                     │
│  ┌───────────────────────────────────────────────────────────────┐  │
│  │                     Middleware Stack                           │  │
│  │  CORS → RateLimit → TenantResolver → Auth → RBAC → Audit    │  │
│  └──────────────────────────┬────────────────────────────────────┘  │
│                              │                                      │
│  ┌──────────────────────────▼────────────────────────────────────┐  │
│  │                       Controllers                              │  │
│  │  TenantController │ AuthController │ UserController            │  │
│  │  RoleController   │ PermissionController                      │  │
│  └──────────────────────────┬────────────────────────────────────┘  │
│                              │                                      │
│  ┌──────────────────────────▼────────────────────────────────────┐  │
│  │                        Services                                │  │
│  │  TenantService │ AuthService │ UserService                     │  │
│  │  RoleService   │ PermissionService │ SessionService            │  │
│  │  AuditService  │ TokenService                                  │  │
│  └──────────────────────────┬────────────────────────────────────┘  │
│                              │                                      │
│  ┌──────────────────────────▼────────────────────────────────────┐  │
│  │                      Repositories                              │  │
│  │  TenantRepo │ UserRepo │ RoleRepo │ PermissionRepo             │  │
│  │  SessionRepo │ RefreshTokenRepo │ PasswordResetTokenRepo       │  │
│  │  MFAConfigRepo │ AuditLogRepo │ TenantSettingsRepo             │  │
│  └──────────────┬──────────────────────┬────────────────┬────────┘  │
│                  │                      │                │           │
│                  ▼                      ▼                ▼           │
│           ┌────────────┐       ┌──────────────┐  ┌────────────┐     │
│           │ PostgreSQL │       │    Redis      │  │  RabbitMQ  │     │
│           └────────────┘       └──────────────┘  └────────────┘     │
└─────────────────────────────────────────────────────────────────────┘
```

---

## Component Diagram

```mermaid
graph TB
    subgraph "External"
        Client["Client Application"]
        Kong["API Gateway (Kong)"]
        SMTP["SMTP Provider"]
    end

    subgraph "Module 0 - Identity Platform"
        subgraph "HTTP Layer"
            MW["Middleware Stack"]
            TC["Tenant Controller"]
            AC["Auth Controller"]
            UC["User Controller"]
            RC["Role Controller"]
            PC["Permission Controller"]
        end

        subgraph "Application Layer"
            TS["Tenant Service"]
            AS["Auth Service"]
            US["User Service"]
            RS["Role Service"]
            PS["Permission Service"]
            SS["Session Service"]
            TKS["Token Service"]
            ADS["Audit Service"]
        end

        subgraph "Domain Layer"
            Models["GORM Models"]
            Events["Domain Events"]
            Rules["Business Rules"]
        end

        subgraph "Infrastructure Layer"
            Repos["Repositories"]
            DB["PostgreSQL"]
            Cache["Redis"]
            Queue["RabbitMQ"]
        end
    end

    Client --> Kong
    Kong --> MW
    MW --> TC & AC & UC & RC & PC
    TC --> TS
    AC --> AS
    UC --> US
    RC --> RS
    PC --> PS
    AS --> SS & TKS
    TS & AS & US & RS & PS --> ADS
    TS & AS & US & RS & PS & SS & TKS & ADS --> Repos
    Repos --> DB
    SS --> Cache
    TKS --> Cache
    TS & AS & US & RS --> Queue
    Queue -.-> SMTP
```

---

## Internal Engine Diagram

### Middleware Pipeline

Every HTTP request passes through the middleware stack in order:

```mermaid
graph LR
    A["Incoming Request"] --> B["CORS Middleware"]
    B --> C["Rate Limiter"]
    C --> D["Request Logger"]
    D --> E["Tenant Resolver"]
    E --> F["JWT Auth"]
    F --> G["User Status Check"]
    G --> H["RBAC Permission Check"]
    H --> I["Audit Logger"]
    I --> J["Controller Handler"]
```

| Middleware | Responsibility | Skipped For |
|-----------|---------------|-------------|
| **CORS** | Set CORS headers | Never |
| **Rate Limiter** | Enforce request rate limits per IP and per user | Never |
| **Request Logger** | Log request method, path, duration, status code | Never |
| **Tenant Resolver** | Extract subdomain → resolve tenant → inject `tenant_id` into context | Tenant management endpoints (Super Admin) |
| **JWT Auth** | Extract and validate JWT from `Authorization` header | Public endpoints (login, forgot-password, reset-password) |
| **User Status Check** | Verify user is still `active` and not soft-deleted | Public endpoints |
| **RBAC Permission Check** | Verify user has the required `resource:action` permission | Public endpoints, self-service endpoints |
| **Audit Logger** | Record the request in the audit log (post-handler, after response) | Read-only GET endpoints (configurable) |

### Tenant Resolution Flow

```mermaid
sequenceDiagram
    participant C as Client
    participant M as Tenant Middleware
    participant DB as PostgreSQL

    C->>M: Request to acme.jaas.com/api/v1/users
    M->>M: Extract subdomain "acme" from Host header
    M->>DB: SELECT * FROM tenants WHERE slug = 'acme' AND deleted_at IS NULL
    alt Tenant Found & Active
        M->>M: Set tenant_id in request context
        M->>C: Continue to next middleware
    else Tenant Not Found
        M->>C: 404 "tenant not found"
    else Tenant Suspended
        M->>C: 403 "tenant is suspended"
    end
```

---

## Service Interactions

### Service Dependency Map

```mermaid
graph TD
    AuthService --> UserRepo
    AuthService --> SessionService
    AuthService --> TokenService
    AuthService --> AuditService
    AuthService --> TenantRepo

    UserService --> UserRepo
    UserService --> RoleRepo
    UserService --> SessionService
    UserService --> AuditService

    TenantService --> TenantRepo
    TenantService --> TenantSettingsRepo
    TenantService --> AuditService

    RoleService --> RoleRepo
    RoleService --> PermissionRepo
    RoleService --> AuditService

    PermissionService --> PermissionRepo

    SessionService --> SessionRepo
    SessionService --> Cache["Redis Cache"]

    TokenService --> RefreshTokenRepo
    TokenService --> PasswordResetTokenRepo
    TokenService --> Cache["Redis Cache"]
```

### Key Service Responsibilities

| Service | Responsibility |
|---------|---------------|
| **AuthService** | Orchestrates login, logout, refresh, forgot-password, reset-password. Coordinates between UserRepo, SessionService, TokenService. |
| **UserService** | User CRUD, invitation, deactivation. Validates role assignments. |
| **TenantService** | Tenant CRUD, activation, suspension. Manages tenant settings. |
| **RoleService** | Role CRUD. Manages role-permission and user-role assignments. |
| **PermissionService** | Read-only permission listing. |
| **SessionService** | Session creation, validation, revocation. Uses Redis for fast lookups. |
| **TokenService** | Refresh token and password reset token generation, validation, rotation. |
| **AuditService** | Audit log creation. Accepts audit entries from all other services. |

---

## Request Flow

### Standard Authenticated Request Flow

```mermaid
sequenceDiagram
    participant C as Client
    participant GW as Kong Gateway
    participant MW as Middleware
    participant CT as Controller
    participant SV as Service
    participant RP as Repository
    participant DB as PostgreSQL
    participant RD as Redis
    participant MQ as RabbitMQ

    C->>GW: HTTPS Request
    GW->>MW: HTTP Request (TLS terminated)
    MW->>MW: Rate limit check
    MW->>MW: Tenant resolution (subdomain → tenant_id)
    MW->>MW: JWT validation
    MW->>RD: Check session validity (cache)
    MW->>MW: User status check
    MW->>MW: RBAC permission check
    MW->>CT: Authenticated request with context
    CT->>CT: Validate request DTO
    CT->>SV: Call service method
    SV->>RP: Database operation
    RP->>DB: SQL query
    DB->>RP: Result
    RP->>SV: Domain model
    SV->>SV: Apply business rules
    SV->>RP: Persist changes (in transaction)
    SV->>MQ: Publish domain event (async)
    SV->>CT: Response DTO
    CT->>MW: HTTP Response
    MW->>MW: Audit log (post-handler)
    MW->>C: JSON Response
```

---

## Authentication Flow

```mermaid
sequenceDiagram
    participant C as Client
    participant CT as AuthController
    participant AS as AuthService
    participant TR as TenantRepo
    participant UR as UserRepo
    participant SS as SessionService
    participant TS as TokenService
    participant AL as AuditService
    participant MQ as RabbitMQ

    C->>CT: POST /api/v1/auth/login {email, password, tenant_slug}
    CT->>CT: Validate request DTO
    CT->>AS: Login(email, password, tenant_slug)

    AS->>TR: FindBySlug(tenant_slug)
    alt Tenant not found or not active
        AS->>AL: Log failed attempt
        AS->>CT: Error: invalid credentials
        CT->>C: 401 Unauthorized
    end

    AS->>UR: FindByEmail(tenant_id, email)
    alt User not found
        AS->>AL: Log failed attempt
        AS->>CT: Error: invalid credentials
        CT->>C: 401 Unauthorized
    end

    AS->>AS: Compare bcrypt hash
    alt Password mismatch
        AS->>AL: Log failed attempt
        AS->>CT: Error: invalid credentials
        CT->>C: 401 Unauthorized
    end

    AS->>AS: Check user status is active
    alt User not active
        AS->>AL: Log failed attempt
        AS->>CT: Error: account is not active
        CT->>C: 403 Forbidden
    end

    AS->>SS: CreateSession(user_id, tenant_id, ip, user_agent)
    AS->>TS: CreateRefreshToken(user_id, tenant_id)
    AS->>AS: Generate JWT (user_id, tenant_id, email, roles)
    AS->>UR: Update last_login_at
    AS->>AL: Log successful login
    AS->>MQ: Publish UserLoggedIn event

    AS->>CT: LoginResponse {access_token, refresh_token, expires_at, user}
    CT->>C: 200 OK
```

---

## Authorization Flow

```mermaid
sequenceDiagram
    participant MW as RBAC Middleware
    participant DB as PostgreSQL

    MW->>MW: Extract user_id and tenant_id from JWT context
    MW->>MW: Determine required permission (resource:action) from route

    MW->>DB: SELECT p.resource, p.action FROM permissions p<br/>JOIN role_permissions rp ON p.id = rp.permission_id<br/>JOIN user_roles ur ON rp.role_id = ur.role_id<br/>WHERE ur.user_id = ? AND ur.tenant_id = ?

    alt User has required permission
        MW->>MW: Continue to controller
    else User lacks permission
        MW->>MW: Return 403 Forbidden
    end
```

---

## Dependency Graph

```mermaid
graph TD
    subgraph "Module 0"
        Identity["Identity Module"]
    end

    subgraph "Infrastructure"
        PG["PostgreSQL"]
        RD["Redis"]
        MQ["RabbitMQ"]
        GW["Kong API Gateway"]
    end

    subgraph "Dependent Modules"
        M1["Module 1: Organization"]
        M2["Module 2: Employee"]
        M3["Module 3: Attendance"]
        M4["Module 4: Projects"]
        M5["Module 5: Meetings"]
        M6["Module 6: Approvals"]
        M7["Module 7: Notifications"]
    end

    Identity --> PG
    Identity --> RD
    Identity --> MQ
    GW --> Identity

    M1 -.-> Identity
    M2 -.-> Identity
    M3 -.-> Identity
    M4 -.-> Identity
    M5 -.-> Identity
    M6 -.-> Identity
    M7 -.-> Identity
```

---

## Database Interaction

### Connection Management

- Use GORM's built-in connection pooling
- Maximum open connections: 25
- Maximum idle connections: 10
- Connection max lifetime: 5 minutes
- Connection max idle time: 1 minute

### Transaction Strategy

| Operation | Transaction Required | Reason |
|-----------|---------------------|--------|
| Create User with Roles | Yes | Writes to `users` and `user_roles` |
| Delete Role | Yes | Deletes from `roles`, `user_roles`, and `role_permissions` |
| Login | Yes | Creates `session`, creates `refresh_token`, updates `last_login_at` |
| Reset Password | Yes | Updates `password_hash`, revokes all sessions and refresh tokens, marks token used |
| Deactivate User | Yes | Updates user status, revokes all sessions and refresh tokens |
| Suspend Tenant | No | Single row update |
| Create Audit Log | No | Single insert, best-effort |

### Query Patterns

All tenant-scoped queries MUST include `WHERE tenant_id = ?` to enforce isolation:

```sql
-- CORRECT: Tenant-scoped query
SELECT * FROM users WHERE tenant_id = $1 AND email = $2 AND deleted_at IS NULL

-- WRONG: Missing tenant isolation
SELECT * FROM users WHERE email = $1 AND deleted_at IS NULL
```

---

## Cache Interaction (Redis)

### Cache Strategy

| Data | Cache Key Pattern | TTL | Strategy |
|------|------------------|-----|----------|
| Session validity | `session:{session_id}` | 24 hours | Write-through on create, invalidate on revoke |
| User permissions | `perms:{user_id}:{tenant_id}` | 5 minutes | Cache-aside, invalidate on role/permission change |
| Rate limit counters | `rate:{ip}:{endpoint}` | 15 minutes | Sliding window counter |
| Token blacklist | `blacklist:{token_jti}` | 15 minutes (matches JWT TTL) | Write on logout/revoke |

### Cache Invalidation

| Event | Invalidated Keys |
|-------|-----------------|
| Logout | `session:{session_id}`, `blacklist:{jti}` (add) |
| User Deactivate | `session:{all_user_sessions}`, `perms:{user_id}:*` |
| Role Assign/Revoke | `perms:{user_id}:{tenant_id}` |
| Permission Assign/Revoke | `perms:{all_users_with_role}:{tenant_id}` |
| Tenant Suspend | `session:{all_tenant_sessions}` |

---

## Queue Interaction (RabbitMQ)

### Exchange Configuration

| Exchange | Type | Durable | Purpose |
|----------|------|---------|---------|
| `jaas.identity.events` | Topic | Yes | All Module 0 domain events |

### Queue Bindings

| Queue | Routing Key Pattern | Consumer |
|-------|-------------------|----------|
| `jaas.notifications.identity` | `identity.user.*` | Module 7 (Notifications) |
| `jaas.audit.identity` | `identity.#` | Audit archival service |
| `jaas.org.identity` | `identity.tenant.*`, `identity.user.created` | Module 1 (Organization) |

### Publishing Pattern

Events are published asynchronously after the database transaction commits. If RabbitMQ is unavailable:
1. Log the failure with full event payload
2. Store the failed event in a `failed_events` table (dead letter persistence)
3. A background worker retries failed events with exponential backoff

---

## Future Scalability Considerations

### Horizontal Scaling

- Module 0 is stateless (sessions in Redis, data in PostgreSQL). Multiple instances can run behind a load balancer.
- JWT validation is self-contained (no database hit for token verification itself), enabling high throughput.

### Database Scaling

- Read replicas for user/role/permission queries
- Partitioned audit logs by month for efficient querying and archival
- Connection pooling via PgBouncer for high-concurrency scenarios

### Cache Scaling

- Redis Cluster for high availability
- Separate Redis instance for rate limiting vs. session caching

### Event Processing

- Multiple queue consumers for parallel event processing
- RabbitMQ clustering for message durability
- Dead letter queues for poison message handling
