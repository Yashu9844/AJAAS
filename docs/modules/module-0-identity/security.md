# Module 0 — Security Specification

---

## Threat Model

### Assets Under Protection

| Asset | Classification | Impact if Compromised |
|-------|---------------|----------------------|
| User credentials (passwords) | Critical | Full account takeover |
| JWT signing keys | Critical | Complete authentication bypass |
| Refresh tokens | High | Persistent unauthorized access |
| Session data | High | Session hijacking |
| Tenant data | High | Data breach, regulatory violation |
| Audit logs | Medium | Cover tracks for attackers |
| MFA secrets | Critical | Bypass second factor |
| Password reset tokens | High | Account takeover |

### Threat Actors

| Actor | Capability | Motivation |
|-------|-----------|------------|
| External attacker | Network access, automated tools | Data theft, ransomware |
| Malicious tenant user | Authenticated access within tenant | Privilege escalation, data exfiltration |
| Compromised insider | Valid credentials, knowledge of system | Data theft, sabotage |
| Competitor | Social engineering, credential stuffing | Business intelligence |

### STRIDE Analysis

| Threat | Category | Attack Vector | Mitigation |
|--------|----------|--------------|------------|
| T1 | **S**poofing | Stolen JWT used for impersonation | Short JWT TTL (15 min), token blacklisting on logout |
| T2 | **T**ampering | Modified JWT claims to escalate privileges | JWT signature verification (HMAC-SHA256 or RS256), claims validated against DB |
| T3 | **R**epudiation | User denies performing an action | Immutable audit logs with user_id, IP, timestamp |
| T4 | **I**nformation Disclosure | Enumeration of valid emails via login/forgot-password | Generic error messages, constant-time comparison |
| T5 | **D**enial of Service | Brute-force login flooding | Rate limiting per IP and per account |
| T6 | **E**levation of Privilege | Cross-tenant data access via tenant_id manipulation | JWT tenant_id validated against subdomain, all queries filtered by tenant_id |
| T7 | **S**poofing | Refresh token theft for persistent access | Refresh token rotation, reuse detection (revoke all on reuse) |
| T8 | **I**nformation Disclosure | Password hash exposure via API response | Never return password_hash in any API response |
| T9 | **T**ampering | SQL injection via input fields | GORM parameterized queries, input validation |
| T10 | **E**levation of Privilege | Modifying own role assignments | RBAC enforcement — only users with `users:update` permission can assign roles |

---

## Authentication Strategy

### Primary Authentication: Email + Password + Tenant Slug

1. User submits `email`, `password`, and `tenant_slug`
2. System resolves tenant from slug
3. System looks up user by email within tenant
4. System compares password against bcrypt hash
5. On success: issue JWT access token + opaque refresh token
6. On failure: return generic "invalid credentials" (no information leakage)

### Token Architecture

```
┌──────────────────────────────────────────────────────────────┐
│                    Token Lifecycle                            │
│                                                              │
│  Login → JWT (15 min) + Refresh Token (7 days)               │
│                                                              │
│  Access API → JWT validated (signature + expiry + claims)    │
│                                                              │
│  Token expires → Client uses refresh token to get new JWT    │
│                                                              │
│  Refresh → Old refresh token revoked, new pair issued        │
│                                                              │
│  Logout → Session revoked, refresh token revoked,            │
│           JWT JTI blacklisted in Redis                       │
└──────────────────────────────────────────────────────────────┘
```

### MFA Strategy

When MFA is enabled for a user:

1. Credential validation succeeds → return partial response with `mfa_required: true`
2. Client submits MFA code to a verification endpoint
3. On valid MFA → issue full JWT + refresh token
4. On invalid MFA → return error, increment MFA failure counter
5. After 5 failed MFA attempts → lock MFA for 15 minutes

---

## Authorization Strategy

### RBAC Model

```
User ──[has many]──→ UserRole ──[has one]──→ Role ──[has many]──→ RolePermission ──[has one]──→ Permission
                                              │
                                    tenant-scoped
```

### Permission Format

```
{resource}:{action}
```

