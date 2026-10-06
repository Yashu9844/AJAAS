# Modules 0–2 hardening report

Branch `fix/module-0-2-hardening` (local only, not pushed). Starting point: the findings in `VERIFICATION_REPORT.md`.
Every defect below was reproduced first (by the `/dev-test` console or a failing Go test), fixed, and is now pinned by an
automated test. Sensor results at the bottom.

## Findings → fixes

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

## Additional issues found while hardening (not in the first report)

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

## Sensors (backend)

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

### Known, intentional deviations
* **Login p95 > 300 ms** — bcrypt cost 12 (security.md NFR-SEC001) costs ~400 ms by itself; the two requirements are mathematically incompatible. Security wins; the performance budget for login should be relaxed to ~600 ms (decision recorded in module-0 `decisions.md`).
* Event bus: nothing consumes broker events yet; the two cross-module rules (user deactivation → org mappings, → employee) are therefore synchronous in-process hooks, executed in the same DB transaction. RabbitMQ publishing is unchanged.
* `tests/api` (needed a hand-started server and a hard-coded docker container name) was replaced by `tests/integration`, which boots the production router in-process.

## Operating it
* Dev: `docker compose up -d postgres redis`, create a fresh database, `DATABASE_DBNAME=<db> go run ./cmd` — schema is applied from `migrations/*.sql` automatically; `PLATFORM_ADMIN_KEY` defaults to a dev value in `configs/development.yaml`.
* Production: set `JWT_SECRET` (≥ 32 chars), `PLATFORM_ADMIN_KEY`, `CORS_ALLOWED_ORIGINS`, `TRUSTED_PROXIES` (if behind a proxy). `database.auto_migrate` stays false.
* Integration suite: `go test ./tests/integration` (needs Postgres + Redis; set `JAAS_REQUIRE_INTEGRATION=1` in CI so missing infrastructure fails instead of skipping). It drops/recreates database `jaas_itest` and flushes Redis DB 15.
* CI should additionally run `go test -race ./...` on a CGO-enabled runner.
