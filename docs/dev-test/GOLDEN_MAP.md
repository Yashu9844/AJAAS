# Golden test map (Modules 0–2)

All golden behaviours run in `backend/tests/integration` against the production router. Locations in
`docs/modules/*/golden-tests.md` that still mention `tests/api/*` are superseded by this map.

## Module 0 (new golden list — see module-0-identity/golden-tests.md)
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

## Module 1 (golden-tests.md G1–G10)
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

## Module 2 (golden-tests.md G1–G10)
| ID | Test |
|---|---|
| G1 isolation | `TestTenantIsolation` |
| G2 atomic onboarding | `TestGoldenM2_AtomicOnboarding` |
| G3 duplicate code | `TestEmployeeLifecycle` |
| G4 foreign-tenant user | `TestGoldenM2_EmployeeForForeignTenantUser` |
| G5 self-service guard | `TestEmployeeSelfService` |
| G6 privilege escalation block | `TestEmployeeRBACTiers`, `TestEmployeeSelfService` |
| G7 PII masked | `TestEmployeeStatutoryAndPIIMasking` |
| G8 sensitive access | `TestEmployeeStatutoryAndPIIMasking` |
| G9 status transition | `TestEmployeeStatusStateMachine` |
| G10 deactivation sync | `TestUserDeactivationConvergesEmployee` |
