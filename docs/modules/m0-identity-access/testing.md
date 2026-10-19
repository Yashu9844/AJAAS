# M0 Identity & Access — Testing

> Design-time contract for M0 Identity & Access, merged from the former 26-file module-0 bundle. Where it differs from code, the code and `docs/SYSTEM_REFERENCE.md` (generated) win; known differences and target-architecture gaps are tracked in `plan.md`.

## Golden tests (protected)

Never edit to make a red test pass (docs/ENGINEERING.md (golden tests)). Implemented in `backend/tests/integration`; see the golden test locations section of this file.

| ID | Golden behaviour | Expected |
|---|---|---|
| G0-1 | Tenant isolation | JWT of tenant A on host B → 403; foreign ids → 404; no cross-tenant list leak |
| G0-2 | RBAC enforcement | Missing permission → 403; grant/revoke effective immediately; tenant_admin bypass |
| G0-3 | Platform routes | `/tenants*` need `X-Platform-Key` (401/403); 503 when unconfigured |
| G0-4 | Token lifecycle | Refresh rotates; reuse of a rotated token revokes all sessions; logout revokes access + refresh |
| G0-5 | No privilege escalation | Non-admins cannot assign tenant_admin or hand out permissions they do not hold |
| G0-6 | Secret hygiene | No password/token hash in any payload |
| G0-7 | Error envelope | `{error:{code,message,details}}` everywhere |
| G0-8 | Audit | Successful mutations audited (readable via `GET /audit-logs`), failures not |

## Test strategy

---

### Testing Philosophy

- Every layer has its own test type.
- Tests are independent — no test depends on the execution order of another test.
- Tests are repeatable — running them multiple times produces the same result.
- External dependencies (PostgreSQL, Redis, RabbitMQ) are either mocked (unit tests) or provided via Docker (integration tests).
- All tests must pass in CI before merge.

---

### Test Pyramid

```
         ┌─────────────────┐
         │   E2E / Load    │  ← Minimal (manual + automated)
         │    Tests        │
         ├─────────────────┤
         │  API / Integration │  ← Moderate coverage
         │     Tests          │
         ├─────────────────────┤
         │     Unit Tests       │  ← Maximum coverage
         └──────────────────────┘
```

---

### 1. Unit Tests

#### Scope

Unit tests cover individual functions and methods in isolation. All external dependencies (database, cache, queue) are mocked.

#### Service Layer Tests

| Service | Test Cases | Coverage Target |
|---------|-----------|----------------|
| TenantService | Create (valid, duplicate slug, duplicate domain, reserved slug, invalid status), Get (found, not found, deleted), List (empty, paginated), Update (partial, immutable slug), Activate (valid, already active, not found), Suspend (valid, already suspended, not found) | 90% |
| AuthService | Login (success, wrong password, wrong email, wrong tenant, suspended tenant, inactive user, deleted user), Logout (success, invalid session), Refresh (success, expired, revoked/reuse detection, inactive user, suspended tenant), ForgotPassword (valid, non-existent email), ResetPassword (success, expired token, used token, invalid token, password mismatch) | 95% |
| UserService | Create (valid, duplicate email, invalid roles, cross-tenant roles), Get (found, not found, different tenant), List (paginated), Update (partial), Deactivate (valid, already inactive, not found), Invite (valid, duplicate) | 90% |
| RoleService | Create (valid, duplicate name), Update (valid, system role), Delete (valid, system role), AssignPermissions (valid, idempotent, invalid IDs), AssignRoles (valid, idempotent, invalid IDs) | 90% |
| SessionService | Create, Validate (valid, expired, revoked), Revoke, RevokeAll | 90% |
| TokenService | GenerateRefreshToken, ValidateRefreshToken (valid, expired, revoked), RotateRefreshToken, GeneratePasswordResetToken, ValidatePasswordResetToken | 90% |
| AuditService | Create (valid), immutability verification | 85% |

#### Mocking Strategy

