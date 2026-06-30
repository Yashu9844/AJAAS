# Module 0 — API Contracts

Complete REST API specification for the Module 0 Identity Platform. Every endpoint is fully documented with purpose, method, authentication, authorization, request/response DTOs, validation rules, error responses, status codes, and events.

---

## API Conventions

### Base URL

```
https://{tenant_slug}.jaas.com/api/v1
```

### Response Envelope

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

### Common Headers

| Header | Description |
|--------|-------------|
| `Content-Type: application/json` | All requests and responses |
| `Authorization: Bearer <jwt>` | Required for authenticated endpoints |
| `X-Request-ID` | Unique request identifier for tracing |

### Pagination

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

## Tenant Endpoints

### POST /api/v1/tenants

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

### GET /api/v1/tenants

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

### GET /api/v1/tenants/{id}

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

### PATCH /api/v1/tenants/{id}

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

### POST /api/v1/tenants/{id}/activate

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

### POST /api/v1/tenants/{id}/suspend

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

## Authentication Endpoints

### POST /api/v1/auth/login

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

### POST /api/v1/auth/logout

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

### POST /api/v1/auth/refresh

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

### POST /api/v1/auth/forgot-password

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

### POST /api/v1/auth/reset-password

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

## User Endpoints

### POST /api/v1/users

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

### GET /api/v1/users

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

### GET /api/v1/users/{id}

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

### PATCH /api/v1/users/{id}

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

### POST /api/v1/users/{id}/deactivate

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

## Role Endpoints

### POST /api/v1/roles

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

### GET /api/v1/roles

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

### PATCH /api/v1/roles/{id}

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

### DELETE /api/v1/roles/{id}

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

## Permission Endpoints

### GET /api/v1/permissions

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

### POST /api/v1/roles/{id}/permissions

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

### POST /api/v1/users/{id}/roles

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
