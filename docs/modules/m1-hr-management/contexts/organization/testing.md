# HR / Organization — Testing

> Part of **M1 HR Management** (bounded context `organization`). Formerly the stand-alone "module-1-organization" bundle; module numbers in older text were renamed to the canonical M0–M12 / HR-context names.

## Golden tests (protected)

Rules: docs/ENGINEERING.md (golden tests). Never edit to make red pass. Change needs reason → spec update → contract version → dependent update → full suite.

| # | Golden behavior | Input | Expected | Why | Location (P2) |
|---|---|---|---|---|---|
| G1 | Tenant isolation on every org endpoint | UserA@TenantA lists/reads/writes TenantB dept/team/mapping IDs | 404 everywhere; JWT-A on subdomain-B → 403 | Isolation is the security boundary | tests/integration/ (see Golden test locations below) — formerly tests/api/org_isolation_test.go |
| G2 | Hierarchy stays acyclic | Set dept parent = self / descendant; manager cycle A→B→A | 400 self; 409 cycle; no partial write (read-back unchanged) | Cycles corrupt the whole forest | tests/integration/ (see Golden test locations below) — formerly tests/api/org_hierarchy_test.go |
| G3 | Depth cap enforced | Chain 10 deep, add 11th child | 422 HIERARCHY_TOO_DEEP + chain; tree unchanged | Unbounded depth kills chart reads | tests/integration/ (see Golden test locations below) — formerly tests/api/org_hierarchy_test.go |
| G4 | RBAC allow/deny | Caller without `organization:update` attempts write; with it succeeds | 403 vs 200/201 | Structure writes are privileged | tests/integration/ (see Golden test locations below) — formerly tests/api/org_rbac_test.go |
| G5 | Deactivate-while-referenced blocked | Deactivate dept with active teams/mappings (no force) | 409 + conflicting IDs; force=true deactivates children only | Prevents orphaned structure | tests/integration/ (see Golden test locations below) — formerly tests/api/org_lifecycle_test.go |
| G6 | Single primary mapping | Two primaries for one user | 409; explicit make_primary swap is atomic (exactly one primary after) | Payroll/approvals assume one primary | tests/integration/ (see Golden test locations below) — formerly tests/api/org_mapping_test.go |
| G7 | UserDeactivated converges | Deactivate user with mappings + reports | Mappings inactive, manager refs cleared/reassigned, audit entries exist | Identity↔org consistency | tests/integration/ (see Golden test locations below) — formerly tests/api/org_events_test.go |
| G8 | Team move keeps mappings consistent | Move team across departments | 200; mappings follow team; audit has old/new dept | Moves must not orphan people | tests/integration/ (see Golden test locations below) — formerly tests/api/org_mapping_test.go |
| G9 | Audit on every state change | Any FR-D/T/DG/H/M write | Immutable audit row with actor/resource/diff, no secrets | Repudiation defense | tests/integration/ (see Golden test locations below) — formerly tests/api/org_audit_test.go |
| G10 | No secret/hash leakage | All org responses | No password_hash/token_hash/secrets in any payload | Response hygiene | tests/security/org_leak_test.go |

## Test strategy and commands

Strategy: unit (services ≥90%, overall ≥80%) + integration (Docker PG) + contract/API + security + golden (protected, see golden-tests.md) + performance (NFR-P001..P004).

### Commands

```bash
cd backend
go build ./internal/organization/...
go vet ./internal/organization/...
gofmt -l internal/organization/
go test ./internal/organization/...                 # unit (RED-first, mocked M0 Identity UserService)
go test -race ./internal/organization/...
go test -cover ./internal/organization/...          # ≥90% services, ≥80% module
grep -r "TODO" internal/organization/               # must be empty
# migrations (000013.. per P2): up → tables exist; down → clean; up again
# API E2E (needs live PG/Redis/RabbitMQ): go test ./tests/api/ -run TestOrg
# security: go test ./tests/security/ -run TestOrg
# perf: k6 run tests/performance/org_chart.js  (NFR-P003: p95 <800ms uncached, <100ms cached)
cd ../frontend && npx tsc --noEmit && npm run lint && npm run build   # P3 frontend slice
```

