# Module 1 — Decisions (ADR log)

Format: Decision / Reason / Alternatives / Tradeoff.

2026-09-30 — Scaffold: 17-file bundle created before design (docs-only, zero production impact). Reason: loop engineering needs module context to start. Alternative: design all modules upfront. Tradeoff: bundles for unbuilt modules stay thin until P1.

2026-10-01 (P1 design) —
- Decision: Separate Module 1 with own tables (not columns on users). Reason: org has independent lifecycle, hierarchy invariants, 4 consumers. Alternatives: extend user model; separate microservice. Tradeoff: more tables, but identity stays decoupled and deployable.
- Decision: Logical (service-checked) FK to users, not hard DB FK. Reason: Module 0 owns users; hard cross-module FKs freeze independent deploys. Alternatives: hard FK; duplicate user snapshot. Tradeoff: convergence via UserDeactivated event + write-time checks instead of DB enforcement.
- Decision: No auto-mapping on UserCreated (explicit mapping only). Reason: org placement is a human decision with RBAC + audit; auto-placement guesses wrong silently. Alternatives: auto-map to default dept/team. Tradeoff: extra mapping step per user, recorded in UX.
- Decision: Computed org-chart + 60s cache (not materialized). Reason: v1 scale fits compute; invalidation trivial; no dual-write drift. Alternatives: materialized path/closure table. Tradeoff: revisit if NFR-P003 breaches.
- Decision: Append-only mappings (status flag, no hard delete). Reason: history needed for audit/payroll/approvals. Alternatives: hard delete + audit only. Tradeoff: table growth; mitigated by tenant scoping + pagination.
- Decision: In-process Go calls to Module 0 (same deployable v1), not HTTP. Reason: single binary, no network hop, contract = Go interfaces + DTOs. Alternatives: HTTP between modules. Tradeoff: revisit at multi-service split (RS256 + HTTP contracts already shaped for it).

2026-10-02 (P2 wiring) —
- Decision: Module 0 exposes read-only accessors on identity.Module (services, RBAC repos, TenantResolver/AuthMiddleware builders); org consumes them in cmd/main.go. Reason: single DI graph, no duplicated wiring. Alternative: main.go constructs identity internals directly. Tradeoff: small internal API surface addition to Module 0 (no REST change). Recorded in module-0 decisions.md too.
- Decision: Live schema currently comes from GORM AutoMigrate at boot; raw SQL pairs (000013..000017) are the production path once golang-migrate is wired. Consequence: case-insensitive uniqueness (lower(name)) and partial indexes exist only in SQL — AutoMigrate creates plain unique indexes; service layer enforces case-insensitive pre-checks. Tradeoff: acceptable at dev boot; must wire golang-migrate before production (tracked in current-status known issues).
- Decision: routes.go signature takes (dbForAudit, auditSvc) and builds auditMW internally; module.go passes them through from main.go. Reason: matches identity route style; audit middleware stays identity-owned.

2026-10-09 — Cross-module record (from Module 6 D6-03): organization.Module gains read-only accessors DepartmentService() and DesignationService() so Recruitment can validate job references. No behaviour or REST change.
