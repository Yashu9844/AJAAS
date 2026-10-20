# M0 Identity & Access — Decisions

2026-09-30 — Scaffold: 17-file operational bundle layered onto the 16-file spec base on branch loop_engineering (docs-only, zero production impact). Reason: MASTER_PROMPT Step 4 — loop needs module context to start. Alternative: write all operational files during G0-1. Tradeoff: bundle exists before code; content filled with real repo facts, no placeholders.
2026-09-30 — plan.md owns phase truth; current-goal.md mirrors CURRENT row; implementation-checklist.md remains canonical task list; todo.md is the operational mirror with Goal IDs. Reason: three artifacts serve different readers (loop tick, human scan, phase tracker) without triple-maintenance — goal text written once in plan.md, mirrored by reference.
2026-10-02 — identity.Module gained read-only accessors (UserService, AuditService, UserRoleRepository, RolePermissionRepository, TenantResolver, AuthMiddleware) for dependent modules. Reason: HR/Organization route registration needs identity-owned middleware + RBAC repos without re-constructing them; single DI instance kept in main.go. Alternative: main.go rebuilds repos/services by hand (duplicate wiring). Tradeoff: internal Go API surface grows; no REST contract change. Mirrors module-1 decisions.md entry.

### D-HARDEN-1 (2026-10-06): login latency budget
bcrypt cost 12 (NFR-SEC001) costs ~400 ms; the <300 ms login p95 budget is relaxed to <600 ms. Security takes precedence.

### D-HARDEN-2: platform routes use an operator key, not a JWT
There is no super-admin identity in the data model. `/tenants*` are guarded by `X-Platform-Key` (config `platform.admin_key`, env `PLATFORM_ADMIN_KEY`), constant-time compared, fail-closed when unset. A real super-admin realm can replace it later without changing the tenant API.

### D-HARDEN-3: SQL migrations are the schema authority
`database.RunMigrations` applies `migrations/*.sql` (embedded) at start-up. GORM AutoMigrate (`database.auto_migrate: true`) is legacy/dev only because its constraints drift from the SQL files (it made business codes globally unique).

### D-HARDEN-4: cross-module rules are synchronous hooks
No broker consumer exists, so user deactivation converges org mappings and employee profiles via in-process hooks inside the same DB transaction (`UserDeactivationConverger`). Events are still published for future consumers.
2026-10-08 — Cross-module record (from HR/Attendance & Leave3-10): HR/Attendance & Leave boot inserts `attendance:read`, `attendance:manage`, `attendance:approve` into the M0 Identity `permissions` table (ON CONFLICT DO NOTHING) using identity `models.Permission`. No M0 Identity code or REST contract changes. Reason: no deploy-time permission seed exists (AS-007). Tradeoff: HR/Attendance & Leave depends on the permissions table shape (resource, action, description).

## Changelog

2026-09-30 — Added: operational bundle (agent.md, current-goal.md G0-1, plan.md P1 CURRENT, current-status.md, todo.md, handoff.md, assumptions/decisions/changelog/files entries). Reason: MASTER_PROMPT Step 4 on branch loop_engineering. Tests: structure validation. Impact: docs only, no contract change.

### 2026-10-06 — hardening pass (branch `fix/module-0-2-hardening`)
Verification-console findings fixed and pinned by tests; full list in the hardening report in `docs/modules/m0-identity-access/testing.md`. Highlights: logout + refresh revocation, platform-key protected tenant routes, admin bootstrap, permission seed, non-escalating grants, audit read API, `/auth/me`, activate/unassign endpoints, atomic transactions, standard error envelope, atomic rate limiter, SQL migration runner.
Sensors: build/vet/gofmt clean; unit + integration + golden + security tests pass; coverage 86.5 % (services 93-97 %); migrations up/down/up clean.

2026-10-20 — Access model adopted (owner-approved; `docs/ARCHITECTURE.md` §17, `plan.md` §3 B0): scoped grants,
role inheritance, predefined and automatic roles, per-user grants with expiry and blocks (blocks win), access
version for cache invalidation, explain-access API. Legacy `RequirePermission` requires tenant scope so no existing
route widens; scoped routes opt in through `RequireScopedPermission`.