### Suites (P2 writes, P3 runs)

- Unit: department/team/designation/mapping/chart services (cycle/depth/guard logic), validators (code format, reserved), event payload builders.
- Integration: repos vs Docker PG — CRUD, uniques (incl. soft-deleted reservation), soft delete, pagination, children/report traversal, primary-swap atomicity.
- Contract/API: 20 endpoints (§4 frozen routes), envelopes, status map, pagination, filters.
- Security: isolation (G1), RBAC (G4), leak scan (G10).
- Golden: G1..G10 (protected).
- Regression: full M0 Identity suite still green after org wiring (M0 Identity tests re-run at P3).

## Delivery evidence (status and Definition of Done at backend close)

Implemented:
- P1 design bundle (spec, architecture, connections, goldens frozen).
- P2 domain + API code:
  - 4 models (Department, Team, Designation, Mapping) + Outbox model + models_test.
  - 5 migrations (000013..000017) up/down pairs.
  - 6 DTO files + custom validators with unit tests.
  - 4 tenant-scoped repositories (department, team, designation, mapping).
  - 5 domain services (Department, Team, Designation, Mapping, OrgChart) with acyclic hierarchy, max-depth (≤10), single-primary, and manager cycle guards.
  - Event consumer handlers for M0 Identity user deactivation convergence (FR-M005) and lifecycle sync.
  - 5 controllers + routes (20 endpoints protected by `organization:read` / `organization:update` RBAC and AuditLog middleware).
  - Domain events: 7 produced events + outbox pattern.
  - Bootstrap wiring: `cmd/main.go` registers models, automigrates, sets up user deactivation converger, and registers `/api/v1` routes.
  - OpenAPI / Swagger documentation: `backend/api/swagger.yaml` updated with all 20 organization endpoints.
- Sensor status:
  - `go build ./...` clean.
  - `go vet ./...` clean.
  - `gofmt -l` clean.
  - `go test ./internal/...` green across all 28 internal packages.
  - Service coverage: 86.8% in organization services, 83.5% in identity services.
  - Zero TODO comments in code.

Partially Implemented:
- Permissions seed: `organization:read` and `organization:update` established in permissions model and route guards; database seed mechanism pending deployment runner.

Not Implemented:
- P3 integration tests vs live PostgreSQL/Redis/RabbitMQ containers via Docker (`tests/api/` integration suite).
- Frontend organization slice (department tree, team views, org chart viewer).

Known Issues:
- Live container tests require Docker engine to execute `tests/api/` suite against real PostgreSQL.

Blocked:
- PG-backed live Docker execution blocked on environment availability. All unit, domain, controller, and sensor suites are unblocked and green.

## Golden test locations (integration suite)

Golden behaviours run in `backend/tests/integration` against the production router. Older locations that mention `tests/api/*` are superseded by this table.

| ID | Test |
|---|---|
| G1 isolation | `TestTenantIsolation` |
| G2 acyclic, no partial write | `TestDepartmentLifecycleAndHierarchy`, `TestGoldenM1_RejectedHierarchyChangeLeavesNoTrace` |
| G3 depth cap | `TestDepartmentDepthLimit` |
| G4 RBAC | `TestOrgRBAC` |
| G5 deactivate-while-referenced | `TestDepartmentLifecycleAndHierarchy`, `TestDesignationLifecycle` |
| G6 single primary mapping | `TestMappingRulesAndUserDeactivationConvergence` |
| G7 user deactivation converges | `TestMappingRulesAndUserDeactivationConvergence`, `TestOrgChartIsReadYourWritesConsistent` |
| G8 team move keeps mappings consistent | `TestGoldenM1_TeamMoveKeepsMappingsConsistent` |
| G9 audit on every write | `TestGoldenM1_AuditOnEveryOrgWrite` |
| G10 no secret leakage | `TestGoldenNoSecretLeakage` |
| G11 locations (FR-L001..FR-L007): normalization, uniqueness, IANA zones, immutable code, filters, mapping link, deactivation blocked while mapped, inactive not assignable, isolation | `TestOrganizationLocations`; unit `TestLocationService_*`, `TestMappingService_Location`, `TestLocationController`; console `s1.loc` |
