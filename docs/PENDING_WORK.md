# JAAS — What's Pending (Per Service)

Source of truth: `docs/modules/module-0-identity/` specs vs what's on disk.
Status: shared foundation done, everything else pending.

## Backend — Module 0 Identity

### 1. Models (11/12 missing)

Only `internal/identity/models/tenant.go` exists — and it doesn't compile (references `User, Role, Session, AuditLog, TenantSettings` that don't exist).

| Model | What it does | Key features / constraints |
|---|---|---|
| `TenantSettings` | Per-tenant key-value config (timezone, locale, mfa_required) | FK tenant_id, unique(tenant_id, key) |
| `User` | Identity within a tenant | unique(tenant_id, email), bcrypt hash (cost 12), first/last/phone/avatar, status active/suspended/inactive/invited, email_verified_at, last_login_at, soft delete |
| `Role` | Tenant-scoped role | unique(tenant_id, name) case-insensitive, is_system flag, soft delete, auto-seed `tenant_admin, member` on tenant create |
| `Permission` | Global resource-action definition | No tenant_id, unique(resource, action), e.g. `users:create`, seeded at deploy, no soft delete |
| `UserRole` | User↔Role join | unique(user_id, role_id, tenant_id), assigned_at, assigned_by, hard delete |
| `RolePermission` | Role↔Permission join | unique(role_id, permission_id, tenant_id), hard delete |
| `Session` | Login session per device | user_id/tenant_id, ip, user_agent, expires_at (24h), revoked_at; multiple concurrent allowed; revoke sets revoked_at, never deletes |
| `RefreshToken` | Opaque refresh token | 32-byte crypto rand, SHA-256 hashed, unique(token_hash), 7d expiry, rotation + reuse detection |
| `PasswordResetToken` | Password reset token | Same hashing, 1h expiry, single-use via used_at |
| `MFAConfig` | Per-user MFA | unique(user_id, type), types totp/sms/email, secret AES-256-GCM, verified_at required before enable |
| `AuditLog` | Immutable audit trail | tenant_id, user_id (nullable), action (`tenant.created`, `user.login.success`…), resource, resource_id, metadata JSONB, ip, user_agent, created_at only — no update/delete |

### 2. Migrations (0/12)

`migrations/` is empty. Need 12 up/down pairs: tenants → users → roles → permissions → user_roles → role_permissions → sessions → refresh_tokens → password_reset_tokens → mfa_configs → tenant_settings → audit_logs. Enable uuid-ossp, UUID PKs, FKs, indexes (incl. tenant_id on all scoped tables), down in reverse order.

### 3. DTOs + Validation (0/7)

`internal/identity/dto/` is empty. Need: pagination (page ≥1 default 1, per_page 1–100 default 20), tenant (Create/Update/Response, slug regex `^[a-z][a-z0-9-]{1,62}[a-z0-9]$`, reserved words api/admin/www/mail/app), auth (Login/Refresh/Forgot/Reset, min 8 password, confirm match), user, role, permission, session. Plus `shared/utils/validator.go`. `shared/errors/errors.go` already done.

### 4. Repositories (0/12)

`internal/identity/repositories/` is empty. Need interfaces + GORM impls for all 12 entities: Tenant, User, Role, Permission, UserRole, RolePermission, Session, RefreshToken, PasswordResetToken, MFAConfig, TenantSettings, AuditLog (Create + FindMany only). All queries tenant_id-scoped (except permissions + global tenant lookups), pagination, preload, `*gorm.DB` tx injection, GORM soft-delete handling.

### 5. TenantService

