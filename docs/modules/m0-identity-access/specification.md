# M0 Identity & Access — Specification

> Design-time contract for M0 Identity & Access, merged from the former 26-file module-0 bundle. Where it differs from code, the code and `docs/SYSTEM_REFERENCE.md` (generated) win; known differences and target-architecture gaps are tracked in `plan.md`.

## Requirements

### Business Requirements

#### BR-001: Multi-Tenant Platform

JAAS must support multiple customer organizations (tenants) on a shared infrastructure. Each tenant operates in complete data isolation. A tenant is identified by a unique subdomain (e.g., `acme.jaas.com`).

#### BR-002: Self-Service Tenant Onboarding

The JAAS Super Admin must be able to create new tenants. Each tenant receives a unique slug used for subdomain routing. Tenants can be activated, suspended, or deactivated.

#### BR-003: User Identity Management

Each tenant must have its own set of users. Users are identified by email within their tenant. A Tenant Admin can create, invite, update, and deactivate users.

#### BR-004: Secure Authentication

Users must authenticate with email and password. The system must support session tracking, token-based authentication (JWT), and refresh token rotation. Multi-factor authentication must be configurable per user.

#### BR-005: Role-Based Access Control

Access to platform features is governed by roles and permissions. Roles are tenant-scoped. Permissions are globally defined resource-action pairs. Users are assigned roles; roles are assigned permissions.

#### BR-006: Audit Compliance

All security-relevant actions must be recorded in an immutable audit log. The audit trail must capture who did what, when, from where, and on which resource.

#### BR-007: Password Security

Users must be able to reset their password via a secure, time-limited token. Passwords must meet minimum complexity requirements.

---

### Functional Requirements

#### Tenant Management

| ID | Requirement |
|----|-------------|
| FR-T001 | The system shall allow creation of a tenant with name, slug, optional domain, and plan. |
| FR-T002 | The system shall enforce unique slugs across all tenants. |
| FR-T003 | The system shall enforce unique domains across all tenants (when provided). |
| FR-T004 | The system shall allow retrieval of a single tenant by ID. |
| FR-T005 | The system shall allow retrieval of all tenants with pagination. |
| FR-T006 | The system shall allow partial update of tenant name, domain, and plan. |
| FR-T007 | The system shall allow activation of a suspended tenant. |
| FR-T008 | The system shall allow suspension of an active tenant. |
| FR-T009 | The system shall support soft deletion of tenants. |
| FR-T010 | The system shall support tenant-scoped settings as key-value pairs. |

#### Authentication

| ID | Requirement |
|----|-------------|
| FR-A001 | The system shall authenticate a user by email, password, and tenant slug. |
| FR-A002 | The system shall issue a JWT access token upon successful authentication. |
| FR-A003 | The system shall issue a refresh token upon successful authentication. |
| FR-A004 | The system shall create a session record upon successful authentication. |
| FR-A005 | The system shall allow a user to log out, revoking their current session and refresh token. |
| FR-A006 | The system shall allow access token renewal using a valid refresh token. |
| FR-A007 | The system shall rotate the refresh token upon each renewal (issue a new one, revoke the old one). |
| FR-A008 | The system shall initiate a password reset by sending a time-limited token to the user's email. |
| FR-A009 | The system shall allow a user to set a new password using a valid, unused password reset token. |
| FR-A010 | The system shall reject authentication for users belonging to suspended tenants. |
| FR-A011 | The system shall reject authentication for inactive, suspended, or invited users. |

#### User Management

| ID | Requirement |
|----|-------------|
| FR-U001 | The system shall allow creation of a user within a tenant with email, password, first name, last name, and optional role assignments. |
| FR-U002 | The system shall enforce unique email addresses within a tenant. |
| FR-U003 | The system shall allow retrieval of a single user by ID within the authenticated tenant. |
| FR-U004 | The system shall allow retrieval of all users within the authenticated tenant with pagination. |
| FR-U005 | The system shall allow partial update of user profile (first name, last name, phone, avatar URL). |
| FR-U006 | The system shall allow deactivation of a user, revoking all active sessions. |
| FR-U007 | The system shall allow invitation of a user by email, creating a user record with status `invited`. |
| FR-U008 | The system shall hash passwords before storage. Plaintext passwords must never be persisted. |

#### Role Management

| ID | Requirement |
|----|-------------|
| FR-R001 | The system shall allow creation of a role within a tenant with name, description, and optional permission assignments. |
| FR-R002 | The system shall enforce unique role names within a tenant. |
| FR-R003 | The system shall allow retrieval of all roles within the authenticated tenant with pagination. |
| FR-R004 | The system shall allow partial update of role name and description. |
| FR-R005 | The system shall allow deletion of non-system roles. |
| FR-R006 | The system shall prevent deletion of system roles. |
| FR-R007 | The system shall allow assignment of permissions to a role. |
| FR-R008 | The system shall allow assignment of roles to a user. |

#### Permission Management

| ID | Requirement |
|----|-------------|
| FR-P001 | The system shall provide a list of all available permissions. |
| FR-P002 | Permissions shall be defined as resource-action pairs (e.g., `users:create`). |
| FR-P003 | Permissions are globally defined and not tenant-scoped. |

#### Session Management

| ID | Requirement |
|----|-------------|
| FR-S001 | The system shall create a session upon login, recording IP address, user agent, and expiry. |
| FR-S002 | The system shall support session revocation on logout. |
| FR-S003 | The system shall revoke all sessions when a user is deactivated. |
| FR-S004 | The system shall enforce session expiry. Expired sessions cannot be used for authentication. |

#### Audit Logging

| ID | Requirement |
|----|-------------|
| FR-AL001 | The system shall record audit logs for all state-changing operations. |
| FR-AL002 | Audit logs shall be immutable — no updates or deletes. |
| FR-AL003 | Audit logs shall include tenant_id, user_id, action, resource, resource_id, metadata, IP address, user agent, and timestamp. |

---

### Non-Functional Requirements

#### Performance

| ID | Requirement | Target |
|----|-------------|--------|
| NFR-P001 | API response time for single-resource endpoints | < 200ms (p95) |
| NFR-P002 | API response time for list endpoints | < 500ms (p95) |
| NFR-P003 | Authentication (login) response time | < 300ms (p95) |
| NFR-P004 | Database query execution time | < 50ms (p95) |
| NFR-P005 | Concurrent users per tenant | 1,000+ |

#### Scalability

| ID | Requirement |
|----|-------------|
| NFR-S001 | The system shall support 10,000+ tenants on shared infrastructure. |
| NFR-S002 | The system shall support 1,000,000+ total users across all tenants. |
| NFR-S003 | Audit log storage shall scale independently of transactional data. |

#### Security

| ID | Requirement |
|----|-------------|
| NFR-SEC001 | Passwords shall be hashed using bcrypt with a cost factor of 12. |
| NFR-SEC002 | JWT access tokens shall expire after 15 minutes. |
| NFR-SEC003 | Refresh tokens shall expire after 7 days. |
| NFR-SEC004 | Password reset tokens shall expire after 1 hour. |
| NFR-SEC005 | All API endpoints (except login, forgot-password, reset-password) shall require authentication. |
| NFR-SEC006 | The system shall rate-limit authentication endpoints (login, forgot-password). |
| NFR-SEC007 | The system shall log all failed authentication attempts. |
| NFR-SEC008 | The system shall enforce minimum password length of 8 characters. |

#### Availability

| ID | Requirement |
|----|-------------|
| NFR-A001 | M0 Identity shall target 99.9% uptime. |
| NFR-A002 | Database failures shall be handled gracefully with appropriate error responses. |
| NFR-A003 | Queue unavailability shall not block synchronous API operations. Event publishing shall be best-effort with retry. |

#### Data Integrity

| ID | Requirement |
|----|-------------|
| NFR-D001 | All database writes shall use transactions where multiple tables are affected. |
| NFR-D002 | UUID primary keys shall be used for all tables. |
| NFR-D003 | Soft deletes shall be used for tenants, users, and roles. Hard deletes only for join tables. |
| NFR-D004 | All tenant-scoped queries must be filtered by `tenant_id`. No cross-tenant data leakage. |

---

### Assumptions

| ID | Assumption |
|----|-----------|
| AS-001 | A single PostgreSQL database instance is shared across all tenants. No schema-per-tenant isolation. |
| AS-002 | Tenant resolution is performed via subdomain extraction from the HTTP `Host` header. |
| AS-003 | The API gateway (Kong) handles TLS termination. The Go application receives plain HTTP internally. |
| AS-004 | Email delivery (for password reset, invitations) is handled by an external SMTP provider. M0 Identity publishes events; a notification service consumes them. |
| AS-005 | The JAAS Super Admin is a special user not scoped to any tenant, or scoped to a dedicated platform tenant. |
| AS-006 | Redis is available for session caching and rate limiting but is not the source of truth. PostgreSQL is the source of truth. |
| AS-007 | Permissions are seeded as part of application deployment, not created via API. |
| AS-008 | The system runs behind a reverse proxy that provides the `X-Forwarded-For` header for IP tracking. |

---

### Constraints

| ID | Constraint |
|----|-----------|
| CO-001 | The system must use Go as the backend language. |
| CO-002 | The system must use PostgreSQL as the primary database. |
| CO-003 | The system must use GORM as the ORM. |
| CO-004 | The system must use Gin as the HTTP framework. |
| CO-005 | The system must expose RESTful JSON APIs. No GraphQL. |
| CO-006 | The system must follow Clean Architecture principles. |
| CO-007 | M0 Identity must be deployable independently of other modules. |
| CO-008 | All API responses must follow a consistent envelope format. |
| CO-009 | M0 Identity must not depend on any other JAAS module. |
| CO-010 | The system must use RabbitMQ for event publishing. No Kafka, no NATS. |

---

### Success Criteria

