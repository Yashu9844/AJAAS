# HR / Performance & OKRs — Design

> Part of **M1 HR Management** (bounded context `performance`). Formerly the stand-alone "module-7-performance" bundle; module numbers in older text were renamed to the canonical M0–M12 / HR-context names.

## Architecture

```text
Gin → TenantResolver → Authenticate → [RequirePermission(performance:read|manage)] + PermissionFlag(read, manage)
→ Controllers (cycle, goal, review)
→ Services (CycleService, GoalService, ReviewService, OutboxRelay) + access policy (owner / manager / permission)
→ score (pure: hundredths fixed point, KR progress, goal progress, goal score, overall score, 9-box bands)
→ lifecycle (pure: cycle graph PM-002, goal status graph FR-GL004)
→ Repositories (cycle, goal + key results + check-ins, review, outbox)
→ PostgreSQL · RabbitMQ (outbox) · M0 Identity Audit · HR/Organization MappingService (manager) · HR/Employee EmployeeService
```

### Access policy (PM-008, D7-05)
canRead(goal) = flag read OR owner OR manager(owner); canWrite(goal) = flag manage OR owner OR manager(owner).
owner compares the caller's employee profile (from the JWT user id) with goal.employee_id.
manager(owner) = the HR/Organization primary active mapping of owner.user_id has manager_user_id = caller user id. Denials are 404.

### Weight cap (PM-004)
Create/update/reactivate takes a transaction-scoped advisory lock keyed on (cycle_id, employee_id), sums non-cancelled weights excluding the goal itself, then writes.

### Launch reviews (PM-010)
One transaction: lock the cycle row, ensure `active`, iterate distinct owners of non-cancelled goals, resolve managers (HR/Organization → HR/Employee GetByUserID), insert missing reviews (pre-checked under the cycle lock; unique index as backstop), set `review`.

## Connections

| # | Source → Destination | Contract | Failure |
|---|---|---|---|
| C1 | Client → HR/Performance (22 ops) | spec §5; JWT + tenant; performance:* RBAC + PermissionFlag | 400–409 |
| C2 | → M0 Identity Audit / middleware | Log after commit; RequirePermission + PermissionFlag(db, "performance", action, key, …) | best-effort / 401/403 |
| C3 | → HR/Organization MappingService | `ListUserMappings(ctx, db, tenant, user, 1, 100)` → primary active mapping's manager_user_id (D7-03) | error propagates (500) |
| C4 | → HR/Employee EmployeeService | `GetByID`, `GetByUserID` | 404 EMPLOYEE_NOT_FOUND |
| C5 | → PostgreSQL | migrations 000051–000056 | rollback |
| C6 | → RabbitMQ `jaas.performance.events` | ids, statuses, progress only | outbox + relay |
| C7 | boot → M0 Identity permissions | performance:read, performance:manage | boot fails |

## Security

Permissions: `performance:read`, `performance:manage`. Self/manager access is decided per request (PM-008) and never trusts ids from the body for identity.

| ID | Threat | Control |
|---|---|---|
| PS-T1 | Review text/ratings leaking | PM-017: no ratings/scores/potential/text in events or audit |
| PS-T2 | Reading colleagues' goals | PM-008 owner/manager/permission; others 404 |
| PS-T3 | Review forgery | only the assigned reviewer (JWT → employee) submits; others 404 (PM-012) |
| PS-T4 | Peer de-anonymisation | reviewer id omitted for peer reviews shown to the subject (PM-014) |
| PS-T5 | Retro-editing after review | cycle state locks goals (PM-003); reviews immutable once submitted |
| PS-T6 | Weight inflation race | advisory lock per (cycle, owner) (PM-004) |
| PS-T7 | Cross-tenant IDOR | tenant-scoped repositories; 404 |
