# Module 0 — Business Rules

Every business rule in this document is numbered and must be enforced in the implementation. No exceptions.

---

## Tenant Rules

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

## Validation Rules

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

## Authentication Rules

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

## Authorization Rules

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

## RBAC Rules

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

## Password Policies

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

## Session Rules

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

## Refresh Token Rules

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

## MFA Rules

| Rule | Description |
|------|-------------|
| **MF-001** | Supported MFA types: `totp`, `sms`, `email`. |
| **MF-002** | A user can have one MFA configuration per type. |
| **MF-003** | MFA must be verified before it is enabled. The `verified_at` timestamp must be set. |
| **MF-004** | MFA secrets must be encrypted at rest. |
| **MF-005** | If MFA is enabled for a user, the login flow must require a second factor after credential validation. |
| **MF-006** | MFA configuration is tenant-scoped. |

---

## Soft Delete Rules

| Rule | Description |
|------|-------------|
| **SD-001** | Soft delete sets `deleted_at` to the current timestamp. The record is not physically removed. |
| **SD-002** | Soft-deleted records are excluded from all default queries. |
| **SD-003** | Soft deletes apply to: `tenants`, `users`, `roles`. |
| **SD-004** | Soft deletes do NOT apply to: `permissions`, `user_roles`, `role_permissions`, `sessions`, `refresh_tokens`, `password_reset_tokens`, `mfa_configs`, `tenant_settings`, `audit_logs`. |
| **SD-005** | Unique constraints must consider soft-deleted records to prevent reuse of slugs, domains, and emails during the soft-deleted state. |
| **SD-006** | A soft-deleted record can be restored by setting `deleted_at` to null (admin operation, not exposed via API in v1). |

---

## Audit Rules

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

## Unique Constraints

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
