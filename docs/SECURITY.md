# JAAS — Security & Code Style Guide

Single enforceable standard for all JAAS code (human or AI).
Supersedes ad-hoc rules. Module specs (`docs/modules/module-0-identity/security.md`, `coding-guidelines.md`) remain detail refs — this file is the law.

Goals: secure by default, readable without essays, self-defining names, small modular services, low latency, smooth UX.

## 1. Security principles

1. Deny by default. Every endpoint requires auth unless explicitly public.
2. Never trust client input. Validate, parameterize, encode.
3. Least privilege. Roles grant minimum `resource:action`. Services get only the repos/clients they need.
4. Defense in depth. Middleware + service checks + DB constraints must all agree.
5. Fail closed. DB down, Redis down, queue down → reject or degrade safely, never bypass auth.
6. No secret in code, logs, responses, or git. Ever.
7. Tenant isolation is a security boundary, not a filter convenience.
8. Every state change is auditable.

Public endpoints (no JWT). Everything else requires JWT:

- `POST /api/v1/auth/login`
- `POST /api/v1/auth/forgot-password`
- `POST /api/v1/auth/reset-password`

## 2. Threat model

### Assets

| Asset | Class | Impact |
|---|---|---|
| Passwords | Critical | Account takeover |
| JWT signing key | Critical | Full auth bypass |
| MFA secrets | Critical | Second-factor bypass |
| Refresh / reset tokens | High | Persistent access |
| Sessions | High | Hijack |
| Tenant data | High | Breach, regulatory |
| Audit logs | Medium | Cover tracks |

### Actors

External attacker (network, automation), malicious tenant user (privilege escalation), compromised insider, credential-stuffing competitor.

### STRIDE

| ID | Threat | Mitigation |
|---|---|---|
| T1 | Stolen JWT replay | 15m TTL, JTI blacklist on logout |
| T2 | JWT tampering | HS256 verify + claims re-checked vs DB |
| T3 | Repudiation | Immutable audit logs (user, IP, time) |
| T4 | Enumeration via login/forgot | Generic messages, same response both paths |
| T5 | Brute force / DoS | Redis sliding-window rate limits |
| T6 | Cross-tenant access | JWT `tid` == subdomain tenant, all queries `tenant_id`-scoped |
| T7 | Refresh theft | Rotation + reuse detection (revoke all) |
| T8 | Hash leak | Never return `password_hash`, `token_hash`, secrets |
| T9 | SQLi | GORM parameterized only, no string concat |
| T10 | Self-promotion | Role assign requires `users:update` |

## 3. Authentication

Flow: `email + password + tenant_slug` → resolve tenant → lookup user in tenant → bcrypt compare → checks → issue tokens + session.

Rules:

- Generic `invalid credentials` for bad tenant, bad email, bad password, soft-deleted user. No existence oracle.
- Reject if tenant != `active` → 403 `tenant is suspended or inactive`.
- Reject if user != `active` → 403 `account is not active`.
- Success: update `last_login_at`, create session, audit `user.login.success`. Failure: audit `user.login.failed` with no password.
- Passwords: bcrypt cost 12 via `golang.org/x/crypto/bcrypt`. Min 8 chars, max 72 bytes (bcrypt limit). Never log, return, or compare with `==`.
- MFA (when enabled): credentials OK → return `mfa_required:true` partial, no tokens. Verify endpoint: 5 fails → 15m lock. Secrets AES-256-GCM at rest.

## 4. Tokens

### JWT access (15m)

- Alg HS256 (migrate to RS256 for multi-service prod). Issuer `jaas-identity`.
- Claims: `sub` (user), `tid` (tenant), `email`, `roles`, `sid` (session), `jti`, `iat`, `exp` (iat+900s), `iss`.
- Secret: 256-bit from `JWT_SECRET` env. Never hardcoded. Support `JWT_SECRET_PREVIOUS` for rotation (verify current, fallback previous).
- Validate on every request: signature → exp → iss → JTI not blacklisted → user exists+active → tenant exists+active → `tid` == subdomain tenant.

