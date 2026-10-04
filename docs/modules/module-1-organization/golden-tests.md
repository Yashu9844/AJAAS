# Module 1 — Golden Tests (FROZEN candidates at P1 — protected at P2)

Rules: MASTER_PROMPT §15. Never edit to make red pass. Change needs reason → spec update → contract version → dependent update → full suite.

| # | Golden behavior | Input | Expected | Why | Location (P2) |
|---|---|---|---|---|---|
| G1 | Tenant isolation on every org endpoint | UserA@TenantA lists/reads/writes TenantB dept/team/mapping IDs | 404 everywhere; JWT-A on subdomain-B → 403 | Isolation is the security boundary | tests/api/org_isolation_test.go |
| G2 | Hierarchy stays acyclic | Set dept parent = self / descendant; manager cycle A→B→A | 400 self; 409 cycle; no partial write (read-back unchanged) | Cycles corrupt the whole forest | tests/api/org_hierarchy_test.go |
| G3 | Depth cap enforced | Chain 10 deep, add 11th child | 422 HIERARCHY_TOO_DEEP + chain; tree unchanged | Unbounded depth kills chart reads | tests/api/org_hierarchy_test.go |
| G4 | RBAC allow/deny | Caller without `organization:update` attempts write; with it succeeds | 403 vs 200/201 | Structure writes are privileged | tests/api/org_rbac_test.go |
| G5 | Deactivate-while-referenced blocked | Deactivate dept with active teams/mappings (no force) | 409 + conflicting IDs; force=true deactivates children only | Prevents orphaned structure | tests/api/org_lifecycle_test.go |
| G6 | Single primary mapping | Two primaries for one user | 409; explicit make_primary swap is atomic (exactly one primary after) | Payroll/approvals assume one primary | tests/api/org_mapping_test.go |
| G7 | UserDeactivated converges | Deactivate user with mappings + reports | Mappings inactive, manager refs cleared/reassigned, audit entries exist | Identity↔org consistency | tests/api/org_events_test.go |
| G8 | Team move keeps mappings consistent | Move team across departments | 200; mappings follow team; audit has old/new dept | Moves must not orphan people | tests/api/org_mapping_test.go |
| G9 | Audit on every state change | Any FR-D/T/DG/H/M write | Immutable audit row with actor/resource/diff, no secrets | Repudiation defense | tests/api/org_audit_test.go |
| G10 | No secret/hash leakage | All org responses | No password_hash/token_hash/secrets in any payload | Response hygiene | tests/security/org_leak_test.go |