| Dependency | Mock Method |
|-----------|-------------|
| Repository interfaces | `gomock` or manual mock structs implementing the interface |
| Redis client | Interface wrapper with mock implementation |
| RabbitMQ publisher | Interface wrapper with mock implementation |
| Time | Inject `clock` interface for deterministic time-based tests |
| UUID generation | Inject `uuid.Generator` interface for deterministic IDs |

#### Test File Naming

```
internal/identity/services/tenant_service_test.go
internal/identity/services/auth_service_test.go
internal/identity/services/user_service_test.go
internal/identity/services/role_service_test.go
```

#### Execution

```bash
go test ./internal/identity/services/... -v -count=1
go test ./internal/identity/services/... -cover
```

---

### 2. Integration Tests

#### Scope

Integration tests verify that components work together correctly. They use real infrastructure (PostgreSQL, Redis, RabbitMQ) via Docker.

#### Repository Integration Tests

| Repository | Test Cases |
|-----------|-----------|
| TenantRepo | CRUD operations, unique constraint violations (slug, domain), soft delete, find by slug |
| UserRepo | CRUD, unique constraint (tenant_id, email), find by email, soft delete, pagination |
| RoleRepo | CRUD, unique constraint (tenant_id, name), soft delete |
| PermissionRepo | CRUD, unique constraint (resource, action) |
| UserRoleRepo | Create, find by user, unique constraint, cascade behavior |
| RolePermissionRepo | Create, find by role, unique constraint |
| SessionRepo | Create, find by ID, revoke, revoke all for user |
| RefreshTokenRepo | Create, find by hash, revoke, revoke all for user |
| PasswordResetTokenRepo | Create, find by hash, mark as used |
| AuditLogRepo | Create, find by tenant, verify no update/delete capability |

#### Test Database Setup

```go
// Use testcontainers-go to spin up PostgreSQL
container, _ := postgres.RunContainer(ctx,
    testcontainers.WithImage("postgres:15"),
    postgres.WithDatabase("jaas_test"),
)

// Run migrations
migrate.Up(db)

// Each test uses a transaction that is rolled back
tx := db.Begin()
defer tx.Rollback()
```

#### Execution

```bash
go test ./internal/identity/repositories/... -v -tags=integration
```

---

### 3. API Tests

#### Scope

Full HTTP request/response cycle tests against a running application instance with real infrastructure.

#### Test Scenarios

##### Authentication Flow

| Test | Steps | Expected |
|------|-------|----------|
| Full login → access → refresh → logout | 1. Login 2. Call protected endpoint 3. Wait/force expiry 4. Refresh 5. Call protected endpoint again 6. Logout | Each step succeeds with correct status codes |
| Login with invalid credentials | 1. Login with wrong password | 401 with generic message |
| Login against suspended tenant | 1. Create tenant 2. Suspend tenant 3. Attempt login | 403 |
| Token reuse attack | 1. Login 2. Refresh (get new token) 3. Refresh with old token | 401, all sessions revoked |

##### Tenant Isolation

| Test | Steps | Expected |
|------|-------|----------|
| Cross-tenant data access | 1. Create Tenant A with User A 2. Create Tenant B with User B 3. User A tries to get User B | 404 (not found in Tenant A) |
| Cross-tenant role assignment | 1. Create role in Tenant A 2. Try to assign Tenant A's role to Tenant B's user | 400/404 |

##### RBAC Flow

| Test | Steps | Expected |
|------|-------|----------|
| Permission grant and use | 1. Create role with `users:read` 2. Assign to user 3. User calls GET /users | 200 |
| Permission denial | 1. Create role without `users:create` 2. Assign to user 3. User calls POST /users | 403 |
| Permission revocation | 1. Remove permission from role 2. User calls endpoint | 403 (within cache TTL) |

##### Password Reset Flow

| Test | Steps | Expected |
|------|-------|----------|
| Full reset flow | 1. Forgot password 2. Capture token from event/DB 3. Reset with new password 4. Login with new password | All succeed |
| Expired token | 1. Create token 2. Manually expire it 3. Attempt reset | 400 |

#### Test Framework

```go
// Use httptest or a real HTTP client
router := setupRouter()
server := httptest.NewServer(router)
defer server.Close()

resp, _ := http.Post(server.URL+"/api/v1/auth/login", "application/json", body)
assert.Equal(t, http.StatusOK, resp.StatusCode)
```