| ID | Criterion | Measurement |
|----|----------|-------------|
| SC-001 | All 12 database tables are created and migrated successfully | Migration runs without errors |
| SC-002 | All 22 API endpoints return correct responses | API integration tests pass |
| SC-003 | Authentication flow (login → access → refresh → logout) works end-to-end | E2E test passes |
| SC-004 | Tenant isolation is enforced — no cross-tenant data access | Security tests pass |
| SC-005 | RBAC enforcement blocks unauthorized access | Permission-denied tests pass |
| SC-006 | All domain events are published on state changes | Event integration tests pass |
| SC-007 | Audit logs are created for all state-changing operations | Audit log assertions pass |
| SC-008 | Password reset flow works end-to-end | E2E test passes |
| SC-009 | API response times meet NFR targets | Load test report shows p95 < thresholds |
| SC-010 | Code coverage exceeds 80% for services layer | Coverage report |

---

### Future Enhancements

| ID | Enhancement | Priority | Target Module |
|----|------------|----------|---------------|
| FE-001 | OAuth2 / SSO integration (Google, Microsoft) | High | M0 Identity v2 |
| FE-002 | API key authentication for machine-to-machine access | Medium | M0 Identity v2 |
| FE-003 | Tenant-level feature flags | Medium | M0 Identity v2 |
| FE-004 | Account lockout after N failed login attempts | High | M0 Identity v2 |
| FE-005 | User email verification flow | High | M0 Identity v2 |
| FE-006 | Hierarchical roles (role inheritance) | Low | M0 Identity v3 |
| FE-007 | Attribute-Based Access Control (ABAC) | Low | M0 Identity v3 |
| FE-008 | Tenant data export (GDPR compliance) | Medium | M0 Identity v2 |
| FE-009 | Login history and device tracking | Low | M0 Identity v3 |
| FE-010 | Webhook registration for tenant events | Medium | M0 Identity v2 |

## Business rules

Every business rule in this document is numbered and must be enforced in the implementation. No exceptions.

---

### Tenant Rules

| Rule | Description |
|------|-------------|
| **TN-001** | A tenant must have a unique, non-empty `slug`. The slug is used for subdomain routing. |
| **TN-002** | A tenant slug must be lowercase, alphanumeric, and may contain hyphens. No spaces, no special characters. Regex: `^[a-z][a-z0-9-]{1,62}[a-z0-9]$`. |
| **TN-003** | A tenant slug is immutable once created. It cannot be updated. |
| **TN-004** | A tenant `domain` (if provided) must be unique across all tenants, including soft-deleted tenants. |
| **TN-005** | A tenant is created with status `active` by default. |
| **TN-006** | Valid tenant statuses are: `active`, `suspended`, `inactive`. |
| **TN-007** | Only an `active` tenant can be suspended. Transition: `active` → `suspended`. |
| **TN-008** | Only a `suspended` tenant can be activated. Transition: `suspended` → `active`. |
| **TN-009** | A suspended tenant's users cannot log in. All authentication attempts must be rejected with a tenant-suspended error. |
| **TN-010** | Suspending a tenant does not delete any data. All data remains intact and is accessible upon reactivation. |
| **TN-011** | Soft-deleting a tenant does not cascade-delete users, roles, or other data. It sets `deleted_at` and prevents all operations on the tenant. |
| **TN-012** | A soft-deleted tenant cannot be activated, suspended, or updated. |
| **TN-013** | Tenant settings are key-value pairs scoped to a single tenant. The combination of (`tenant_id`, `key`) must be unique. |

---

### Validation Rules

| Rule | Description |
|------|-------------|
| **VL-001** | All UUID fields must be valid UUIDv4 format. |
| **VL-002** | Email addresses must conform to RFC 5322 format. |
| **VL-003** | All required string fields must be non-empty after trimming whitespace. |
| **VL-004** | Pagination `page` must be ≥ 1. Default: 1. |
| **VL-005** | Pagination `per_page` must be between 1 and 100 (inclusive). Default: 20. |
| **VL-006** | String fields with a `max` constraint must not exceed the defined limit. |
| **VL-007** | Enum fields must contain one of the allowed values. Invalid values must be rejected with a 400 error. |
| **VL-008** | Request bodies must be valid JSON. Malformed JSON must be rejected with a 400 error. |
| **VL-009** | Unknown fields in request bodies should be silently ignored (not cause errors). |

---

### Authentication Rules

| Rule | Description |
|------|-------------|
| **AU-001** | Login requires three fields: `email`, `password`, and `tenant_slug`. All three are mandatory. |
| **AU-002** | The system must resolve the tenant by `tenant_slug` before authenticating the user. If the tenant is not found, return a generic "invalid credentials" error. Do not reveal whether the tenant exists. |
| **AU-003** | The system must verify the user exists within the resolved tenant. If the user is not found, return a generic "invalid credentials" error. Do not reveal whether the user exists. |
| **AU-004** | The system must compare the provided password against the stored `password_hash` using bcrypt. If the password is incorrect, return a generic "invalid credentials" error. |
| **AU-005** | Authentication must be rejected if the tenant status is not `active`. Return error: "tenant is suspended or inactive." |
| **AU-006** | Authentication must be rejected if the user status is not `active`. Return error: "account is not active." |
| **AU-007** | On successful authentication, the system must issue a JWT access token containing: `user_id`, `tenant_id`, `email`, `roles`, `exp`, `iat`. |
| **AU-008** | The JWT access token must expire after 15 minutes. |
| **AU-009** | On successful authentication, the system must issue a refresh token (opaque, random, hashed before storage). |
| **AU-010** | On successful authentication, the system must create a session record. |
| **AU-011** | On logout, the system must revoke the current session and the associated refresh token. |
| **AU-012** | Failed login attempts must be recorded in the audit log. |
| **AU-013** | Successful login attempts must be recorded in the audit log. |
| **AU-014** | The system must update the user's `last_login_at` timestamp on successful login. |

---

### Authorization Rules

| Rule | Description |
|------|-------------|
| **AZ-001** | All API endpoints (except public endpoints) require a valid JWT access token in the `Authorization: Bearer <token>` header. |
| **AZ-002** | Public endpoints that do not require authentication: `POST /auth/login`, `POST /auth/forgot-password`, `POST /auth/reset-password`. |
| **AZ-003** | The JWT must be validated for: signature integrity, expiration, required claims (`user_id`, `tenant_id`). |
| **AZ-004** | After JWT validation, the system must verify the user still exists and is active. |
| **AZ-005** | After JWT validation, the system must verify the tenant still exists and is active. |
| **AZ-006** | The `tenant_id` from the JWT must match the tenant resolved from the subdomain. Cross-tenant access is forbidden. |
| **AZ-007** | For protected endpoints, the system must check that the authenticated user has the required permission (`resource:action`) via their assigned roles. |
| **AZ-008** | If the user lacks the required permission, return 403 Forbidden. |
| **AZ-009** | Tenant management endpoints (`/api/v1/tenants`) are accessible only to the JAAS Super Admin. |

---

### RBAC Rules

| Rule | Description |
|------|-------------|
| **RB-001** | Roles are scoped to a tenant. The same role name can exist in different tenants. |
| **RB-002** | Role names must be unique within a tenant. Case-insensitive comparison. |
| **RB-003** | System roles (`is_system = true`) cannot be deleted or renamed. They can only have permissions modified. |
| **RB-004** | When a tenant is created, the following system roles must be auto-created: `tenant_admin`, `member`. |
| **RB-005** | A user can have multiple roles within a tenant. |
| **RB-006** | A role can have multiple permissions. |
| **RB-007** | Assigning a role to a user that already has that role is idempotent — no error, no duplicate. |
| **RB-008** | Assigning a permission to a role that already has that permission is idempotent — no error, no duplicate. |
| **RB-009** | When a role is deleted, all `user_roles` and `role_permissions` entries for that role must also be removed. |
| **RB-010** | Revoking a role from a user does not revoke the user's other roles. |
| **RB-011** | Permissions define what actions can be performed on what resources. Format: `resource:action` (e.g., `users:create`, `roles:delete`). |
| **RB-012** | Removing a permission from a role takes effect immediately. Existing JWTs still contain the old roles, but the permission check happens at request time against the database. |

---

### Password Policies

| Rule | Description |
|------|-------------|
| **PW-001** | Passwords must be at least 8 characters long. |
| **PW-002** | Passwords must be hashed using bcrypt with a cost factor of 12 before storage. |
| **PW-003** | Plaintext passwords must never be logged, returned in API responses, or stored. |
| **PW-004** | The password reset token must be a cryptographically random string (at least 32 bytes). |
| **PW-005** | The password reset token must be hashed (SHA-256) before storage. The raw token is sent to the user via email. |
| **PW-006** | A password reset token expires after 1 hour. |
| **PW-007** | A password reset token can only be used once. After use, `used_at` is set and the token is invalidated. |
| **PW-008** | Requesting a password reset for a non-existent email must return a success response (do not reveal whether the email exists). |
| **PW-009** | When a password is reset, all existing sessions and refresh tokens for that user must be revoked. |
| **PW-010** | The `confirm_password` field in the reset request must match the `new_password` field. |

---

### Session Rules

| Rule | Description |
|------|-------------|
| **SS-001** | A session is created on every successful login. |
| **SS-002** | A session records: `user_id`, `tenant_id`, `ip_address`, `user_agent`, `expires_at`. |
| **SS-003** | Sessions expire after 24 hours by default. |
| **SS-004** | A user can have multiple concurrent sessions (e.g., different devices). |
| **SS-005** | Revoking a session sets `revoked_at` to the current timestamp. It does not delete the record. |
| **SS-006** | A revoked session cannot be used for any operations. |
| **SS-007** | When a user is deactivated, all their sessions must be revoked. |
| **SS-008** | When a tenant is suspended, active session checks should fail at the middleware level (tenant status check). |

---

### Refresh Token Rules

| Rule | Description |
|------|-------------|
| **RT-001** | Refresh tokens are opaque, cryptographically random strings (at least 32 bytes). |
| **RT-002** | Refresh tokens are hashed (SHA-256) before storage. The raw token is returned to the client. |
| **RT-003** | Refresh tokens expire after 7 days. |
| **RT-004** | When a refresh token is used, it must be revoked and a new refresh token issued (token rotation). |
| **RT-005** | If a revoked refresh token is presented, the system must revoke ALL refresh tokens and sessions for that user (compromise detection). This is called "refresh token reuse detection." |
| **RT-006** | A refresh token is bound to a specific user and tenant. It cannot be used across tenants. |
| **RT-007** | On logout, the refresh token associated with the session must be revoked. |

