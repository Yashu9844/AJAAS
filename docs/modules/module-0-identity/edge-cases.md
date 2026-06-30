# Module 0 — Edge Cases

Every edge case listed below must be handled by the implementation. Each entry defines the scenario, the expected system behaviour, and the HTTP status code (where applicable).

---

## Tenant Edge Cases

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

## Authentication Edge Cases

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

## Refresh Token Edge Cases

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

## Password Reset Edge Cases

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

## User Management Edge Cases

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

## Role Management Edge Cases

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

## Permission Edge Cases

| ID | Scenario | Expected Behaviour | Status |
|----|----------|-------------------|--------|
| EC-PM001 | Assign a permission to a role that already has it | Idempotent — return success, do not create duplicate entry | 200 OK |
| EC-PM002 | Assign a non-existent permission ID to a role | Return error: "one or more permission IDs are invalid" | 400 Bad Request |
| EC-PM003 | Remove a permission from a role, user has an active session with that permission | Permission check at request time will deny access. JWT is not revoked — it will work until it expires, but permission checks are real-time. | — |

---

## MFA Edge Cases

| ID | Scenario | Expected Behaviour | Status |
|----|----------|-------------------|--------|
| EC-M001 | Enable MFA of the same type twice for a user | Return error: "MFA type already configured" | 409 Conflict |
| EC-M002 | Enable MFA without verifying it first | Return error: "MFA must be verified before enabling" | 400 Bad Request |
| EC-M003 | MFA verification with incorrect code | Return error: "invalid MFA code" | 401 Unauthorized |
| EC-M004 | MFA verification with expired TOTP code | Return error: "invalid MFA code" | 401 Unauthorized |
| EC-M005 | Login with MFA enabled but no MFA code provided | Return partial response indicating MFA is required; do not issue tokens yet | 200 OK (with MFA challenge) |

---

## Race Conditions and Concurrency

| ID | Scenario | Expected Behaviour |
|----|----------|--------------------|
| EC-RC001 | Two concurrent requests try to create a tenant with the same slug | The database unique constraint ensures only one succeeds. The second receives 409 Conflict. |
| EC-RC002 | Two concurrent requests try to create a user with the same email in the same tenant | The database unique constraint ensures only one succeeds. The second receives 409 Conflict. |
| EC-RC003 | A role is deleted while a user is being assigned that role | The foreign key constraint or a pre-check prevents the assignment. Return error: "role not found." |
| EC-RC004 | A user is deactivated while they are in the middle of a refresh token exchange | The refresh completes or fails based on timing. Next request will fail at middleware level. |
| EC-RC005 | Two concurrent refresh token requests with the same token | One succeeds (rotates the token); the other encounters a revoked token and triggers reuse detection, revoking all tokens. |
| EC-RC006 | A tenant is suspended while a user is mid-request | The current request may complete. Subsequent requests will be rejected by tenant status middleware. |

---

## Database Conflict Scenarios

| ID | Scenario | Expected Behaviour |
|----|----------|--------------------|
| EC-DB001 | Database connection failure during login | Return 503 Service Unavailable |
| EC-DB002 | Database timeout during a write operation | Return 503 Service Unavailable. The transaction is rolled back. |
| EC-DB003 | Unique constraint violation on any field | Return 409 Conflict with a descriptive message |
| EC-DB004 | Foreign key violation (e.g., reference to non-existent tenant) | Return 400 Bad Request with a descriptive message |
| EC-DB005 | Transaction deadlock between concurrent operations | Retry the transaction once. If it fails again, return 503 Service Unavailable. |

---

## Security Abuse Scenarios

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

## Failure Recovery

| ID | Scenario | Expected Behaviour |
|----|----------|--------------------|
| EC-FR001 | RabbitMQ is unavailable when an event needs to be published | Log the failure. The synchronous operation (e.g., user creation) completes successfully. The event is published asynchronously with retry (see events.md). |
| EC-FR002 | Redis is unavailable for session caching | Fall back to PostgreSQL for session validation. Log the cache miss. |
| EC-FR003 | Email delivery fails for password reset | The token is still created. The user can request a new reset. The failed delivery should be logged and retried. |
| EC-FR004 | Application crash mid-transaction | PostgreSQL transaction rollback ensures no partial writes. |
| EC-FR005 | Migration failure on a production database | The migration tool (golang-migrate) supports rollback. The `.down.sql` file reverses the change. |