What: tenant lifecycle. Only Super Admin can call.
Features: Create (slug unique incl. soft-deleted, domain unique, default active/free, auto-create system roles), Get, List (paginated), Update (name/domain/plan only — slug immutable, ignored not error), Activate (suspended→active), Suspend (active→suspended, users can't login, data intact, no cascade delete). Soft-deleted tenants: not found for all ops.
Events: TenantCreated, TenantActivated, TenantSuspended. Every state change audit-logged.

### 6. AuthService

What: login, logout, refresh, forgot/reset password.
Features:
- Login (email + password + tenant_slug, all required): resolve tenant → user → bcrypt compare. Generic `invalid credentials` for bad tenant/email/password/soft-deleted user (no enumeration). 403 for suspended tenant or non-active user. Success: JWT (15m, claims sub/tid/email/roles/sid/jti/iat/exp, iss `jaas-identity`, HS256) + opaque refresh (7d) + session + last_login_at update + audit.
- Logout: revoke session + refresh, blacklist JWT JTI in Redis. Publishes SessionRevoked.
- Refresh: hash → lookup → rotate (revoke old, issue new pair). Reuse of revoked token → revoke ALL tokens + sessions for user (compromise detection), 401. Re-check user/tenant active.
- Forgot: always 200 even if email missing (PW-008). 5/hour per email rate limit. Publishes event only if user exists.
- Reset: token valid + unused + <1h, new ≥8 chars, confirm match. On success: hash password, set used_at, revoke all sessions/tokens. Publishes PasswordReset.
- MFA hook: if enabled, return `mfa_required:true` partial instead of tokens; verify endpoint (5 fails → 15m lock).

### 7. UserService

What: user CRUD + invite within authenticated tenant. Requires `users:create/read/update`.
Features: Create (email unique per tenant incl. soft-deleted, cross-tenant reuse allowed, bcrypt, role_ids must exist in same tenant), Get/List (tenant-scoped, paginated), Update (first/last/phone/avatar only — self or `users:update`), Deactivate (revoke all sessions/tokens, already-inactive → 409). Invite creates status `invited`. Never return/log plaintext or hash.
Events: UserCreated, UserInvited, UserDeactivated.

### 8. RoleService

What: roles + assignments. Requires `roles:create/read/update/delete`, `users:update` for user-role assign.
Features: Create (per-tenant unique name case-insensitive), List, Update (name/desc — system role rename → 403), Delete (non-system only, cascades user_roles + role_permissions). AssignPermissions / AssignRoles: idempotent (skip existing, no dup), validate same-tenant IDs. Permission checks are real-time DB (JWT roles stale-OK, 5-min Redis cache).
Events: RoleAssigned, RoleRevoked, PermissionAssigned, PermissionRevoked.

### 9. PermissionService

What: read-only catalog. Requires `permissions:read`.
Features: List global `resource:action` pairs (users:create/read/update, roles:*, permissions:read, tenants:create/read/update). Seeded at deploy, not created via API.

### 10. SessionService + TokenService

What: session lifecycle + crypto token ops. Redis-backed, PG fallback.
Features: Create (ip, user-agent, 24h expiry), Validate (check revoked + expiry + user/tenant active), Revoke / RevokeAll (on deactivate, password reset, reuse detection, logout). Token gen: crypto/rand 32 bytes, base64url to client, SHA-256 in DB. JTI blacklist on logout. Permission cache 5-min TTL, cleared on PermissionAssigned/Revoked events.

### 11. AuditService

What: immutable security trail, best-effort non-blocking.
Features: Create entries for every state change: login.success/failed (no passwords), logout, token.refresh, user.created/updated/deactivated/invited, role.created/updated/deleted/assigned/revoked, permission.assigned/revoked, tenant.created/updated/activated/suspended, password.reset_requested/completed, token.reuse_detected, rate_limit.exceeded. Fields: tenant_id, user_id (null for failed logins), action, resource, resource_id, metadata JSONB, ip (X-Forwarded-For), user_agent, created_at UTC.

### 12. Controllers + Routes (0/24 endpoints)

`controllers/`, `routes/` empty. Pattern: parse body → DTO → validate → service → response DTO → status code. Envelope `{data, meta}` / `{error:{code,message,details}}`, `X-Request-ID`.

| Group | Endpoints |
|---|---|
| Tenants (Super Admin only) | POST /tenants (201) · GET /tenants · GET /tenants/{id} · PATCH /tenants/{id} · POST /tenants/{id}/activate · POST /tenants/{id}/suspend |
| Auth (login/forgot/reset public) | POST /auth/login · POST /auth/logout · POST /auth/refresh · POST /auth/forgot-password (always 200) · POST /auth/reset-password |
| Users (`users:*`) | POST /users (201) · GET /users · GET /users/{id} · PATCH /users/{id} · POST /users/{id}/deactivate |
| Roles (`roles:*`) | POST /roles (201) · GET /roles · PATCH /roles/{id} · DELETE /roles/{id} |
| Permissions | GET /permissions · POST /roles/{id}/permissions · POST /users/{id}/roles |

### 13. Middleware (0/6)

`identity/middleware/`, `shared/middleware/`, `shared/queue/`, `shared/cache/` all empty.
Order: CORS → RateLimit → TenantResolver → Auth → RBAC → Audit.
- TenantResolver: Host subdomain → slug → tenant_id in context. 404 unknown, 403 suspended.
- JWT Auth: Bearer validate (signature, exp, iss, JTI not blacklisted), set user/tenant/roles, verify JWT tid == subdomain tid, verify user + tenant still active.
- RBAC: required `resource:action` per route → DB permission query → 403 if missing.
- RateLimiter: Redis sliding window `rate:{endpoint}:{id}:{window}`. login 10/15m per email + 100/15m per IP; forgot 5/h per email; reset 10/h per IP; refresh 30/15m per user; default 100/min per user. 429 + `Retry-After` + `X-RateLimit-*` headers.
- Audit: post-handler log with metadata. CORS: whitelist origins (no `*` prod) + secure headers (nosniff, DENY, HSTS, CSP).

### 14. Events / Queue (0)

No publisher, no RabbitMQ conn, no event defs. Need: exchange `jaas.identity.events` (topic, durable), routing `identity.{resource}.{action}`, envelope (event_id, event_type, routing_key, version, occurred_at, producer, tenant_id, correlation_id, payload). 12 events: TenantCreated/Activated/Suspended, UserCreated/Invited/Deactivated, RoleAssigned/Revoked, PermissionAssigned/Revoked, PasswordReset, SessionRevoked. 3 retries (1s/5s/25s), DLQ `jaas.identity.events.dlq` (alert if depth >10), idempotency via event_id, publish-after-commit, queue-down never blocks API (best-effort + fallback table + retry worker).

### 15. Bootstrap, infra, testing (missing)

- `cmd/main.go` doesn't exist (Makefile `dev-backend` points here). Need DI wiring (config → logger → DB → Redis → RabbitMQ → repos → services → controllers → middleware → Gin), permission seed, Swagger.
- Compose has frontend + backend only — missing Postgres, Redis, RabbitMQ, Kong (TLS + routing), SMTP. Prod secrets empty (`jwt.secret`, `db.password` must inject). `backend/Dockerfile` go1.22 vs go1.25. No golang-migrate wiring, no CI.
- Tests: only 3 shared tests exist. Need service unit (90%), controller, middleware, repo integration (Docker PG), API E2E (login→access→refresh→logout, isolation, RBAC), security (enumeration, tampering, reuse, injection), `go test -race`, coverage ≥80%. Perf targets: p95 <200ms single, <500ms list, <300ms login, 100 concurrent logins.

## Frontend (only bootstrap exists)

`src/app/page.tsx` is a placeholder. All of `components/hooks/lib/services/stores/styles/types/utils` + `modules/identity,organization,employee` are `.gitkeep`.

### Identity module (`modules/identity/`)

- API client for all 24 endpoints: envelope unwrap, pagination, error mapping, `Retry-After` handling.
- Auth service: login (email/password/tenant_slug), logout, silent refresh + 401→refresh→retry, JWT storage, tenant context from subdomain.
- Pages: login, forgot-password, reset-password, invite-accept. States for generic errors (no enumeration leak), 403 suspended/inactive, MFA challenge (`mfa_required`) hook.
- Components: LoginForm, AuthGuard, RBAC guard (`resource:action`), user/role/permission tables + forms.
- Hooks: useAuth, useSession, usePermissions. Types: User, Tenant, Role, JWT claims. Store: auth + tenant context.

### Organization module (blocked — Module 1 not designed)

Next task after ER diagram per `context01.md`: Departments, Teams, Designations, Reporting hierarchy, Employee mapping, Org chart. Depends on Tenant + User + RBAC. No spec yet — expect services/pages for CRUD + tree views + assignment flows.

### Employee module (blocked)

Profiles linked to users, status sync on UserDeactivated, depends on Org + Identity. Expect profile CRUD, invite→employee-stub flow (consumes UserCreated event on backend), directory pages.

### Shared shell

Subdomain routing (`{slug}.jaas.com`), fetch wrapper, validation utils, design system/styles, error boundary, rate-limit UX. No state lib / form lib / UI kit chosen yet.

## Modules 1–12 (design + code pending)

Organization → Employee → Attendance/Leave → Projects → Meetings → Approvals → Notifications (consumes all identity events for emails) → Assets → Payroll → Analytics (depends all) → AI (depends all) → IoT. All consume Module 0 contracts: User Resolution, Permission Check, Tenant Resolution, domain events.

## Suggested build order

1. Fix `tenant.go` deps: add 11 models so backend compiles.
2. 12 migrations + `cmd/main.go` + compose infra (PG/Redis/RabbitMQ) so API can boot.
3. DTOs → repos → services (Tenant → Auth → User → Role) → controllers → middleware → events.
4. Frontend identity (login + guards + user/role pages) against live API.
5. ER review → Module 1 Organization design.