---

### MFA Rules

| Rule | Description |
|------|-------------|
| **MF-001** | Supported MFA types: `totp`, `sms`, `email`. |
| **MF-002** | A user can have one MFA configuration per type. |
| **MF-003** | MFA must be verified before it is enabled. The `verified_at` timestamp must be set. |
| **MF-004** | MFA secrets must be encrypted at rest. |
| **MF-005** | If MFA is enabled for a user, the login flow must require a second factor after credential validation. |
| **MF-006** | MFA configuration is tenant-scoped. |

---

### Soft Delete Rules

| Rule | Description |
|------|-------------|
| **SD-001** | Soft delete sets `deleted_at` to the current timestamp. The record is not physically removed. |
| **SD-002** | Soft-deleted records are excluded from all default queries. |
| **SD-003** | Soft deletes apply to: `tenants`, `users`, `roles`. |
| **SD-004** | Soft deletes do NOT apply to: `permissions`, `user_roles`, `role_permissions`, `sessions`, `refresh_tokens`, `password_reset_tokens`, `mfa_configs`, `tenant_settings`, `audit_logs`. |
| **SD-005** | Unique constraints must consider soft-deleted records to prevent reuse of slugs, domains, and emails during the soft-deleted state. |
| **SD-006** | A soft-deleted record can be restored by setting `deleted_at` to null (admin operation, not exposed via API in v1). |

---

### Audit Rules

| Rule | Description |
|------|-------------|
| **AD-001** | Audit logs are immutable. No UPDATE or DELETE operations are permitted. |
| **AD-002** | Audit logs do not have `updated_at` or `deleted_at` fields. |
| **AD-003** | Every state-changing API request must produce an audit log entry. |
| **AD-004** | Audit log entries must include: `tenant_id`, `user_id` (nullable for system actions), `action`, `resource`, `resource_id`, `metadata` (JSONB), `ip_address`, `user_agent`, `created_at`. |
| **AD-005** | The `action` field uses a verb format: `tenant.created`, `user.login.success`, `user.login.failed`, `role.assigned`, `permission.revoked`, etc. |
| **AD-006** | The `metadata` field stores action-specific context as JSON (e.g., changed fields, old values, new values). |
| **AD-007** | Audit logs for failed authentication must not include the attempted password. |
| **AD-008** | Audit log queries must be filtered by `tenant_id` to prevent cross-tenant data leakage. |

---

### Unique Constraints

| Table | Constraint | Scope |
|-------|-----------|-------|
| `tenants` | `slug` | Global (unique across all tenants, including soft-deleted) |
| `tenants` | `domain` | Global (unique across all tenants, including soft-deleted, when not null) |
| `users` | (`tenant_id`, `email`) | Per-tenant unique email |
| `roles` | (`tenant_id`, `name`) | Per-tenant unique role name |
| `permissions` | (`resource`, `action`) | Global unique permission definition |
| `user_roles` | (`user_id`, `role_id`, `tenant_id`) | Prevent duplicate role assignment |
| `role_permissions` | (`role_id`, `permission_id`, `tenant_id`) | Prevent duplicate permission assignment |
| `tenant_settings` | (`tenant_id`, `key`) | Per-tenant unique setting key |
| `mfa_configs` | (`user_id`, `type`) | One MFA config per type per user |
| `refresh_tokens` | `token_hash` | Global unique hash |
| `password_reset_tokens` | `token_hash` | Global unique hash |

## API contracts

Complete REST API specification for the M0 Identity Platform. Every endpoint is fully documented with purpose, method, authentication, authorization, request/response DTOs, validation rules, error responses, status codes, and events.

---

### API Conventions

#### Base URL

```
https://{tenant_slug}.jaas.com/api/v1
```

#### Response Envelope

All successful responses follow this format:

```json
{
  "data": { ... },
  "meta": { ... }
}
```

All error responses follow this format:

```json
{
  "error": {
    "code": "ERROR_CODE",
    "message": "Human-readable error message",
    "details": [
      {
        "field": "email",
        "message": "email is required"
      }
    ]
  }
}
```

#### Common Headers

| Header | Description |
|--------|-------------|
| `Content-Type: application/json` | All requests and responses |
| `Authorization: Bearer <jwt>` | Required for authenticated endpoints |
| `X-Request-ID` | Unique request identifier for tracing |

#### Pagination

List endpoints support pagination via query parameters:

| Parameter | Default | Max | Description |
|-----------|---------|-----|-------------|
| `page` | 1 | — | Page number (1-indexed) |
| `per_page` | 20 | 100 | Items per page |

Pagination metadata is returned in the `meta` field:

```json
{
  "meta": {
    "page": 1,
    "per_page": 20,
    "total_items": 150,
    "total_pages": 8
  }
}
```

---

### Tenant Endpoints

#### POST /api/v1/tenants

**Purpose**: Create a new tenant organization.

**Authentication**: Required (Super Admin JWT)

**Authorization**: Super Admin only

**Request DTO**:

```json
{
  "name": "Acme Corporation",
  "slug": "acme",
  "domain": "acme.com",
  "plan": "professional"
}
```

| Field | Type | Required | Validation |
|-------|------|----------|------------|
| name | string | Yes | min=1, max=255 |
| slug | string | Yes | min=3, max=64, regex=`^[a-z][a-z0-9-]{1,62}[a-z0-9]$`, not in reserved words |
| domain | string | No | max=255, valid domain format |
| plan | string | No | default="free", max=50 |

**Response DTO** (201 Created):

```json
{
  "data": {
    "id": "550e8400-e29b-41d4-a716-446655440000",
    "name": "Acme Corporation",
    "slug": "acme",
    "domain": "acme.com",
    "status": "active",
    "plan": "professional",
    "created_at": "2026-07-01T00:00:00Z",
    "updated_at": "2026-07-01T00:00:00Z"
  }
}
```

**Error Responses**:

| Status | Code | Condition |
|--------|------|-----------|
| 400 | VALIDATION_ERROR | Invalid request body or field validation failure |
| 401 | UNAUTHORIZED | Missing or invalid JWT |
| 403 | FORBIDDEN | Not a Super Admin |
| 409 | CONFLICT | Slug or domain already exists |

**Published Events**: `TenantCreated`

---

#### GET /api/v1/tenants

**Purpose**: List all tenants with pagination.

**Authentication**: Required (Super Admin JWT)

**Authorization**: Super Admin only

**Query Parameters**: `page`, `per_page`

**Response DTO** (200 OK):

```json
{
  "data": [
    {
      "id": "550e8400-e29b-41d4-a716-446655440000",
      "name": "Acme Corporation",
      "slug": "acme",
      "domain": "acme.com",
      "status": "active",
      "plan": "professional",
      "created_at": "2026-07-01T00:00:00Z",
      "updated_at": "2026-07-01T00:00:00Z"
    }
  ],
  "meta": {
    "page": 1,
    "per_page": 20,
    "total_items": 1,
    "total_pages": 1
  }
}
```

**Error Responses**:

| Status | Code | Condition |
|--------|------|-----------|
| 401 | UNAUTHORIZED | Missing or invalid JWT |
| 403 | FORBIDDEN | Not a Super Admin |

**Published Events**: None

---

#### GET /api/v1/tenants/{id}

**Purpose**: Retrieve a single tenant by ID.

**Authentication**: Required (Super Admin JWT)

**Authorization**: Super Admin only

**Path Parameters**: `id` (UUID)

**Response DTO** (200 OK): Same as single tenant object above.

**Error Responses**:

| Status | Code | Condition |
|--------|------|-----------|
| 400 | VALIDATION_ERROR | Invalid UUID format |
| 401 | UNAUTHORIZED | Missing or invalid JWT |
| 403 | FORBIDDEN | Not a Super Admin |
| 404 | NOT_FOUND | Tenant not found or soft-deleted |

**Published Events**: None

---

#### PATCH /api/v1/tenants/{id}

**Purpose**: Partially update a tenant. Slug is immutable and will be ignored.

**Authentication**: Required (Super Admin JWT)

**Authorization**: Super Admin only

**Path Parameters**: `id` (UUID)

**Request DTO**:

```json
{
  "name": "Acme Corp Updated",
  "domain": "newdomain.com",
  "plan": "enterprise"
}
```

| Field | Type | Required | Validation |
|-------|------|----------|------------|
| name | string | No | min=1, max=255 |
| domain | string | No | max=255, valid domain format |
| plan | string | No | max=50 |

**Response DTO** (200 OK): Updated tenant object.

**Error Responses**:

| Status | Code | Condition |
|--------|------|-----------|
| 400 | VALIDATION_ERROR | Invalid field values |
| 401 | UNAUTHORIZED | Missing or invalid JWT |
| 403 | FORBIDDEN | Not a Super Admin |
| 404 | NOT_FOUND | Tenant not found |
| 409 | CONFLICT | Domain already in use |

**Published Events**: None

---

#### POST /api/v1/tenants/{id}/activate

**Purpose**: Activate a suspended tenant.

**Authentication**: Required (Super Admin JWT)

**Authorization**: Super Admin only

**Path Parameters**: `id` (UUID)

**Request DTO**: None (empty body)

**Response DTO** (200 OK): Updated tenant object with `status: "active"`.

**Error Responses**:

| Status | Code | Condition |
|--------|------|-----------|
| 401 | UNAUTHORIZED | Missing or invalid JWT |
| 403 | FORBIDDEN | Not a Super Admin |
| 404 | NOT_FOUND | Tenant not found |
| 409 | CONFLICT | Tenant is not suspended (already active or inactive) |

**Published Events**: `TenantActivated`

---

#### POST /api/v1/tenants/{id}/suspend

**Purpose**: Suspend an active tenant. All users of this tenant will be unable to log in.

**Authentication**: Required (Super Admin JWT)