#### Execution

```bash
go test ./tests/api/... -v -tags=api
```

---

### 4. Security Tests

#### Scope

Tests specifically targeting security requirements and attack vectors.

#### Test Scenarios

| Test | Attack Vector | Expected |
|------|--------------|----------|
| JWT tampering | Modify JWT payload, re-sign with different key | 401 |
| JWT expiry bypass | Use expired JWT | 401 |
| Cross-tenant JWT | Use JWT from Tenant A on Tenant B's subdomain | 403 |
| SQL injection | `email: "admin@acme.com'; DROP TABLE users;--"` | 400 (validation) or safe parameterized query |
| Password in response | Create user, check response body | No `password_hash` field |
| Password in logs | Create user, check application logs | No password logged |
| Email enumeration via login | Login with non-existent email vs wrong password | Same error message |
| Email enumeration via forgot-password | Request reset for non-existent vs existent email | Same response |
| Rate limiting | Send 11 login requests in 15 min | 429 on 11th |
| Refresh token reuse | Use revoked refresh token | 401 + all sessions revoked |
| Brute force | Rapid consecutive login attempts | Rate limited after threshold |
| Information leakage | Check all error responses for stack traces or internal details | No internal details exposed |

#### Execution

```bash
go test ./tests/security/... -v -tags=security
```

---

### 5. Performance Tests

#### Scope

Verify that the system meets NFR performance targets under load.

#### Test Scenarios

| Endpoint | Method | Concurrent Users | Duration | Target (p95) |
|----------|--------|-----------------|----------|--------------|
| /auth/login | POST | 100 | 60s | < 300ms |
| /users | GET | 100 | 60s | < 500ms |
| /users/{id} | GET | 100 | 60s | < 200ms |
| /roles | GET | 100 | 60s | < 500ms |
| /permissions | GET | 100 | 60s | < 200ms |

#### Tools

- **k6** (preferred) for HTTP load testing
- **wrk** as an alternative

#### Sample k6 Script

```javascript
import http from 'k6/http';
import { check, sleep } from 'k6';

export let options = {
  stages: [
    { duration: '10s', target: 50 },
    { duration: '30s', target: 100 },
    { duration: '10s', target: 0 },
  ],
  thresholds: {
    http_req_duration: ['p(95)<300'],
  },
};

export default function () {
  let payload = JSON.stringify({
    email: 'test@acme.com',
    password: 'TestP@ss1',
    tenant_slug: 'acme',
  });

  let res = http.post('http://localhost:8080/api/v1/auth/login', payload, {
    headers: { 'Content-Type': 'application/json' },
  });

  check(res, {
    'status is 200': (r) => r.status === 200,
  });

  sleep(1);
}
```

#### Execution

```bash
k6 run tests/performance/login_load_test.js
```

---

### 6. Load Tests

#### Scope

Verify system stability under sustained high traffic.

#### Scenarios

| Scenario | Setup | Duration | Goal |
|----------|-------|----------|------|
| Sustained load | 200 concurrent users, mixed read/write | 10 minutes | No errors, p95 < thresholds |
| Spike test | Ramp from 0 to 500 users in 30s | 2 minutes | No crashes, graceful degradation |
| Soak test | 50 concurrent users | 1 hour | No memory leaks, stable response times |

#### Monitoring During Load

- Application memory usage
- Database connection pool utilization
- Redis connection count
- RabbitMQ queue depth
- Error rate
- Response time percentiles (p50, p95, p99)

---

### 7. Failure Tests

#### Scope

Verify system behavior when external dependencies fail.

#### Scenarios