Examples:
- `users:create` — create a new user
- `users:read` — view user details
- `users:update` — update user profile, assign roles
- `users:delete` — delete a user
- `roles:create` — create a new role
- `roles:read` — list roles
- `roles:update` — update role, assign permissions
- `roles:delete` — delete a role
- `permissions:read` — list permissions
- `tenants:create` — create a tenant (Super Admin)
- `tenants:read` — view tenant details (Super Admin)
- `tenants:update` — update tenant, activate, suspend (Super Admin)

### Permission Check Flow

1. Extract `user_id` and `tenant_id` from JWT context
2. Query user's permissions: `SELECT DISTINCT p.resource, p.action FROM permissions p JOIN role_permissions rp ON p.id = rp.permission_id JOIN user_roles ur ON rp.role_id = ur.role_id WHERE ur.user_id = ? AND ur.tenant_id = ?`
3. Check if the required permission exists in the result set
4. If present → allow. If absent → 403 Forbidden.
5. Cache the result in Redis with 5-minute TTL

### Super Admin Authorization

Super Admin is a special role with unrestricted access to tenant management endpoints. Identification:
- The Super Admin belongs to a platform-level tenant (e.g., slug: `jaas-platform`)
- The Super Admin role has a special system role flag
- Tenant management endpoints (`/api/v1/tenants/*`) skip subdomain-based tenant resolution and instead verify Super Admin status

---

## JWT Strategy

### Token Structure

**Header**:
```json
{
  "alg": "HS256",
  "typ": "JWT"
}
```

**Payload Claims**:

| Claim | Type | Description |
|-------|------|-------------|
| `sub` | string (UUID) | User ID |
| `tid` | string (UUID) | Tenant ID |
| `email` | string | User email |
| `roles` | []string | Array of role names |
| `sid` | string (UUID) | Session ID |
| `jti` | string (UUID) | Unique token identifier |
| `iat` | number | Issued-at timestamp (Unix epoch) |
| `exp` | number | Expiration timestamp (Unix epoch, iat + 900s) |
| `iss` | string | Issuer: "jaas-identity" |

### Signing

| Property | Value |
|----------|-------|
| Algorithm | HS256 (HMAC-SHA256) for initial implementation. Migrate to RS256 (RSA) for production multi-service environments. |
| Secret | 256-bit key stored in environment variable `JWT_SECRET`. Never hardcoded. |
| Key Rotation | Support key rotation via `JWT_SECRET_PREVIOUS` for graceful rollover. Verify with current key first, fall back to previous. |

### Validation Steps

1. Parse and decode the JWT
2. Verify signature using `JWT_SECRET`
3. Check `exp` > current time
4. Check `iss` == "jaas-identity"
5. Check `jti` is not in the Redis blacklist
6. Extract `sub` (user_id), `tid` (tenant_id), `sid` (session_id)
7. Verify `tid` matches subdomain-resolved tenant_id

---

## Refresh Token Rotation

### Token Generation

1. Generate 32 bytes of cryptographically secure random data (`crypto/rand`)
2. Base64-URL encode the random bytes → this is the raw refresh token returned to the client
3. SHA-256 hash the raw token → store the hash in `refresh_tokens.token_hash`
4. Set `expires_at` to current time + 7 days

### Rotation Flow

```
Client sends refresh_token →
  Server hashes it →
    Looks up token_hash in DB →
      If found AND not revoked AND not expired:
        1. Revoke old token (set revoked_at)
        2. Generate new refresh token
        3. Issue new JWT
        4. Return new access_token + refresh_token
      If found AND revoked (REUSE DETECTED):
        1. Revoke ALL refresh tokens for this user
        2. Revoke ALL sessions for this user
        3. Return 401 with security alert
      If not found or expired:
        Return 401
```

### Reuse Detection Rationale

If a revoked refresh token is presented, it means either:
- The token was intercepted by an attacker, OR
- A legitimate client is using a stale token (e.g., due to a race condition)

In both cases, the safest action is to revoke everything and force re-authentication.

---

## Password Hashing

| Property | Value |
|----------|-------|
| Algorithm | bcrypt |
| Cost Factor | 12 |
| Library | `golang.org/x/crypto/bcrypt` |
| Storage Format | `$2a$12$...` (standard bcrypt output) |

### Password Requirements