**Authorization**: Super Admin only

**Path Parameters**: `id` (UUID)

**Request DTO**: None (empty body)

**Response DTO** (200 OK): Updated tenant object with `status: "suspended"`.

**Error Responses**:

| Status | Code | Condition |
|--------|------|-----------|
| 401 | UNAUTHORIZED | Missing or invalid JWT |
| 403 | FORBIDDEN | Not a Super Admin |
| 404 | NOT_FOUND | Tenant not found |
| 409 | CONFLICT | Tenant is not active (already suspended or inactive) |

**Published Events**: `TenantSuspended`

---

### Authentication Endpoints

#### POST /api/v1/auth/login

**Purpose**: Authenticate a user and issue access/refresh tokens.

**Authentication**: None (public endpoint)

**Authorization**: None

**Request DTO**:

```json
{
  "email": "admin@acme.com",
  "password": "SecureP@ss1",
  "tenant_slug": "acme"
}
```

| Field | Type | Required | Validation |
|-------|------|----------|------------|
| email | string | Yes | valid email format |
| password | string | Yes | min=1 |
| tenant_slug | string | Yes | min=1 |

**Response DTO** (200 OK):

```json
{
  "data": {
    "access_token": "eyJhbGciOiJIUzI1NiIs...",
    "refresh_token": "dGhpcyBpcyBhIHJhbmRvbSByZWZyZXNoIHRva2Vu...",
    "expires_at": "2026-07-01T00:15:00Z",
    "user": {
      "id": "550e8400-e29b-41d4-a716-446655440001",
      "email": "admin@acme.com",
      "first_name": "John",
      "last_name": "Doe",
      "status": "active",
      "roles": [
        {
          "id": "550e8400-e29b-41d4-a716-446655440010",
          "name": "tenant_admin"
        }
      ]
    }
  }
}
```

**Error Responses**:

| Status | Code | Condition |
|--------|------|-----------|
| 400 | VALIDATION_ERROR | Missing or invalid fields |
| 401 | INVALID_CREDENTIALS | Wrong email, password, or tenant slug |
| 403 | TENANT_SUSPENDED | Tenant is not active |
| 403 | ACCOUNT_NOT_ACTIVE | User status is not active |
| 429 | RATE_LIMITED | Too many login attempts |

**Published Events**: `UserLoggedIn` (on success)

---

#### POST /api/v1/auth/logout

**Purpose**: Revoke the current session and refresh token.

**Authentication**: Required

**Authorization**: Any authenticated user

**Request DTO**: None (session determined from JWT)

**Response DTO** (200 OK):

```json
{
  "data": {
    "message": "logged out successfully"
  }
}
```

**Error Responses**:

| Status | Code | Condition |
|--------|------|-----------|
| 401 | UNAUTHORIZED | Missing or invalid JWT |

**Published Events**: `SessionRevoked`

---

#### POST /api/v1/auth/refresh

**Purpose**: Exchange a refresh token for a new access token and refresh token.

**Authentication**: None (the refresh token itself is the credential)

**Authorization**: None

**Request DTO**:

```json
{
  "refresh_token": "dGhpcyBpcyBhIHJhbmRvbSByZWZyZXNoIHRva2Vu..."
}
```

| Field | Type | Required | Validation |
|-------|------|----------|------------|
| refresh_token | string | Yes | min=1 |

**Response DTO** (200 OK):

```json
{
  "data": {
    "access_token": "eyJhbGciOiJIUzI1NiIs...",
    "refresh_token": "bmV3IHJlZnJlc2ggdG9rZW4...",
    "expires_at": "2026-07-01T00:15:00Z"
  }
}
```

**Error Responses**:

| Status | Code | Condition |
|--------|------|-----------|
| 400 | VALIDATION_ERROR | Missing refresh_token |
| 401 | INVALID_REFRESH_TOKEN | Token not found or expired |
| 401 | TOKEN_REUSED | Reuse detected — all sessions revoked |
| 403 | TENANT_SUSPENDED | Tenant was suspended after token was issued |
| 403 | ACCOUNT_NOT_ACTIVE | User was deactivated after token was issued |

**Published Events**: None

---

#### POST /api/v1/auth/forgot-password

**Purpose**: Initiate password reset by generating a reset token and publishing an event for email delivery.

**Authentication**: None (public endpoint)

**Authorization**: None

**Request DTO**:

```json
{
  "email": "admin@acme.com",
  "tenant_slug": "acme"
}
```

| Field | Type | Required | Validation |
|-------|------|----------|------------|
| email | string | Yes | valid email format |
| tenant_slug | string | Yes | min=1 |

**Response DTO** (200 OK): Always returns success, regardless of whether email/tenant exists.

```json
{
  "data": {
    "message": "if the email exists, a reset link has been sent"
  }
}
```

**Error Responses**:

| Status | Code | Condition |
|--------|------|-----------|
| 400 | VALIDATION_ERROR | Missing or invalid fields |
| 429 | RATE_LIMITED | Too many reset requests for this email |

**Published Events**: `PasswordResetRequested` (only when user exists)

---

#### POST /api/v1/auth/reset-password

**Purpose**: Set a new password using a valid password reset token.

**Authentication**: None (the reset token is the credential)

**Authorization**: None

**Request DTO**:

```json
{
  "token": "raw-reset-token-from-email",
  "new_password": "NewSecureP@ss2",
  "confirm_password": "NewSecureP@ss2"
}
```

| Field | Type | Required | Validation |
|-------|------|----------|------------|
| token | string | Yes | min=1 |
| new_password | string | Yes | min=8 |
| confirm_password | string | Yes | must match new_password |

**Response DTO** (200 OK):

```json
{
  "data": {
    "message": "password has been reset successfully"
  }
}
```

**Error Responses**:

| Status | Code | Condition |
|--------|------|-----------|
| 400 | VALIDATION_ERROR | Missing fields, password too short, passwords don't match |
| 400 | INVALID_TOKEN | Token not found |
| 400 | TOKEN_EXPIRED | Token has expired (> 1 hour) |
| 400 | TOKEN_ALREADY_USED | Token was already consumed |

**Published Events**: `PasswordReset`

---

### User Endpoints

#### POST /api/v1/users

**Purpose**: Create a new user or invite a user within the authenticated tenant.

**Authentication**: Required

**Authorization**: `users:create`

**Request DTO**:

```json
{
  "email": "jane@acme.com",
  "password": "InitialP@ss1",
  "first_name": "Jane",
  "last_name": "Smith",
  "phone": "+1234567890",
  "role_ids": [
    "550e8400-e29b-41d4-a716-446655440010"
  ]
}
```

| Field | Type | Required | Validation |
|-------|------|----------|------------|
| email | string | Yes | valid email format, max=255 |
| password | string | Yes | min=8 |
| first_name | string | Yes | min=1, max=100 |
| last_name | string | Yes | min=1, max=100 |
| phone | string | No | max=20 |
| role_ids | []string | No | each must be valid UUID |

**Response DTO** (201 Created):

```json
{
  "data": {
    "id": "550e8400-e29b-41d4-a716-446655440002",
    "email": "jane@acme.com",
    "first_name": "Jane",
    "last_name": "Smith",
    "phone": "+1234567890",
    "avatar_url": null,
    "status": "active",
    "email_verified_at": null,
    "last_login_at": null,
    "created_at": "2026-07-01T00:00:00Z",
    "updated_at": "2026-07-01T00:00:00Z",
    "roles": [
      {
        "id": "550e8400-e29b-41d4-a716-446655440010",
        "name": "member"
      }
    ]
  }
}
```

**Error Responses**:

| Status | Code | Condition |
|--------|------|-----------|
| 400 | VALIDATION_ERROR | Invalid fields |
| 401 | UNAUTHORIZED | Missing or invalid JWT |
| 403 | FORBIDDEN | Insufficient permissions |
| 409 | CONFLICT | Email already exists in tenant |

**Published Events**: `UserCreated`

---

#### GET /api/v1/users

**Purpose**: List all users within the authenticated tenant with pagination.

**Authentication**: Required

**Authorization**: `users:read`

**Query Parameters**: `page`, `per_page`

**Response DTO** (200 OK):

```json
{
  "data": [
    {
      "id": "550e8400-e29b-41d4-a716-446655440002",
      "email": "jane@acme.com",
      "first_name": "Jane",
      "last_name": "Smith",
      "phone": "+1234567890",
      "avatar_url": null,
      "status": "active",
      "email_verified_at": null,
      "last_login_at": null,
      "created_at": "2026-07-01T00:00:00Z",
      "updated_at": "2026-07-01T00:00:00Z",
      "roles": []
    }
  ],
  "meta": {
    "page": 1,
    "per_page": 20,
    "total_items": 1,
    "total_pages": 1
  }
}
```

**Error Responses**:

| Status | Code | Condition |
|--------|------|-----------|
| 401 | UNAUTHORIZED | Missing or invalid JWT |
| 403 | FORBIDDEN | Insufficient permissions |

**Published Events**: None

---

#### GET /api/v1/users/{id}

**Purpose**: Retrieve a single user by ID within the authenticated tenant.

**Authentication**: Required

**Authorization**: `users:read`

**Path Parameters**: `id` (UUID)

**Response DTO** (200 OK): Same as single user object above.

**Error Responses**:

| Status | Code | Condition |
|--------|------|-----------|
| 400 | VALIDATION_ERROR | Invalid UUID format |
| 401 | UNAUTHORIZED | Missing or invalid JWT |
| 403 | FORBIDDEN | Insufficient permissions |
| 404 | NOT_FOUND | User not found in tenant |

**Published Events**: None

---

#### PATCH /api/v1/users/{id}

**Purpose**: Partially update a user's profile within the authenticated tenant.

**Authentication**: Required

**Authorization**: `users:update` or self (user updating their own profile)

**Path Parameters**: `id` (UUID)

**Request DTO**:

```json
{
  "first_name": "Janet",
  "last_name": "Smith-Jones",
  "phone": "+1987654321",
  "avatar_url": "https://cdn.jaas.com/avatars/jane.jpg"
}
```