| Scenario | Failure Injection | Expected Behavior |
|----------|------------------|-------------------|
| Database down | Stop PostgreSQL | All API calls return 503. Application does not crash. |
| Database slow | Add 5s latency to PostgreSQL | Request timeouts return 504. Other requests succeed. |
| Redis down | Stop Redis | Session validation falls back to PostgreSQL. Rate limiting may be temporarily disabled. Application does not crash. |
| RabbitMQ down | Stop RabbitMQ | Synchronous operations succeed. Events fail to publish and are logged. Application does not crash. |
| Database connection pool exhausted | Set max connections to 2, send 10 concurrent requests | Some requests queue, some return 503. No data corruption. |
| Network partition | Block network between app and database | Requests fail with appropriate errors. Automatic reconnection when network recovers. |

#### Execution

- Use Docker Compose to selectively stop services
- Use `tc` (traffic control) or `toxiproxy` for network failure simulation

---

### Test Coverage Goals

| Layer | Target Coverage | Tool |
|-------|----------------|------|
| Services | ≥ 90% | `go test -cover` |
| Controllers | ≥ 80% | `go test -cover` |
| Middleware | ≥ 85% | `go test -cover` |
| Repositories | ≥ 80% | `go test -cover` (integration) |
| Overall Module | ≥ 80% | `go test -cover ./internal/identity/...` |

---

### CI Pipeline Integration

```yaml
# .github/workflows/test.yml
stages:
  - lint:
      - go vet ./...
      - golangci-lint run
  - unit:
      - go test ./internal/identity/services/... -v -cover
      - go test ./internal/identity/controllers/... -v -cover
      - go test ./internal/identity/middleware/... -v -cover
  - integration:
      - docker-compose up -d postgres redis rabbitmq
      - go test ./internal/identity/repositories/... -v -tags=integration
      - go test ./tests/api/... -v -tags=api
  - security:
      - go test ./tests/security/... -v -tags=security
  - race:
      - go test -race ./internal/identity/...
  - coverage:
      - go test -coverprofile=coverage.out ./internal/identity/...
      - go tool cover -func=coverage.out | grep total
```

## Golden test locations (integration suite)

Golden behaviours run in `backend/tests/integration` against the production router. Older locations that mention `tests/api/*` are superseded by this table.

| ID | Behaviour | Test |
|---|---|---|
| G0-1 | Tenant isolation: token only valid on its own host; foreign ids are 404; no list leaks | `TestTenantIsolation` |
| G0-2 | RBAC allow/deny per `resource:action`, takes effect immediately, revocation too | `TestRBACEnforcement` |
| G0-3 | Platform routes need the platform key; fail closed | `TestTenantRoutesRequirePlatformKey` |
| G0-4 | Token lifecycle: rotation, reuse detection, logout revokes access + refresh | `TestRefreshRotationReuseAndLogout` |
| G0-5 | No privilege escalation through grants | `TestPrivilegeEscalationIsBlocked` |
| G0-6 | No secret/hash in any payload | `TestGoldenNoSecretLeakage` |
| G0-7 | One error envelope, no validator internals | `TestErrorEnvelopeIsUniform` |
| G0-8 | Audit trail written for successful mutations only, tenant-scoped | `TestAuditTrail` |

## Hardening report (2026-10-06, M0 + HR/Organization + HR/Employee)

Branch `fix/module-0-2-hardening` (local only, not pushed). Starting point: the findings in `VERIFICATION_REPORT.md`.
Every defect below was reproduced first (by the `/dev-test` console or a failing Go test), fixed, and is now pinned by an
automated test. Sensor results at the bottom.

### Findings → fixes

