# Module 0 — Decisions (operational ADR additions)

2026-09-30 — Scaffold: 17-file operational bundle layered onto the 16-file spec base on branch loop_engineering (docs-only, zero production impact). Reason: MASTER_PROMPT Step 4 — loop needs module context to start. Alternative: write all operational files during G0-1. Tradeoff: bundle exists before code; content filled with real repo facts, no placeholders.
2026-09-30 — plan.md owns phase truth; current-goal.md mirrors CURRENT row; implementation-checklist.md remains canonical task list; todo.md is the operational mirror with Goal IDs. Reason: three artifacts serve different readers (loop tick, human scan, phase tracker) without triple-maintenance — goal text written once in plan.md, mirrored by reference.
2026-10-02 — identity.Module gained read-only accessors (UserService, AuditService, UserRoleRepository, RolePermissionRepository, TenantResolver, AuthMiddleware) for dependent modules. Reason: Module 1 route registration needs identity-owned middleware + RBAC repos without re-constructing them; single DI instance kept in main.go. Alternative: main.go rebuilds repos/services by hand (duplicate wiring). Tradeoff: internal Go API surface grows; no REST contract change. Mirrors module-1 decisions.md entry.

## D-HARDEN-1 (2026-10-06): login latency budget
bcrypt cost 12 (NFR-SEC001) costs ~400 ms; the <300 ms login p95 budget is relaxed to <600 ms. Security takes precedence.

## D-HARDEN-2: platform routes use an operator key, not a JWT
There is no super-admin identity in the data model. `/tenants*` are guarded by `X-Platform-Key` (config `platform.admin_key`, env `PLATFORM_ADMIN_KEY`), constant-time compared, fail-closed when unset. A real super-admin realm can replace it later without changing the tenant API.

## D-HARDEN-3: SQL migrations are the schema authority
`database.RunMigrations` applies `migrations/*.sql` (embedded) at start-up. GORM AutoMigrate (`database.auto_migrate: true`) is legacy/dev only because its constraints drift from the SQL files (it made business codes globally unique).

## D-HARDEN-4: cross-module rules are synchronous hooks
No broker consumer exists, so user deactivation converges org mappings and employee profiles via in-process hooks inside the same DB transaction (`UserDeactivationConverger`). Events are still published for future consumers.