| Field | Type | Required | Validation |
|-------|------|----------|------------|
| first_name | string | No | min=1, max=100 |
| last_name | string | No | min=1, max=100 |
| phone | string | No | max=20 |
| avatar_url | string | No | max=500, valid URL |

**Response DTO** (200 OK): Updated user object.

**Error Responses**:

| Status | Code | Condition |
|--------|------|-----------|
| 400 | VALIDATION_ERROR | Invalid field values |
| 401 | UNAUTHORIZED | Missing or invalid JWT |
| 403 | FORBIDDEN | Insufficient permissions |
| 404 | NOT_FOUND | User not found |

**Published Events**: None

---

#### POST /api/v1/users/{id}/deactivate

**Purpose**: Deactivate a user, revoking all their sessions and tokens.

**Authentication**: Required

**Authorization**: `users:update`

**Path Parameters**: `id` (UUID)

**Request DTO**: None

**Response DTO** (200 OK):

```json
{
  "data": {
    "message": "user deactivated successfully"
  }
}
```

**Error Responses**:

| Status | Code | Condition |
|--------|------|-----------|
| 401 | UNAUTHORIZED | Missing or invalid JWT |
| 403 | FORBIDDEN | Insufficient permissions |
| 404 | NOT_FOUND | User not found |
| 409 | CONFLICT | User is already inactive |

**Published Events**: `UserDeactivated`

---

### Role Endpoints

#### POST /api/v1/roles

**Purpose**: Create a new role within the authenticated tenant.

**Authentication**: Required

**Authorization**: `roles:create`

**Request DTO**:

```json
{
  "name": "editor",
  "description": "Can edit content but not manage users",
  "permission_ids": [
    "550e8400-e29b-41d4-a716-446655440020",
    "550e8400-e29b-41d4-a716-446655440021"
  ]
}
```

| Field | Type | Required | Validation |
|-------|------|----------|------------|
| name | string | Yes | min=1, max=100 |
| description | string | No | max=500 |
| permission_ids | []string | No | each must be valid UUID |

**Response DTO** (201 Created):

```json
{
  "data": {
    "id": "550e8400-e29b-41d4-a716-446655440011",
    "name": "editor",
    "description": "Can edit content but not manage users",
    "is_system": false,
    "permissions": [
      {
        "id": "550e8400-e29b-41d4-a716-446655440020",
        "resource": "content",
        "action": "update"
      }
    ],
    "created_at": "2026-07-01T00:00:00Z",
    "updated_at": "2026-07-01T00:00:00Z"
  }
}
```

**Error Responses**:

| Status | Code | Condition |
|--------|------|-----------|
| 400 | VALIDATION_ERROR | Invalid fields or permission IDs |
| 401 | UNAUTHORIZED | Missing or invalid JWT |
| 403 | FORBIDDEN | Insufficient permissions |
| 409 | CONFLICT | Role name already exists in tenant |

**Published Events**: None

---

#### GET /api/v1/roles

**Purpose**: List all roles within the authenticated tenant with pagination.

**Authentication**: Required

**Authorization**: `roles:read`

**Query Parameters**: `page`, `per_page`

**Response DTO** (200 OK): Array of role objects with pagination meta.

**Error Responses**:

| Status | Code | Condition |
|--------|------|-----------|
| 401 | UNAUTHORIZED | Missing or invalid JWT |
| 403 | FORBIDDEN | Insufficient permissions |

**Published Events**: None

---

#### PATCH /api/v1/roles/{id}

**Purpose**: Partially update a role's name and description.

**Authentication**: Required

**Authorization**: `roles:update`

**Path Parameters**: `id` (UUID)

**Request DTO**:

```json
{
  "name": "senior_editor",
  "description": "Updated description"
}
```

| Field | Type | Required | Validation |
|-------|------|----------|------------|
| name | string | No | min=1, max=100 |
| description | string | No | max=500 |

**Response DTO** (200 OK): Updated role object.

**Error Responses**:

| Status | Code | Condition |
|--------|------|-----------|
| 400 | VALIDATION_ERROR | Invalid field values |
| 401 | UNAUTHORIZED | Missing or invalid JWT |
| 403 | FORBIDDEN | Insufficient permissions or renaming a system role |
| 404 | NOT_FOUND | Role not found |
| 409 | CONFLICT | New name already exists in tenant |

**Published Events**: None

---

#### DELETE /api/v1/roles/{id}

**Purpose**: Delete a non-system role. Removes all user-role and role-permission associations.

**Authentication**: Required

**Authorization**: `roles:delete`

**Path Parameters**: `id` (UUID)

**Response DTO** (200 OK):

```json
{
  "data": {
    "message": "role deleted successfully"
  }
}
```

**Error Responses**:

| Status | Code | Condition |
|--------|------|-----------|
| 401 | UNAUTHORIZED | Missing or invalid JWT |
| 403 | FORBIDDEN | Insufficient permissions or attempting to delete a system role |
| 404 | NOT_FOUND | Role not found |

**Published Events**: None

---

### Permission Endpoints

#### GET /api/v1/permissions

**Purpose**: List all available permissions.

**Authentication**: Required

**Authorization**: `permissions:read`

**Query Parameters**: `page`, `per_page`

**Response DTO** (200 OK):

```json
{
  "data": [
    {
      "id": "550e8400-e29b-41d4-a716-446655440020",
      "resource": "users",
      "action": "create",
      "description": "Create a new user"
    },
    {
      "id": "550e8400-e29b-41d4-a716-446655440021",
      "resource": "users",
      "action": "read",
      "description": "View user details"
    }
  ],
  "meta": {
    "page": 1,
    "per_page": 20,
    "total_items": 2,
    "total_pages": 1
  }
}
```

**Error Responses**:

| Status | Code | Condition |
|--------|------|-----------|
| 401 | UNAUTHORIZED | Missing or invalid JWT |
| 403 | FORBIDDEN | Insufficient permissions |

**Published Events**: None

---

#### POST /api/v1/roles/{id}/permissions

**Purpose**: Assign permissions to a role. Idempotent — existing assignments are skipped.

**Authentication**: Required

**Authorization**: `roles:update`

**Path Parameters**: `id` (UUID — role ID)

**Request DTO**:

```json
{
  "permission_ids": [
    "550e8400-e29b-41d4-a716-446655440020",
    "550e8400-e29b-41d4-a716-446655440021"
  ]
}
```

| Field | Type | Required | Validation |
|-------|------|----------|------------|
| permission_ids | []string | Yes | min=1 item, each must be valid UUID |

**Response DTO** (200 OK):

```json
{
  "data": {
    "message": "permissions assigned successfully"
  }
}
```

**Error Responses**:

| Status | Code | Condition |
|--------|------|-----------|
| 400 | VALIDATION_ERROR | Invalid or empty permission_ids |
| 401 | UNAUTHORIZED | Missing or invalid JWT |
| 403 | FORBIDDEN | Insufficient permissions |
| 404 | NOT_FOUND | Role not found |

**Published Events**: `PermissionAssigned`

---

#### POST /api/v1/users/{id}/roles

**Purpose**: Assign roles to a user. Idempotent — existing assignments are skipped.

**Authentication**: Required

**Authorization**: `users:update`

**Path Parameters**: `id` (UUID — user ID)

**Request DTO**:

```json
{
  "role_ids": [
    "550e8400-e29b-41d4-a716-446655440010",
    "550e8400-e29b-41d4-a716-446655440011"
  ]
}
```

| Field | Type | Required | Validation |
|-------|------|----------|------------|
| role_ids | []string | Yes | min=1 item, each must be valid UUID |

**Response DTO** (200 OK):

```json
{
  "data": {
    "message": "roles assigned successfully"
  }
}
```

**Error Responses**:

| Status | Code | Condition |
|--------|------|-----------|
| 400 | VALIDATION_ERROR | Invalid or empty role_ids |
| 401 | UNAUTHORIZED | Missing or invalid JWT |
| 403 | FORBIDDEN | Insufficient permissions |
| 404 | NOT_FOUND | User or role not found |

**Published Events**: `RoleAssigned`

## Events

Every domain event published by M0 Identity. Includes producer, consumer, payload, retry strategy, idempotency, dead letter queue behaviour, and ordering requirements.

---

### Event Infrastructure

#### Exchange

| Property | Value |
|----------|-------|
| Exchange Name | `jaas.identity.events` |
| Exchange Type | Topic |
| Durable | Yes |
| Auto-Delete | No |

#### Routing Key Convention

```
identity.{resource}.{action}
```

Examples:
- `identity.tenant.created`
- `identity.user.login_success`
- `identity.role.assigned`

#### Event Envelope

Every event is wrapped in a standard envelope:

```json
{
  "event_id": "uuid-v4",
  "event_type": "TenantCreated",
  "routing_key": "identity.tenant.created",
  "version": "1.0",
  "occurred_at": "2026-07-01T00:00:00Z",
  "producer": "identity-service",
  "tenant_id": "uuid",
  "correlation_id": "request-uuid",
  "payload": { ... }
}
```

| Field | Type | Description |
|-------|------|-------------|
| event_id | UUID | Unique identifier for this event instance |
| event_type | string | Event type name |
| routing_key | string | RabbitMQ routing key |
| version | string | Schema version for payload |
| occurred_at | timestamp | When the event occurred |
| producer | string | Service name that produced the event |
| tenant_id | UUID | Tenant scope (if applicable) |
| correlation_id | UUID | Request ID for tracing |
| payload | object | Event-specific data |

---

### Event Definitions

#### 1. TenantCreated

| Property | Value |
|----------|-------|
| **Routing Key** | `identity.tenant.created` |
| **Producer** | TenantService |
| **Consumers** | HR/Organization (Organization) — auto-create default org structure |
| **Trigger** | POST /api/v1/tenants (after commit) |

**Payload**:

```json
{
  "tenant_id": "550e8400-e29b-41d4-a716-446655440000",
  "name": "Acme Corporation",
  "slug": "acme",
  "plan": "professional",
  "created_at": "2026-07-01T00:00:00Z"
}
```