| # | Severity | Finding (verification report) | Fix | Pinned by |
|---|---|---|---|---|
| 1 | Critical | Logout always 401 (`tenant_id` missing) | `Authenticate` sets tenant from the verified JWT when no host resolver ran | `TestRefreshRotationReuseAndLogout` |
| 1b | Critical | Refresh token survived logout | refresh tokens are linked to the session (`session_id`, `revoked_reason`); logout revokes them; a logged-out token is a plain 401 (not a theft signal, other sessions unaffected) | same + `TestAuthService_RefreshAfterLogoutIsPlain401NotReuse` |
| 2 | Critical | `/tenants*` open to anonymous | `X-Platform-Key` (`PLATFORM_ADMIN_KEY`) middleware, constant-time compare, fails closed (503) when unset | `TestTenantRoutesRequirePlatformKey`, `TestPlatformAdmin` |
| 3 | Critical | `?unmasked=true` leaked PII to any `employee:read` user | server-side `employee:read_sensitive` check via `PermissionFlag`; request alone never unmasks; unmasked reads are audited | `TestEmployeeStatutoryAndPIIMasking` |
| 4 | High | Statutory upsert 500 (schema drift) | model/migration unique index aligned; app now applies versioned SQL migrations (`database.RunMigrations`), AutoMigrate is opt-in | `TestEmployeeStatutoryAndPIIMasking`, `TestMigrationsAreIdempotentAndComplete` |
| 5 | High | Duplicate dept/team/designation code → 500 | GORM `TranslateError` + `errors.Normalize` map unique/FK violations to 409/400 for all modules | `TestDepartmentLifecycleAndHierarchy`, `TestTeamLifecycle`, `TestDesignationLifecycle` |
| 6 | High | Unknown permission → 500 + partial role | validate-then-write, **every mutating request runs in one DB transaction** (`database.BeginTx/Finish`) | `TestRolesAndPermissions` (no partial role) |
| 7 | High | `roles:null` / `permissions` missing | batched preloads (`FindByUserIDs`, `FindByRoleIDs`) | `TestUserLifecycle`, `TestRolesAndPermissions` |
| 8 | Medium | Dept deactivation counted inactive teams | count ACTIVE teams only | `TestDepartmentLifecycleAndHierarchy` |
| 9 | Medium | Org chart stale for 60 s after writes | cache invalidated on every org write and on user-deactivation convergence | `TestOrgChartIsReadYourWritesConsistent` |
| 10 | Medium | `emergency_contacts` plain text → 500 | validated as JSON array of `{name,relation,phone}` (400 otherwise) | `TestEmployeeSelfService`, validators tests |
| 11 | High | No status state machine; exit did not deactivate user | FR-ED002 matrix, FR-ED003/004/005 effects (confirmation, tentative exit date, exit stamp, user deactivation in same tx) | `TestEmployeeStatusStateMachine`, `TestApplyStatusEffects` |
| 11b | High | FR-EV001 not implemented (no consumer anywhere) | synchronous convergence hook: user deactivation → employee inactive + timeline, no broker needed | `TestUserDeactivationConvergesEmployee` |
| 12 | Medium | `POST /mappings/:id/deactivate` needed a body | body optional | `TestMappingRulesAndUserDeactivationConvergence` |
| 13 | High | No admin bootstrap / permission seed / activate / unassign / audit read / "my permissions" | tenant create accepts `admin`; permission catalogue seeded idempotently; `POST /users/:id/activate`, `DELETE /users/:id/roles/:rid`, `DELETE /roles/:id/permissions/:pid`, `GET /audit-logs`, `GET /auth/me` | `TestTenantAdminProvisioningIsAtomic`, `TestAssignAndRemoveRoles`, `TestAuditTrail`, `TestAuthMe` |
| 14 | Medium | Mixed error shapes, Go validator internals in messages | one envelope everywhere, field-level `details` | `TestErrorEnvelopeIsUniform` |
| 15 | Medium | Employee list meta differed | `total_items`/`total_pages` everywhere; pagination clamped (per_page=0 used to risk a divide-by-zero) | `TestPaginationIsClampedEverywhere` |
| 16 | Low | Rate limiter non-atomic, sliding TTL, counted successes | atomic Lua fixed-window; `FailureRateLimiter` for login; `X-Forwarded-For` ignored unless `TRUSTED_PROXIES` | `TestLoginRateLimitCountsFailuresOnly`, limiter unit tests |

### Additional issues found while hardening (not in the first report)