| Requirement | Value |
|------------|-------|
| Minimum length | 8 characters |
| Maximum length | 72 bytes (bcrypt limit) |
| Complexity | Not enforced in v1 (consider adding in v2: uppercase, lowercase, digit, special char) |

### Hashing Rules

1. Hash on creation: `bcrypt.GenerateFromPassword([]byte(password), 12)`
2. Hash on reset: same as creation
3. Compare on login: `bcrypt.CompareHashAndPassword(hash, []byte(password))`
4. Never store plaintext. Never log plaintext. Never return hash in API responses.
5. Use constant-time comparison (bcrypt does this internally).

---

## Rate Limiting

### Rate Limit Configuration

| Endpoint | Rate Limit | Window | Key |
|----------|-----------|--------|-----|
| POST /auth/login | 10 requests | 15 minutes | Per email + tenant_slug |
| POST /auth/login | 100 requests | 15 minutes | Per IP address |
| POST /auth/forgot-password | 5 requests | 1 hour | Per email + tenant_slug |
| POST /auth/reset-password | 10 requests | 1 hour | Per IP address |
| POST /auth/refresh | 30 requests | 15 minutes | Per user_id |
| All other endpoints | 100 requests | 1 minute | Per user_id |

### Implementation

- Use Redis-backed sliding window counter
- Key format: `rate:{endpoint}:{identifier}:{window_start}`
- Return `429 Too Many Requests` with `Retry-After` header when limit exceeded
- Rate limit headers on every response:
  - `X-RateLimit-Limit`: Maximum requests allowed
  - `X-RateLimit-Remaining`: Remaining requests in window
  - `X-RateLimit-Reset`: Unix timestamp when window resets

---

## Brute Force Protection

### Account-Level Protection

| Measure | Threshold | Action |
|---------|----------|--------|
| Failed login attempts | 10 per account per 15 min | Return 429, log security event |
| Failed MFA attempts | 5 per account per 15 min | Lock MFA verification for 15 min |
| Failed password reset | 10 per token per 1 hour | Return 429 |

### IP-Level Protection

| Measure | Threshold | Action |
|---------|----------|--------|
| Login attempts from single IP | 100 per 15 min | Return 429 for all login attempts from that IP |
| Password reset from single IP | 20 per 1 hour | Return 429 |

### Account Lockout (Future Enhancement)

In v2, implement progressive lockout:
1. After 10 failed attempts → 15-minute cooldown
2. After 20 failed attempts → 1-hour cooldown
3. After 30 failed attempts → account locked, requires admin unlock

---

## Audit Logging

### What to Log

| Event Category | Actions |
|---------------|---------|
| Authentication | login.success, login.failed, logout, token.refresh |
| User Management | user.created, user.updated, user.deactivated, user.invited |
| Role Management | role.created, role.updated, role.deleted, role.assigned, role.revoked |
| Permission Management | permission.assigned, permission.revoked |
| Tenant Management | tenant.created, tenant.updated, tenant.activated, tenant.suspended |
| Password | password.reset_requested, password.reset_completed |
| Security | token.reuse_detected, rate_limit.exceeded |

### Audit Log Fields

| Field | Source |
|-------|--------|
| tenant_id | JWT context or request body |
| user_id | JWT context (null for failed logins) |
| action | Predefined action string |
| resource | Resource type string |
| resource_id | UUID of affected resource |
| metadata | JSON with change details (old values, new values, reason) |
| ip_address | `X-Forwarded-For` or `RemoteAddr` |
| user_agent | `User-Agent` header |
| created_at | Server timestamp (UTC) |

### Immutability Enforcement

- No UPDATE operations on `audit_logs` table
- No DELETE operations on `audit_logs` table
- The repository interface exposes only `Create` and `FindMany` methods
- GORM model does not include `updated_at` or `deleted_at` fields
- Database-level trigger (optional) to prevent UPDATE/DELETE at the DB layer

---

## Encryption

### Data at Rest

| Data | Encryption |
|------|-----------|
| Passwords | bcrypt hash (one-way) |
| Refresh tokens | SHA-256 hash (one-way) |
| Password reset tokens | SHA-256 hash (one-way) |
| MFA secrets | AES-256-GCM encryption (two-way, decryptable for verification) |
| Database | PostgreSQL TDE or filesystem-level encryption (infrastructure) |
| Redis | Redis AUTH + TLS in production |