| Concern | Strategy |
|---------|----------|
| **Retry** | 3 retries with exponential backoff (1s, 5s, 25s) |
| **Idempotency** | Consumer uses `event_id` as deduplication key. Processing the same `event_id` twice is a no-op. |
| **Dead Letter Queue** | `jaas.identity.events.dlq` — after 3 failed retries, message is routed to DLQ for manual inspection |
| **Ordering** | Not required. Tenant creation is a one-time event. |

---

#### 2. TenantActivated

| Property | Value |
|----------|-------|
| **Routing Key** | `identity.tenant.activated` |
| **Producer** | TenantService |
| **Consumers** | M10 Notifications — send reactivation notification to tenant admin |
| **Trigger** | POST /api/v1/tenants/{id}/activate (after commit) |

**Payload**:

```json
{
  "tenant_id": "550e8400-e29b-41d4-a716-446655440000",
  "previous_status": "suspended",
  "new_status": "active",
  "activated_at": "2026-07-01T00:00:00Z"
}
```

| Concern | Strategy |
|---------|----------|
| **Retry** | 3 retries with exponential backoff |
| **Idempotency** | Consumer uses `event_id` as deduplication key |
| **Dead Letter Queue** | `jaas.identity.events.dlq` |
| **Ordering** | Ordered per `tenant_id`. Activate must be processed after any preceding suspend. Use a single consumer per tenant partition. |

---

#### 3. TenantSuspended

| Property | Value |
|----------|-------|
| **Routing Key** | `identity.tenant.suspended` |
| **Producer** | TenantService |
| **Consumers** | M10 Notifications — send suspension notification; all modules — pause tenant-specific background jobs |
| **Trigger** | POST /api/v1/tenants/{id}/suspend (after commit) |

**Payload**:

```json
{
  "tenant_id": "550e8400-e29b-41d4-a716-446655440000",
  "previous_status": "active",
  "new_status": "suspended",
  "suspended_at": "2026-07-01T00:00:00Z"
}
```

| Concern | Strategy |
|---------|----------|
| **Retry** | 3 retries with exponential backoff |
| **Idempotency** | Consumer uses `event_id` as deduplication key |
| **Dead Letter Queue** | `jaas.identity.events.dlq` |
| **Ordering** | Ordered per `tenant_id`. Must be processed before any subsequent activate event. |

---

#### 4. UserCreated

| Property | Value |
|----------|-------|
| **Routing Key** | `identity.user.created` |
| **Producer** | UserService |
| **Consumers** | HR/Organization (Organization) — auto-create employee stub; M10 Notifications — send welcome email |
| **Trigger** | POST /api/v1/users (after commit) |

**Payload**:

```json
{
  "user_id": "550e8400-e29b-41d4-a716-446655440001",
  "tenant_id": "550e8400-e29b-41d4-a716-446655440000",
  "email": "jane@acme.com",
  "first_name": "Jane",
  "last_name": "Smith",
  "status": "active",
  "role_ids": ["550e8400-e29b-41d4-a716-446655440010"],
  "created_by": "550e8400-e29b-41d4-a716-446655440002",
  "created_at": "2026-07-01T00:00:00Z"
}
```

| Concern | Strategy |
|---------|----------|
| **Retry** | 3 retries with exponential backoff |
| **Idempotency** | Consumer uses `event_id` as deduplication key |
| **Dead Letter Queue** | `jaas.identity.events.dlq` |
| **Ordering** | Not strictly required. But should be processed before any UserDeactivated for the same user. |

---

#### 5. UserInvited

| Property | Value |
|----------|-------|
| **Routing Key** | `identity.user.invited` |
| **Producer** | UserService |
| **Consumers** | M10 Notifications — send invitation email with activation link |
| **Trigger** | POST /api/v1/users (when user created with status "invited") |

**Payload**:

```json
{
  "user_id": "550e8400-e29b-41d4-a716-446655440001",
  "tenant_id": "550e8400-e29b-41d4-a716-446655440000",
  "email": "invited@acme.com",
  "first_name": "New",
  "last_name": "Employee",
  "invited_by": "550e8400-e29b-41d4-a716-446655440002",
  "invited_at": "2026-07-01T00:00:00Z"
}
```

| Concern | Strategy |
|---------|----------|
| **Retry** | 3 retries with exponential backoff |
| **Idempotency** | Consumer uses `event_id` as deduplication key |
| **Dead Letter Queue** | `jaas.identity.events.dlq` |
| **Ordering** | Not required |

---

#### 6. UserDeactivated

| Property | Value |
|----------|-------|
| **Routing Key** | `identity.user.deactivated` |
| **Producer** | UserService |
| **Consumers** | HR/Organization (Organization) — update employee status; M10 Notifications — send deactivation notice to admins; all modules — revoke active user assignments |
| **Trigger** | POST /api/v1/users/{id}/deactivate (after commit) |

**Payload**:

```json
{
  "user_id": "550e8400-e29b-41d4-a716-446655440001",
  "tenant_id": "550e8400-e29b-41d4-a716-446655440000",
  "deactivated_by": "550e8400-e29b-41d4-a716-446655440002",
  "deactivated_at": "2026-07-01T00:00:00Z"
}
```

| Concern | Strategy |
|---------|----------|
| **Retry** | 3 retries with exponential backoff |
| **Idempotency** | Consumer uses `event_id` as deduplication key |
| **Dead Letter Queue** | `jaas.identity.events.dlq` |
| **Ordering** | Ordered per `user_id`. Must be processed after UserCreated for the same user. |

---

#### 7. RoleAssigned

| Property | Value |
|----------|-------|
| **Routing Key** | `identity.role.assigned` |
| **Producer** | RoleService |
| **Consumers** | M10 Notifications — notify user of new role |
| **Trigger** | POST /api/v1/users/{id}/roles (after commit) |

**Payload**:

```json
{
  "user_id": "550e8400-e29b-41d4-a716-446655440001",
  "tenant_id": "550e8400-e29b-41d4-a716-446655440000",
  "role_ids": ["550e8400-e29b-41d4-a716-446655440010"],
  "assigned_by": "550e8400-e29b-41d4-a716-446655440002",
  "assigned_at": "2026-07-01T00:00:00Z"
}
```

| Concern | Strategy |
|---------|----------|
| **Retry** | 3 retries with exponential backoff |
| **Idempotency** | Consumer uses `event_id` as deduplication key |
| **Dead Letter Queue** | `jaas.identity.events.dlq` |
| **Ordering** | Not strictly required |

---

#### 8. RoleRevoked

| Property | Value |
|----------|-------|
| **Routing Key** | `identity.role.revoked` |
| **Producer** | RoleService |
| **Consumers** | M10 Notifications — notify user of role removal |
| **Trigger** | Role removal operation (after commit) |

**Payload**:

```json
{
  "user_id": "550e8400-e29b-41d4-a716-446655440001",
  "tenant_id": "550e8400-e29b-41d4-a716-446655440000",
  "role_ids": ["550e8400-e29b-41d4-a716-446655440010"],
  "revoked_by": "550e8400-e29b-41d4-a716-446655440002",
  "revoked_at": "2026-07-01T00:00:00Z"
}
```

| Concern | Strategy |
|---------|----------|
| **Retry** | 3 retries with exponential backoff |
| **Idempotency** | Consumer uses `event_id` as deduplication key |
| **Dead Letter Queue** | `jaas.identity.events.dlq` |
| **Ordering** | Not strictly required |

---

#### 9. PermissionAssigned

| Property | Value |
|----------|-------|
| **Routing Key** | `identity.permission.assigned` |
| **Producer** | RoleService |
| **Consumers** | Cache invalidation service — clear permission cache for affected users |
| **Trigger** | POST /api/v1/roles/{id}/permissions (after commit) |

**Payload**:

```json
{
  "role_id": "550e8400-e29b-41d4-a716-446655440010",
  "tenant_id": "550e8400-e29b-41d4-a716-446655440000",
  "permission_ids": ["550e8400-e29b-41d4-a716-446655440020"],
  "assigned_at": "2026-07-01T00:00:00Z"
}
```

| Concern | Strategy |
|---------|----------|
| **Retry** | 3 retries with exponential backoff |
| **Idempotency** | Consumer uses `event_id` as deduplication key |
| **Dead Letter Queue** | `jaas.identity.events.dlq` |
| **Ordering** | Not required |

---

#### 10. PermissionRevoked

| Property | Value |
|----------|-------|
| **Routing Key** | `identity.permission.revoked` |
| **Producer** | RoleService |
| **Consumers** | Cache invalidation service — clear permission cache for affected users |
| **Trigger** | Permission removal operation (after commit) |

**Payload**:

```json
{
  "role_id": "550e8400-e29b-41d4-a716-446655440010",
  "tenant_id": "550e8400-e29b-41d4-a716-446655440000",
  "permission_ids": ["550e8400-e29b-41d4-a716-446655440020"],
  "revoked_at": "2026-07-01T00:00:00Z"
}
```

| Concern | Strategy |
|---------|----------|
| **Retry** | 3 retries with exponential backoff |
| **Idempotency** | Consumer uses `event_id` as deduplication key |
| **Dead Letter Queue** | `jaas.identity.events.dlq` |
| **Ordering** | Not required |

---

#### 11. PasswordReset

| Property | Value |
|----------|-------|
| **Routing Key** | `identity.password.reset` |
| **Producer** | AuthService |
| **Consumers** | M10 Notifications — send confirmation email; Security monitoring — alert on password changes |
| **Trigger** | POST /api/v1/auth/reset-password (after commit) |

**Payload**:

```json
{
  "user_id": "550e8400-e29b-41d4-a716-446655440001",
  "tenant_id": "550e8400-e29b-41d4-a716-446655440000",
  "reset_at": "2026-07-01T00:00:00Z"
}
```

| Concern | Strategy |
|---------|----------|
| **Retry** | 3 retries with exponential backoff |
| **Idempotency** | Consumer uses `event_id` as deduplication key |
| **Dead Letter Queue** | `jaas.identity.events.dlq` |
| **Ordering** | Not required |

---

#### 12. SessionRevoked