### Refresh (7d, opaque, rotated)

1. `crypto/rand` 32 bytes → base64url to client.
2. SHA-256 hash stored in `refresh_tokens.token_hash` (unique).
3. Use → revoke old, issue new pair.
4. Revoked token presented → reuse detected → revoke ALL tokens + sessions for user, 401 + security audit.
5. Bound to one user + one tenant. Logout revokes.

### Password reset (1h, single-use)

Same generation/hashing. `used_at` set on consume. Always return success on forgot (even if email missing). Rate limit 5/hour/email. On reset success: new bcrypt hash + revoke all sessions/tokens + `PasswordReset` event.

## 5. Authorization + tenant isolation

Model: `User → UserRole → Role (tenant-scoped) → RolePermission → Permission (global resource:action)`.

- Permissions: `users:create/read/update`, `roles:create/read/update/delete`, `permissions:read`, `tenants:create/read/update` (Super Admin only).
- Check at request time against DB (JWT roles are hints, not truth). Redis cache 5m, invalidated on `PermissionAssigned/Revoked`.
- Missing permission → 403. No auth → 401.
- Isolation: every tenant-owned row carries `tenant_id`. Every query filters it. JWT `tid` must equal subdomain-resolved tenant. Cross-tenant ID in body is ignored — IDs are always resolved within JWT tenant. Unique scopes: `tenants.slug` global, `users(tenant_id,email)`, `roles(tenant_id,name)` case-insensitive, `permissions(resource,action)`.
- System roles (`is_system=true`, seeded `tenant_admin`, `member`): cannot delete or rename. Delete role cascades joins. Assigns are idempotent.

## 6. Rate limiting + brute force

Redis sliding window, key `rate:{endpoint}:{id}:{window}`. 429 + `Retry-After` + `X-RateLimit-Limit/Remaining/Reset`.

| Endpoint | Limit | Key |
|---|---|---|
| `POST /auth/login` | 10/15m | per email+slug; plus 100/15m per IP |
| `POST /auth/forgot-password` | 5/h | per email+slug |
| `POST /auth/reset-password` | 10/h | per IP |
| `POST /auth/refresh` | 30/15m | per user |
| default | 100/min | per user |

Lockouts (v2): 10 fails → 15m cooldown, 20 → 1h, 30 → admin unlock. MFA: 5 fails → 15m lock.

## 7. Audit logging

Immutable. No UPDATE/DELETE (repo exposes Create + Find only; optional DB trigger to enforce). No `updated_at`/`deleted_at`.

Fields: `tenant_id`, `user_id` (null for failed logins), `action` (`tenant.created`, `user.login.success`, `role.assigned`…), `resource`, `resource_id`, `metadata` JSONB (diffs, never passwords), `ip` (X-Forwarded-For), `user_agent`, `created_at` UTC.

Log: auth, user/role/permission/tenant lifecycle, password flows, `token.reuse_detected`, `rate_limit.exceeded`. Best-effort non-blocking — audit failure never fails the API.

## 8. Data protection + secrets

At rest: passwords bcrypt (one-way), refresh/reset tokens SHA-256 (one-way), MFA secrets AES-256-GCM (two-way), DB filesystem/TDE (infra), Redis AUTH+TLS prod.
In transit: TLS 1.2+ client→Kong; HTTP inside VPC; `sslmode=require` PG prod; TLS Redis/RabbitMQ prod.
Keys: `JWT_SECRET` (rotate 90d), `MFA_ENCRYPTION_KEY` (re-encrypt on rotate), DB password (secrets manager). Empty defaults in `production.yaml` — must inject via env. Never commit.

## 9. API hardening

