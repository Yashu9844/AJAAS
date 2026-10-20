# HR / Organization — Decisions

> Part of **M1 HR Management** (bounded context `organization`). Formerly the stand-alone "module-1-organization" bundle; module numbers in older text were renamed to the canonical M0–M12 / HR-context names.

Format: Decision / Reason / Alternatives / Tradeoff.

2026-09-30 — Scaffold: 17-file bundle created before design (docs-only, zero production impact). Reason: loop engineering needs module context to start. Alternative: design all modules upfront. Tradeoff: bundles for unbuilt modules stay thin until P1.

2026-10-01 (P1 design) —
- Decision: Separate HR/Organization with own tables (not columns on users). Reason: org has independent lifecycle, hierarchy invariants, 4 consumers. Alternatives: extend user model; separate microservice. Tradeoff: more tables, but identity stays decoupled and deployable.
- Decision: Logical (service-checked) FK to users, not hard DB FK. Reason: M0 Identity owns users; hard cross-module FKs freeze independent deploys. Alternatives: hard FK; duplicate user snapshot. Tradeoff: convergence via UserDeactivated event + write-time checks instead of DB enforcement.
- Decision: No auto-mapping on UserCreated (explicit mapping only). Reason: org placement is a human decision with RBAC + audit; auto-placement guesses wrong silently. Alternatives: auto-map to default dept/team. Tradeoff: extra mapping step per user, recorded in UX.
- Decision: Computed org-chart + 60s cache (not materialized). Reason: v1 scale fits compute; invalidation trivial; no dual-write drift. Alternatives: materialized path/closure table. Tradeoff: revisit if NFR-P003 breaches.
- Decision: Append-only mappings (status flag, no hard delete). Reason: history needed for audit/payroll/approvals. Alternatives: hard delete + audit only. Tradeoff: table growth; mitigated by tenant scoping + pagination.
- Decision: In-process Go calls to M0 Identity (same deployable v1), not HTTP. Reason: single binary, no network hop, contract = Go interfaces + DTOs. Alternatives: HTTP between modules. Tradeoff: revisit at multi-service split (RS256 + HTTP contracts already shaped for it).

2026-10-02 (P2 wiring) —
- Decision: M0 Identity exposes read-only accessors on identity.Module (services, RBAC repos, TenantResolver/AuthMiddleware builders); org consumes them in cmd/main.go. Reason: single DI graph, no duplicated wiring. Alternative: main.go constructs identity internals directly. Tradeoff: small internal API surface addition to M0 Identity (no REST change). Recorded in module-0 decisions.md too.
- Decision: Live schema currently comes from GORM AutoMigrate at boot; raw SQL pairs (000013..000017) are the production path once golang-migrate is wired. Consequence: case-insensitive uniqueness (lower(name)) and partial indexes exist only in SQL — AutoMigrate creates plain unique indexes; service layer enforces case-insensitive pre-checks. Tradeoff: acceptable at dev boot; must wire golang-migrate before production (tracked in current-status known issues).
- Decision: routes.go signature takes (dbForAudit, auditSvc) and builds auditMW internally; module.go passes them through from main.go. Reason: matches identity route style; audit middleware stays identity-owned.

2026-10-09 — Cross-module record (from M7 Approvals D6-03): organization.Module gains read-only accessors DepartmentService() and DesignationService() so Recruitment can validate job references. No behaviour or REST change.

## Changelog

2026-09-30 — Created: 17-file doc bundle scaffold with real dependency metadata. Reason: MASTER_PROMPT Step 4 on branch loop_engineering. Tests: structure validation. Impact: docs only, no contract change.
2026-10-01 — P1 design DONE: specification (FR-D/T/DG/H/M/O/E), architecture, connections C1..C9, goldens G1..G10, security/testing/files/decisions/assumptions, contracts frozen. Reason: G1-1 (M0 Identity merged 3af3d09; context01 next task). Tests: structure check (docs phase). Impact: contracts frozen for P2; no code yet.

### 2026-10-06 — hardening pass (branch `fix/module-0-2-hardening`)
Verification-console findings fixed and pinned by tests; full list in the hardening report in `docs/modules/m0-identity-access/testing.md`. Highlights: duplicate codes → 409, active-team deactivation rule, org-chart cache invalidation, team move keeps mappings consistent, tenant-scoped unique codes, optional deactivate bodies.
Sensors: build/vet/gofmt clean; unit + integration + golden + security tests pass; coverage 86.5 % (services 93-97 %); migrations up/down/up clean.

### 2026-10-20 — locations (HR parity P1)
- DO-L1: a location's state is a code (ISO 3166-2 subdivision without country) rather than a free name, so payroll PT slabs and holiday calendars can key on it.
- DO-L2: time zones are validated against the embedded IANA database (`time/tzdata`) so validation does not depend on the host.
- DO-L3: the worker's location is an optional field of the mapping (FR-L005); core will own effective-dated assignment later (CH-012) and mappings become its projection.
- DO-L4: location writes are audited by the route audit trail only (actor-aware); the service does not write a second, actor-less audit row.
Evidence: migration hr/000047 (up/down/up verified); services 95.7 %, location controller 100 %; `TestOrganizationLocations` green; console `s1.loc` 15 steps; whole console 37/37.