| Property | Value |
|----------|-------|
| **Routing Key** | `identity.session.revoked` |
| **Producer** | AuthService / SessionService |
| **Consumers** | Cache invalidation service — remove session from Redis; Security monitoring |
| **Trigger** | POST /api/v1/auth/logout, user deactivation, tenant suspension (after commit) |

**Payload**:

```json
{
  "session_id": "550e8400-e29b-41d4-a716-446655440099",
  "user_id": "550e8400-e29b-41d4-a716-446655440001",
  "tenant_id": "550e8400-e29b-41d4-a716-446655440000",
  "reason": "logout",
  "revoked_at": "2026-07-01T00:00:00Z"
}
```

| Concern | Strategy |
|---------|----------|
| **Retry** | 3 retries with exponential backoff |
| **Idempotency** | Consumer uses `event_id` as deduplication key |
| **Dead Letter Queue** | `jaas.identity.events.dlq` |
| **Ordering** | Not required |

---

### Dead Letter Queue (DLQ) Strategy

| Property | Value |
|----------|-------|
| DLQ Exchange | `jaas.identity.events.dlx` |
| DLQ Queue | `jaas.identity.events.dlq` |
| DLQ Routing Key | Original routing key preserved |
| Max Retries Before DLQ | 3 |
| DLQ TTL | None (messages persist until manually processed) |
| Alerting | Alert operations team when DLQ depth > 10 messages |
| Resolution | Manual inspection via admin tooling. Replay or discard. |

---

### Consumer Best Practices

1. **Always check `event_id` for deduplication** before processing.
2. **Use manual acknowledgement** — ACK only after successful processing.
3. **Reject and requeue** on transient failures (database timeout, network error).
4. **Reject without requeue** (→ DLQ) on permanent failures (schema mismatch, business rule violation).
5. **Log every event** received, processed, and failed for auditing.
6. **Set prefetch count** to limit concurrent processing per consumer.

## Edge cases

Every edge case listed below must be handled by the implementation. Each entry defines the scenario, the expected system behaviour, and the HTTP status code (where applicable).

---

### Tenant Edge Cases

| ID | Scenario | Expected Behaviour | Status |
|----|----------|-------------------|--------|
| EC-T001 | Create tenant with a slug that already exists | Return error: "tenant slug already exists" | 409 Conflict |
| EC-T002 | Create tenant with a slug that belongs to a soft-deleted tenant | Return error: "tenant slug already exists" (soft-deleted slugs are reserved) | 409 Conflict |
| EC-T003 | Create tenant with a domain that already exists | Return error: "domain already in use" | 409 Conflict |
| EC-T004 | Create tenant with an empty slug | Return validation error: "slug is required" | 400 Bad Request |
| EC-T005 | Create tenant with a slug containing uppercase or special characters | Return validation error: "slug must be lowercase alphanumeric with hyphens" | 400 Bad Request |
| EC-T006 | Get tenant by non-existent ID | Return error: "tenant not found" | 404 Not Found |
| EC-T007 | Get tenant by ID that belongs to a soft-deleted tenant | Return error: "tenant not found" | 404 Not Found |
| EC-T008 | Update tenant slug (immutable field) | Ignore the slug field in the request body; do not update it; do not error | 200 OK |
| EC-T009 | Activate an already active tenant | Return error: "tenant is already active" | 409 Conflict |
| EC-T010 | Activate a soft-deleted tenant | Return error: "tenant not found" | 404 Not Found |
| EC-T011 | Suspend an already suspended tenant | Return error: "tenant is already suspended" | 409 Conflict |
| EC-T012 | Suspend a soft-deleted tenant | Return error: "tenant not found" | 404 Not Found |
| EC-T013 | Update a soft-deleted tenant | Return error: "tenant not found" | 404 Not Found |
| EC-T014 | Create tenant with slug that is a reserved word (e.g., `api`, `admin`, `www`, `mail`, `app`) | Return error: "slug is reserved" | 409 Conflict |
| EC-T015 | List tenants when no tenants exist | Return empty array with pagination metadata showing 0 total | 200 OK |

---

### Authentication Edge Cases

| ID | Scenario | Expected Behaviour | Status |
|----|----------|-------------------|--------|
| EC-A001 | Login with non-existent tenant slug | Return generic error: "invalid credentials" (do not reveal tenant existence) | 401 Unauthorized |
| EC-A002 | Login with non-existent email within a valid tenant | Return generic error: "invalid credentials" (do not reveal email existence) | 401 Unauthorized |
| EC-A003 | Login with wrong password | Return generic error: "invalid credentials" | 401 Unauthorized |
| EC-A004 | Login with correct credentials but tenant is suspended | Return error: "tenant is suspended" | 403 Forbidden |
| EC-A005 | Login with correct credentials but user status is `inactive` | Return error: "account is not active" | 403 Forbidden |
| EC-A006 | Login with correct credentials but user status is `invited` | Return error: "account is not active" | 403 Forbidden |
| EC-A007 | Login with correct credentials but user status is `suspended` | Return error: "account is not active" | 403 Forbidden |
| EC-A008 | Login with correct credentials but user is soft-deleted | Return generic error: "invalid credentials" | 401 Unauthorized |
| EC-A009 | Login with empty email | Return validation error: "email is required" | 400 Bad Request |
| EC-A010 | Login with empty password | Return validation error: "password is required" | 400 Bad Request |
| EC-A011 | Login with empty tenant_slug | Return validation error: "tenant_slug is required" | 400 Bad Request |
| EC-A012 | Logout without a valid session | Return error: "session not found" | 401 Unauthorized |
| EC-A013 | Logout with an already revoked session | Return error: "session already revoked" | 401 Unauthorized |
| EC-A014 | Access protected endpoint with expired JWT | Return error: "token expired" | 401 Unauthorized |
| EC-A015 | Access protected endpoint with malformed JWT | Return error: "invalid token" | 401 Unauthorized |
| EC-A016 | Access protected endpoint with JWT signed by a different key | Return error: "invalid token" | 401 Unauthorized |
| EC-A017 | Access protected endpoint with JWT for a user that no longer exists | Return error: "user not found" | 401 Unauthorized |
| EC-A018 | Access protected endpoint with JWT for a user that has been deactivated | Return error: "account is not active" | 403 Forbidden |
| EC-A019 | JWT tenant_id does not match subdomain tenant_id | Return error: "unauthorized" | 403 Forbidden |
| EC-A020 | Concurrent logins from different devices | Allow — create separate sessions for each device | 200 OK |

---

### Refresh Token Edge Cases

| ID | Scenario | Expected Behaviour | Status |
|----|----------|-------------------|--------|
| EC-R001 | Refresh with a valid refresh token | Issue new access token and new refresh token. Revoke old refresh token. | 200 OK |
| EC-R002 | Refresh with an expired refresh token | Return error: "refresh token expired" | 401 Unauthorized |
| EC-R003 | Refresh with a revoked refresh token (reuse detection) | Revoke ALL refresh tokens and sessions for the user. Return error: "refresh token reused — all sessions revoked" | 401 Unauthorized |
| EC-R004 | Refresh with a token that does not exist in the database | Return error: "invalid refresh token" | 401 Unauthorized |
| EC-R005 | Refresh with an empty token | Return validation error: "refresh_token is required" | 400 Bad Request |
| EC-R006 | Refresh with a valid token but user has been deactivated since token was issued | Revoke all tokens and sessions. Return error: "account is not active" | 403 Forbidden |
| EC-R007 | Refresh with a valid token but tenant has been suspended since token was issued | Return error: "tenant is suspended" | 403 Forbidden |

---

### Password Reset Edge Cases

| ID | Scenario | Expected Behaviour | Status |
|----|----------|-------------------|--------|
| EC-P001 | Forgot password with a valid, active email | Generate token, publish event for email delivery. Return success message. | 200 OK |
| EC-P002 | Forgot password with a non-existent email | Return the same success message as EC-P001 (do not reveal email existence) | 200 OK |
| EC-P003 | Forgot password with email of a soft-deleted user | Return the same success message (do not reveal email existence) | 200 OK |
| EC-P004 | Forgot password with email of an inactive user | Return the same success message (do not reveal email existence) | 200 OK |
| EC-P005 | Multiple forgot-password requests for the same email within a short period | Allow — generate a new token each time. Previous tokens remain valid until expiry. Consider rate limiting. | 200 OK |
| EC-P006 | Reset password with a valid, unused token | Reset password, mark token as used, revoke all sessions and refresh tokens | 200 OK |
| EC-P007 | Reset password with an expired token | Return error: "token has expired" | 400 Bad Request |
| EC-P008 | Reset password with an already-used token | Return error: "token has already been used" | 400 Bad Request |
| EC-P009 | Reset password with a non-existent token | Return error: "invalid token" | 400 Bad Request |
| EC-P010 | Reset password where `new_password` and `confirm_password` don't match | Return validation error: "passwords do not match" | 400 Bad Request |
| EC-P011 | Reset password with a password shorter than 8 characters | Return validation error: "password must be at least 8 characters" | 400 Bad Request |
| EC-P012 | Rapid repeated forgot-password requests (potential abuse) | Rate limit: max 5 requests per email per hour | 429 Too Many Requests |

---

### User Management Edge Cases

| ID | Scenario | Expected Behaviour | Status |
|----|----------|-------------------|--------|
| EC-U001 | Create user with an email that already exists in the tenant | Return error: "email already exists" | 409 Conflict |
| EC-U002 | Create user with an email that exists in a different tenant | Allow — emails are unique per tenant, not globally | 201 Created |
| EC-U003 | Create user with an email that belongs to a soft-deleted user in the same tenant | Return error: "email already exists" (soft-deleted emails are reserved) | 409 Conflict |
| EC-U004 | Create user with a password shorter than 8 characters | Return validation error: "password must be at least 8 characters" | 400 Bad Request |
| EC-U005 | Create user with invalid email format | Return validation error: "invalid email format" | 400 Bad Request |
| EC-U006 | Create user with non-existent role IDs | Return error: "one or more role IDs are invalid" | 400 Bad Request |
| EC-U007 | Create user with role IDs from a different tenant | Return error: "one or more role IDs are invalid" | 400 Bad Request |
| EC-U008 | Get user from a different tenant than the authenticated tenant | Return error: "user not found" (tenant isolation) | 404 Not Found |
| EC-U009 | Update a soft-deleted user | Return error: "user not found" | 404 Not Found |
| EC-U010 | Deactivate a user that is already inactive | Return error: "user is already inactive" | 409 Conflict |
| EC-U011 | Deactivate a user and they try to use an existing JWT | Middleware must check user status on every request — return "account is not active" | 403 Forbidden |
| EC-U012 | Invite a user with an email that already exists in the tenant | Return error: "email already exists" | 409 Conflict |
| EC-U013 | List users with page number exceeding total pages | Return empty array with pagination metadata | 200 OK |