- Envelope: success `{data, meta}`, error `{error:{code,message,details[]}}`. No stack traces.
- Pagination: `page` ≥1 (default 1), `per_page` 1–100 (default 20). All lists paginated.
- Validation: Gin `ShouldBindJSON` → `validator/v10` tags → 400 with field details. Unknown fields ignored. Malformed JSON → 400.
- Headers on all responses: `X-Content-Type-Options: nosniff`, `X-Frame-Options: DENY`, `Strict-Transport-Security`, `Content-Security-Policy`, `X-Request-ID` (trace).
- CORS: explicit origin whitelist. No `*` in prod.
- No user-controlled URL fetch in Module 0 (no SSRF surface). All JSON responses (no HTML → XSS-safe); still sanitize stored strings on write.
- Kong terminates TLS. App receives plain HTTP internally. Trust `X-Forwarded-For` only behind proxy.

## 10. OWASP Top 10 (2021) mapping

| # | Threat | Mitigation |
|---|---|---|
| A01 | Broken access | RBAC middleware, tenant checks, JWT re-validation |
| A02 | Crypto failures | bcrypt, SHA-256, AES-256-GCM, TLS |
| A03 | Injection | GORM params, DTO validation |
| A04 | Insecure design | Clean Architecture, threat model, review gates |
| A05 | Misconfig | No defaults, env config, strict CORS |
| A06 | Vuln components | `go mod audit`, dependabot, pinned images |
| A07 | Auth failures | Rotation, MFA, rate limits, generic errors |
| A08 | Integrity failures | JWT verify, immutable audit |
| A09 | Logging failures | Audit spec §7, failed-login tracking |
| A10 | SSRF | No fetch of user URLs |

## 11. Secure SDLC (loop sensors)

Every agent loop must pass these before done. Silent success, verbose failure.

Backend: `go build ./...`, `go vet ./...`, `gofmt -l`, `go test -race ./...`, coverage ≥80% (≥90% services), `grep -r TODO` empty, migrations up/down/up clean, 24 endpoint contract tests, isolation + reuse + enumeration security tests.
Frontend: `tsc --noEmit`, `eslint` (no disable comments), `next build`, auth refresh + 403-state tests.
Perf budgets (p95): single GET <200ms, list <500ms, login <300ms, DB query <50ms.
Promote any repeated prose rule to a mechanical check (custom lint, arch test, CI gate). `// eslint-disable-next-line` and `//nolint` are banned without reviewer approval.

## 12. Pre-production checklist

- [ ] 256-bit `JWT_SECRET` in env, not in git; rotation path tested
- [ ] bcrypt cost 12; no plaintext in logs/DB/responses
- [ ] Rotation + reuse detection live; reset tokens 1h single-use
- [ ] Generic login/forgot messages verified identical
- [ ] Rate limits live on all auth endpoints with headers
- [ ] CORS whitelist, TLS everywhere, `sslmode=require` prod
- [ ] All scoped queries include `tenant_id`; JWT `tid` == subdomain
- [ ] Audit immutable; MFA secrets encrypted; JTI blacklist on logout; session expiry enforced
- [ ] DTO validation on all inputs; parameterized queries only; secure headers on
- [ ] Deps scanned; no `panic()` in prod paths; structured logs with `request_id`, no secrets

## 13. Code style — readable, modular, fast

### Readability rules

- Self-defining names beat comments. `RevokeAllUserSessions(ctx, userID)` needs no comment. `DoStuff()` is banned.
- Concise proper comments only: exported symbols get one-line `// Name does X.` Doc comments on all exported Go symbols. Explain WHY when non-obvious (rule ID, e.g. `// PW-008: same response to prevent enumeration`). Never restate the code. No dead code, no TODOs in prod.
- Small units: func ≤50 lines, file ≤300 lines, depth ≤4, params ≤4 (else options struct). One use case per service method. No god methods, no `init()` for wiring, no globals for mutable state, no `fmt.Println`/`log.Println` (structured logger only).
- Errors: Repository returns raw/model errors → Service wraps in `AppError` → Controller maps to HTTP. Never return raw errors to client. Use `errors.Is/As`, never string match. Return early, flat code.