### Data in Transit

| Path | Encryption |
|------|-----------|
| Client → Kong | TLS 1.2+ (mandatory in production) |
| Kong → Application | Plain HTTP (internal network, TLS terminated at gateway) |
| Application → PostgreSQL | TLS in production (`sslmode=require`) |
| Application → Redis | TLS in production |
| Application → RabbitMQ | TLS in production |

### Key Management

| Key | Storage | Rotation |
|-----|---------|----------|
| JWT signing secret | Environment variable `JWT_SECRET` | Rotate every 90 days, support dual-key verification during rollover |
| MFA encryption key | Environment variable `MFA_ENCRYPTION_KEY` | Rotate with re-encryption of all MFA secrets |
| Database password | Environment variable or secrets manager | Rotate per organization policy |

---

## OWASP Considerations

### OWASP Top 10 (2021) Mapping

| # | Threat | Relevance | Mitigation |
|---|--------|-----------|------------|
| A01 | Broken Access Control | **High** | RBAC middleware on every endpoint. Tenant isolation via tenant_id. JWT validation. |
| A02 | Cryptographic Failures | **High** | bcrypt for passwords. SHA-256 for tokens. AES-256 for MFA. TLS in transit. |
| A03 | Injection | **Medium** | GORM parameterized queries. Input validation on all DTOs. |
| A04 | Insecure Design | **Medium** | Clean Architecture. Security by design. Threat modeling. |
| A05 | Security Misconfiguration | **Medium** | No default credentials. Environment-based config. Strict CORS. |
| A06 | Vulnerable Components | **Low** | Regular dependency updates. `go mod audit`. |
| A07 | Identification and Auth Failures | **High** | bcrypt, JWT rotation, MFA, rate limiting, brute-force protection. |
| A08 | Software and Data Integrity Failures | **Medium** | JWT signature verification. Immutable audit logs. |
| A09 | Security Logging and Monitoring Failures | **High** | Comprehensive audit logging. Failed login tracking. |
| A10 | Server-Side Request Forgery | **Low** | No user-controlled URL fetching in Module 0. |

### Additional OWASP Recommendations

| Recommendation | Implementation |
|---------------|---------------|
| Secure HTTP headers | `X-Content-Type-Options: nosniff`, `X-Frame-Options: DENY`, `Strict-Transport-Security`, `X-XSS-Protection: 0` (modern browsers), `Content-Security-Policy` |
| CORS configuration | Whitelist specific origins. No `*` in production. |
| Cookie security (if applicable) | `HttpOnly`, `Secure`, `SameSite=Strict` |
| Error handling | Never expose stack traces or internal error details to clients |
| Input length limits | Enforce max length on all string inputs at validation layer |

---

## Security Checklist

### Pre-Production Checklist

- [ ] JWT signing secret is at least 256 bits and stored in environment variable
- [ ] JWT signing secret is not committed to source control
- [ ] bcrypt cost factor is set to 12
- [ ] All passwords are hashed before storage
- [ ] No plaintext passwords in logs, responses, or database
- [ ] Refresh token rotation is implemented
- [ ] Refresh token reuse detection is implemented
- [ ] Password reset tokens expire after 1 hour
- [ ] Password reset tokens are single-use
- [ ] Generic error messages on login/forgot-password (no enumeration)
- [ ] Rate limiting is configured for all auth endpoints
- [ ] CORS is configured with specific origins (no wildcard)
- [ ] TLS is enforced in production
- [ ] Database connections use TLS in production
- [ ] All API responses follow consistent error format (no stack traces)
- [ ] Audit logs are immutable (no UPDATE/DELETE)
- [ ] All tenant-scoped queries include tenant_id filter
- [ ] JWT tenant_id is validated against subdomain
- [ ] MFA secrets are encrypted at rest
- [ ] Session timeout is enforced
- [ ] Token blacklisting is implemented for logout
- [ ] Input validation is applied to all request DTOs
- [ ] SQL injection is prevented via parameterized queries
- [ ] Dependency vulnerabilities are scanned (`go mod audit`)
- [ ] Security headers are set on all responses