| Severity | Issue | Fix | Test |
|---|---|---|---|
| Critical | **Privilege escalation**: `users:update` could self-assign `tenant_admin`; `roles:update` could grant any permission | grants are non-escalating: only tenant_admin may assign tenant_admin; non-admins may only assign roles / attach permissions they hold | `TestPrivilegeEscalationIsBlocked`, `grants_test.go` |
| High | Document verified through another employee's URL | verify checks the document belongs to the path employee | `TestEmployeeDocuments`, service test |
| High | Employee/department/designation **codes were globally unique** under AutoMigrate (tenant A blocked tenant B) | tenant-scoped constraints; SQL migrations are now the schema authority | `TestBusinessCodesAreTenantScoped` |
| High | Moving a team left its mappings pointing at the old department | mappings follow the team | `TestGoldenM1_TeamMoveKeepsMappingsConsistent` |
| Medium | Refresh token TTL ignored config (hard-coded 7 d); session TTL hard-coded 15 min | both follow config | `TestTokenService_RefreshTTLAndValidation`, `TestSessionServiceTTL` |
| Medium | Login timing revealed whether an email exists | dummy bcrypt for unknown users | `TestAuthService_RulesAndTimingPath` |
| Medium | Last `tenant_admin` could be removed; users could deactivate themselves; roles in use could be deleted | guards (409) | `TestAssignAndRemoveRoles`, `TestUserLifecycle` |
| Medium | No body-size limit / security headers; CORS `*` with credentials | 1 MiB limit, nosniff/DENY/no-store/CSP, explicit CORS allow-list | `TestSecurityHeadersAndBodyLimit`, CORS tests |
| Medium | Docker image could not build (Go 1.22 vs go.mod 1.25), compose used env names the app ignores, volume hid the binary | fixed Dockerfile (non-root) + compose | manual `docker compose config` |
| Low | Empty `jwt.secret` accepted in production | `ValidateConfig` refuses < 32 chars in production | build-time check |
| Low | `/dev-test` console reachable in production builds | `notFound()` unless `NEXT_PUBLIC_ENABLE_DEV_TEST=1` | `next build` |

### Sensors (backend)

| Sensor | Result |
|---|---|
| `go build ./...`, `go vet ./...`, `gofmt -l` | clean |
| `go test ./...` (unit + in-process integration/golden/security against real Postgres 15 + Redis 7) | all pass |
| Coverage (`-coverpkg=./internal/...`) | **86.5 %** total; services: identity **97.0 %**, organization **96.8 %**, employee **93.6 %** |
| `grep -r TODO` (non-test Go) | empty |
| Migrations up → down → up | clean (25 migrations); schema parity checked against GORM models (only int/bigint width differs) |
| p95 latency (local) | single GET 7 ms, list users 14 ms, org-chart 6 ms, list employees 18 ms — **login 431 ms** |
| `-race` | **not run**: no C compiler (CGO) on this machine; run `go test -race ./...` in CI (see below) |
| Frontend `tsc`, `eslint`, `next build` | clean |

#### Known, intentional deviations
* **Login p95 > 300 ms** — bcrypt cost 12 (security.md NFR-SEC001) costs ~400 ms by itself; the two requirements are mathematically incompatible. Security wins; the performance budget for login should be relaxed to ~600 ms (decision recorded in module-0 `decisions.md`).
* Event bus: nothing consumes broker events yet; the two cross-module rules (user deactivation → org mappings, → employee) are therefore synchronous in-process hooks, executed in the same DB transaction. RabbitMQ publishing is unchanged.
* `tests/api` (needed a hand-started server and a hard-coded docker container name) was replaced by `tests/integration`, which boots the production router in-process.

### Operating it
* Dev: `docker compose up -d postgres redis`, create a fresh database, `DATABASE_DBNAME=<db> go run ./cmd` — schema is applied from `migrations/*.sql` automatically; `PLATFORM_ADMIN_KEY` defaults to a dev value in `configs/development.yaml`.
* Production: set `JWT_SECRET` (≥ 32 chars), `PLATFORM_ADMIN_KEY`, `CORS_ALLOWED_ORIGINS`, `TRUSTED_PROXIES` (if behind a proxy). `database.auto_migrate` stays false.
* Integration suite: `go test ./tests/integration` (needs Postgres + Redis; set `JAAS_REQUIRE_INTEGRATION=1` in CI so missing infrastructure fails instead of skipping). It drops/recreates database `jaas_itest` and flushes Redis DB 15.
* CI should additionally run `go test -race ./...` on a CGO-enabled runner.