```go
// Login authenticates email+password+slug and issues tokens. AU-001..AU-014.
func (s *authService) Login(ctx context.Context, req dto.LoginRequest) (*dto.LoginResponse, error) {
    tenant, err := s.tenants.FindBySlug(ctx, req.TenantSlug)
    if err != nil || tenant == nil {
        return nil, errors.ErrInvalidCredentials // PW/AU: no existence oracle
    }
    // ... bcrypt compare, status checks, tx: session + tokens + last_login_at
}
```

### Backend layout (Clean Architecture)

`cmd/main.go` → `internal/{identity,organization,employee}/{controllers,dto,events,middleware,models,repositories,routes,services,validators}` → `internal/shared/{cache,config,constants,database,errors,logger,middleware,queue,utils}`. `migrations/*.up/down.sql`, `api/swagger.yaml`, `tests/{api,security,performance}`.

- Naming: files snake_case (`tenant_service.go`, `user_role.go`, `*_test.go`); packages lowercase (`models`, `dto`); types PascalCase (`TenantService`); impl unexported (`tenantServiceImpl`); interfaces behavioral (`TenantRepository`); errors `ErrX`; JSON/DB snake_case; tables plural; routes lowercase-hyphen, plural resources, no trailing slash.
- DTOs: Request ≠ Response structs, in `dto` only, `json` snake_case + `validate` tags, pointer fields for PATCH optionality, never include hashes. Repos take/return models, no business logic, support `*gorm.DB` tx injection. Services take DTOs + `context.Context`, never `*gin.Context`, never HTTP codes. Controllers parse → validate → service → map → status. Constructors inject interfaces; wire in `module.go`/bootstrap. Transactions for multi-table writes; publish events AFTER commit; keep tx short, no I/O inside.

### Frontend (TS + Next.js App Router)

- `src/{app,components,hooks,lib,services,stores,styles,types,utils}` + `src/modules/{identity,organization,employee}/{api,components,pages,hooks,services,types}`. One concern per file; shared fetch wrapper unwraps `{data,meta}` / throws typed `{code,message,details}`; handles 401→refresh→retry once, 429 `Retry-After`, pagination defaults.
- Naming: components PascalCase, hooks `useX`, functions camelCase, types PascalCase, files kebab or matching export. Strict TS, no `any` without guard. Forms validate client + server messages. Guards: `AuthGuard` (logged in), `PermissionGuard(resource, action)`, tenant context from subdomain. Never store JWT in localStorage if httpOnly cookie path chosen — pick one and stick to it. Never render hashes or tokens.

### Latency + smoothness

- Backend: pool 25 open/10 idle/5m; index every `tenant_id` + FK + unique cols; no N+1 (Preload/Dataloader, capped); paginate everything; permission check cached 5m; Redis for sessions/blacklist/rate, PG fallback; short tx; best-effort events (queue down never blocks API); payloads small (no secrets, no blobs); `request_id` + `duration_ms` logs to find slow paths.
- Frontend: server components by default, client only for interactivity; no waterfall fetches (parallelize); paginate/virtualize tables; optimistic UI with rollback on `{error}`; debounce search; skeleton states; respect `Retry-After`; keep bundle lean (React Compiler on).
- DB: UUID PKs, `timestamptz`, soft delete (`tenants,users,roles` only), composite uniques per spec, FKs with correct cascade, down migrations reverse order.

## 14. Review checklist (every PR / loop REVIEW phase)

- [ ] Layers respected: controller thin, service owns logic, repo owns data
- [ ] `tenant_id` on every scoped query; JWT re-validated; RBAC on route
- [ ] No secrets/hashes in code, logs, responses; generic auth errors
- [ ] Tx for multi-write; events after commit; audit entry written
- [ ] DTO validation + tests (unit + contract + isolation); coverage met; race clean
- [ ] Names self-defining; one-line docs on exports; no TODO, no `panic`, no disables
- [ ] Latency budget met; no N+1; pagination; cache invalidated where needed