---

### Role Management Edge Cases

| ID | Scenario | Expected Behaviour | Status |
|----|----------|-------------------|--------|
| EC-RL001 | Create role with a name that already exists in the tenant | Return error: "role name already exists" | 409 Conflict |
| EC-RL002 | Create role with a name that exists in a different tenant | Allow — role names are unique per tenant | 201 Created |
| EC-RL003 | Delete a system role (`is_system = true`) | Return error: "system roles cannot be deleted" | 403 Forbidden |
| EC-RL004 | Delete a role that has users assigned | Delete the role and remove all `user_roles` entries for that role | 200 OK |
| EC-RL005 | Delete a non-existent role | Return error: "role not found" | 404 Not Found |
| EC-RL006 | Update a system role's name | Return error: "system role name cannot be changed" | 403 Forbidden |
| EC-RL007 | Assign a role to a user that already has that role | Idempotent — return success, do not create duplicate entry | 200 OK |
| EC-RL008 | Assign a non-existent role to a user | Return error: "role not found" | 404 Not Found |
| EC-RL009 | Assign a role from a different tenant to a user | Return error: "role not found" (tenant isolation) | 404 Not Found |

---

### Permission Edge Cases

| ID | Scenario | Expected Behaviour | Status |
|----|----------|-------------------|--------|
| EC-PM001 | Assign a permission to a role that already has it | Idempotent — return success, do not create duplicate entry | 200 OK |
| EC-PM002 | Assign a non-existent permission ID to a role | Return error: "one or more permission IDs are invalid" | 400 Bad Request |
| EC-PM003 | Remove a permission from a role, user has an active session with that permission | Permission check at request time will deny access. JWT is not revoked — it will work until it expires, but permission checks are real-time. | — |

---

### MFA Edge Cases

| ID | Scenario | Expected Behaviour | Status |
|----|----------|-------------------|--------|
| EC-M001 | Enable MFA of the same type twice for a user | Return error: "MFA type already configured" | 409 Conflict |
| EC-M002 | Enable MFA without verifying it first | Return error: "MFA must be verified before enabling" | 400 Bad Request |
| EC-M003 | MFA verification with incorrect code | Return error: "invalid MFA code" | 401 Unauthorized |
| EC-M004 | MFA verification with expired TOTP code | Return error: "invalid MFA code" | 401 Unauthorized |
| EC-M005 | Login with MFA enabled but no MFA code provided | Return partial response indicating MFA is required; do not issue tokens yet | 200 OK (with MFA challenge) |

---

### Race Conditions and Concurrency

| ID | Scenario | Expected Behaviour |
|----|----------|--------------------|
| EC-RC001 | Two concurrent requests try to create a tenant with the same slug | The database unique constraint ensures only one succeeds. The second receives 409 Conflict. |
| EC-RC002 | Two concurrent requests try to create a user with the same email in the same tenant | The database unique constraint ensures only one succeeds. The second receives 409 Conflict. |
| EC-RC003 | A role is deleted while a user is being assigned that role | The foreign key constraint or a pre-check prevents the assignment. Return error: "role not found." |
| EC-RC004 | A user is deactivated while they are in the middle of a refresh token exchange | The refresh completes or fails based on timing. Next request will fail at middleware level. |
| EC-RC005 | Two concurrent refresh token requests with the same token | One succeeds (rotates the token); the other encounters a revoked token and triggers reuse detection, revoking all tokens. |
| EC-RC006 | A tenant is suspended while a user is mid-request | The current request may complete. Subsequent requests will be rejected by tenant status middleware. |

---

### Database Conflict Scenarios

| ID | Scenario | Expected Behaviour |
|----|----------|--------------------|
| EC-DB001 | Database connection failure during login | Return 503 Service Unavailable |
| EC-DB002 | Database timeout during a write operation | Return 503 Service Unavailable. The transaction is rolled back. |
| EC-DB003 | Unique constraint violation on any field | Return 409 Conflict with a descriptive message |
| EC-DB004 | Foreign key violation (e.g., reference to non-existent tenant) | Return 400 Bad Request with a descriptive message |
| EC-DB005 | Transaction deadlock between concurrent operations | Retry the transaction once. If it fails again, return 503 Service Unavailable. |

---

### Security Abuse Scenarios

| ID | Scenario | Expected Behaviour |
|----|----------|--------------------|
| EC-SA001 | Brute-force login attempts on a single account | Rate limit: max 10 failed login attempts per account per 15-minute window. Return 429 Too Many Requests after threshold. |
| EC-SA002 | Credential stuffing (many accounts, automated login) | Rate limit by IP: max 100 login attempts per IP per 15-minute window. Return 429. |
| EC-SA003 | JWT tampering (modified payload or signature) | JWT signature verification fails. Return 401 Unauthorized. |
| EC-SA004 | SQL injection via input fields | Parameterized queries via GORM prevent SQL injection. Return 400 if input validation fails. |
| EC-SA005 | XSS via stored string fields (name, description) | Input sanitization on write. Output encoding on read. All responses are JSON (no HTML rendering). |
| EC-SA006 | Enumeration attack via forgot-password endpoint | Always return the same success response regardless of whether the email exists. |
| EC-SA007 | Enumeration attack via login endpoint | Always return the same "invalid credentials" error regardless of which field is wrong. |
| EC-SA008 | Privilege escalation by modifying JWT claims | JWT signature verification prevents claim modification. Claims are verified against the database on each request. |
| EC-SA009 | Cross-tenant access by modifying tenant_id in request | JWT tenant_id is validated against subdomain. All queries are filtered by JWT's tenant_id, not request body. |
| EC-SA010 | Replay attack with a stolen access token | Short token lifetime (15 min) limits damage. Implement token blacklisting on logout for immediate revocation. |

---

### Failure Recovery

| ID | Scenario | Expected Behaviour |
|----|----------|--------------------|
| EC-FR001 | RabbitMQ is unavailable when an event needs to be published | Log the failure. The synchronous operation (e.g., user creation) completes successfully. The event is published asynchronously with retry (see events.md). |
| EC-FR002 | Redis is unavailable for session caching | Fall back to PostgreSQL for session validation. Log the cache miss. |
| EC-FR003 | Email delivery fails for password reset | The token is still created. The user can request a new reset. The failed delivery should be logged and retried. |
| EC-FR004 | Application crash mid-transaction | PostgreSQL transaction rollback ensures no partial writes. |
| EC-FR005 | Migration failure on a production database | The migration tool (golang-migrate) supports rollback. The `.down.sql` file reverses the change. |

## Acceptance criteria

Every criterion below must be satisfied before M0 Identity is considered production-ready. Each criterion is measurable and verifiable.

---

### Database (AC-DB)

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

### API Endpoints (AC-API)

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

### Authentication (AC-AUTH)

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

### Authorization (AC-AUTHZ)

| ID | Criterion | Verification Method |
|----|----------|-------------------|
| AC-AUTHZ-001 | Unauthenticated requests to protected endpoints return 401 | API test |
| AC-AUTHZ-002 | Authenticated user without required permission gets 403 | API test |
| AC-AUTHZ-003 | Authenticated user with required permission gets 200 | API test |
| AC-AUTHZ-004 | System roles cannot be deleted | API test → 403 |
| AC-AUTHZ-005 | System role names cannot be changed | API test → 403 |
| AC-AUTHZ-006 | Permission changes take effect immediately (not cached beyond 5 min) | Assign permission → verify access within 5 min |

---

### Tenant Isolation (AC-ISO)

| ID | Criterion | Verification Method |
|----|----------|-------------------|
| AC-ISO-001 | User in Tenant A cannot see users in Tenant B | API test with two tenants |
| AC-ISO-002 | User in Tenant A cannot see roles in Tenant B | API test |
| AC-ISO-003 | User in Tenant A cannot assign roles from Tenant B | API test |
| AC-ISO-004 | JWT with Tenant A's tenant_id cannot access Tenant B's subdomain | API test → 403 |
| AC-ISO-005 | Audit logs are filtered by tenant_id | DB query verification |

---

### Events (AC-EVT)

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

### Audit Logging (AC-AUD)

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

### Security (AC-SEC)

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

### Performance (AC-PERF)

| ID | Criterion | Verification Method |
|----|----------|-------------------|
| AC-PERF-001 | Login endpoint responds in < 300ms (p95) | Load test with k6 or wrk |
| AC-PERF-002 | Single-resource GET responds in < 200ms (p95) | Load test |
| AC-PERF-003 | List endpoint with 100 items responds in < 500ms (p95) | Load test |
| AC-PERF-004 | System handles 100 concurrent login requests without errors | Load test |

---

### Code Quality (AC-CQ)

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

## Assumptions

- Loop entry assumes agent reads agent.md → current-goal.md → plan.md → current-status.md in order; skipping plan.md causes phase mistakes (e.g. starting migrations before models freeze).
- G0-1 assumes database-schema.dbml is frozen for v1; any DBML change after P1 DONE requires decisions.md + migration revision, not silent tag edits.
- G0-3 assumes Docker is available for PG/Redis/RabbitMQ; without it, boot validation substitutes to build+unit only and reports the gap in handoff (never claims E2E passed).
- Frontend G0-13 assumes backend P10+P11 DONE with live contracts; no frontend work against mocked seams.
