# Module 0 — Tenant + Identity + RBAC

## Purpose

Module 0 is the foundational identity platform for the JAAS multi-tenant SaaS system. It provides tenant lifecycle management, user identity, authentication, authorization, role-based access control (RBAC), session management, and audit logging. Every other module in the JAAS ecosystem depends on Module 0 for identity resolution, tenant isolation, and permission enforcement.

Module 0 is designed to be implemented first and in isolation. It has zero dependencies on any other JAAS module.

---

## Responsibilities

| Concern | Description |
|---------|-------------|
| **Tenant Management** | Create, activate, suspend, and query tenants. Each tenant represents an isolated customer organization (e.g., `acme.jaas.com`). |
| **User Management** | Create, update, deactivate, and invite users within a tenant. Users are always scoped to exactly one tenant. |
| **Authentication** | Validate user credentials, issue JWT access tokens, issue and rotate refresh tokens, and manage sessions. |
| **Authorization** | Enforce role-based access control on every API request. Permissions are checked against the authenticated user's roles. |
| **Role Management** | Define, assign, and revoke roles within a tenant. Roles are tenant-scoped. System roles cannot be deleted. |
| **Permission Management** | Define granular resource-action permissions. Permissions are global definitions; roles bind them to tenants. |
| **Session Management** | Track active user sessions with IP, user agent, and expiry. Support session revocation. |
| **Password Management** | Handle forgot-password and reset-password flows with secure, expiring tokens. |
| **MFA Configuration** | Define per-user multi-factor authentication configuration (TOTP, SMS, Email). |
| **Audit Logging** | Record every security-relevant action with actor, resource, metadata, IP, and timestamp. Audit logs are immutable. |

---

## Dependencies

### Internal Dependencies

Module 0 has **no internal dependencies**. It is the root of the JAAS dependency graph.

### External Services

| Service | Purpose | Protocol |
|---------|---------|----------|
| **PostgreSQL** | Primary persistent storage for all identity data | TCP/5432 |
| **Redis** | Session cache, rate limiting counters, token blacklist | TCP/6379 |
| **RabbitMQ** | Asynchronous domain event publishing | AMQP/5672 |
| **SMTP Provider** | Email delivery for password reset and user invitation flows | SMTP/587 |

### Downstream Dependents

The following modules depend on Module 0:

```
Module 1 (Organization)  → Tenant, User, RBAC
Module 2 (Employee)      → User, RBAC
Module 3 (Attendance)    → User, RBAC
Module 4 (Projects)      → User, RBAC
Module 5 (Meetings)      → User, RBAC
Module 6 (Approvals)     → User, RBAC
Module 7 (Notifications) → User
Module 8 (Assets)        → User, RBAC
Module 9 (Payroll)       → User, RBAC
Module 10 (Analytics)    → All
Module 11 (AI)           → All
Module 12 (IoT)          → User, RBAC
```

---

## Architecture Overview

Module 0 follows **Clean Architecture** with **Domain-Driven Design** principles:

```
┌──────────────────────────────────────────────────────┐
│                    HTTP Layer                         │
│   Routes → Middleware → Controllers                  │
├──────────────────────────────────────────────────────┤
│                  Application Layer                    │
│   Services (Use Cases) → DTOs → Validators           │
├──────────────────────────────────────────────────────┤
│                    Domain Layer                       │
│   Models → Events → Business Rules                   │
├──────────────────────────────────────────────────────┤
│                Infrastructure Layer                   │
│   Repositories → Database → Cache → Queue            │
└──────────────────────────────────────────────────────┘
```

**Key architectural decisions:**

1. **Shared database, tenant isolation via `tenant_id`** — All tenants share a single PostgreSQL instance. Every tenant-owned row carries a `tenant_id` foreign key. All queries are filtered by `tenant_id`.
2. **Subdomain-based routing** — Tenant resolution happens via subdomain (e.g., `acme.jaas.com`). The tenant middleware extracts the tenant context from the request.
3. **JWT + Refresh Token** — Short-lived access tokens (15 min) with long-lived refresh tokens (7 days). Refresh token rotation is mandatory.
4. **Event-driven side effects** — State changes publish domain events to RabbitMQ. Consumers handle notifications, cache invalidation, and cross-module sync.

---

## Technology Stack

| Layer | Technology | Version |
|-------|-----------|---------|
| Language | Go | 1.22+ |
| HTTP Router | Gin | v1.10+ |
| ORM | GORM | v1.25+ |
| Database | PostgreSQL | 15+ |
| Cache | Redis | 7+ |
| Message Queue | RabbitMQ | 3.12+ |
| Authentication | JWT (golang-jwt/jwt) | v5 |
| Password Hashing | bcrypt | cost 12 |
| UUID | google/uuid | v1 |
| Validation | go-playground/validator | v10 |
| API Documentation | OpenAPI 3.0 (Swagger) | — |
| Containerization | Docker | 24+ |
| Migration Tool | golang-migrate/migrate | v4 |

---

## Module Boundaries

### Module 0 Owns

- `tenants` table and all tenant lifecycle operations
- `users` table and all user lifecycle operations
- `roles` table and all role CRUD operations
- `permissions` table and permission definitions
- `user_roles` and `role_permissions` join tables
- `sessions` table and session lifecycle
- `refresh_tokens` table and token rotation
- `password_reset_tokens` table and reset flow
- `mfa_configs` table and MFA setup
- `tenant_settings` table and tenant configuration
- `audit_logs` table and audit trail
- All `/api/v1/tenants`, `/api/v1/auth`, `/api/v1/users`, `/api/v1/roles`, `/api/v1/permissions` endpoints
- All identity-related domain events

### Module 0 Does NOT Own

- Organization structure (departments, teams, designations) — Module 1
- Employee profiles (employment details, reporting hierarchy) — Module 2
- Business feature authorization (project-level, meeting-level) — respective modules
- Email template rendering — shared infrastructure
- API gateway routing — Kong (infrastructure)
- Infrastructure provisioning — DevOps

### Contracts Exposed to Other Modules

Module 0 exposes the following interfaces for consumption by other modules:

1. **User Resolution** — Given a `user_id` and `tenant_id`, return user details
2. **Permission Check** — Given a `user_id`, `tenant_id`, `resource`, and `action`, return allow/deny
3. **Tenant Resolution** — Given a `slug` or `tenant_id`, return tenant details and status
4. **Domain Events** — Published events that other modules can subscribe to (e.g., `UserCreated`, `TenantSuspended`)
