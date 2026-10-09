# Module 7 — Architecture (FROZEN at P1 2026-10-09)

```text
Gin → TenantResolver → Authenticate → [RequirePermission(performance:read|manage)] + PermissionFlag(read, manage)
→ Controllers (cycle, goal, review)
→ Services (CycleService, GoalService, ReviewService, OutboxRelay) + access policy (owner / manager / permission)
→ score (pure: hundredths fixed point, KR progress, goal progress, goal score, overall score, 9-box bands)
→ lifecycle (pure: cycle graph PM-002, goal status graph FR-GL004)
→ Repositories (cycle, goal + key results + check-ins, review, outbox)
→ PostgreSQL · RabbitMQ (outbox) · Module 0 Audit · Module 1 MappingService (manager) · Module 2 EmployeeService
```

## Access policy (PM-008, D7-05)
canRead(goal) = flag read OR owner OR manager(owner); canWrite(goal) = flag manage OR owner OR manager(owner).
owner compares the caller's employee profile (from the JWT user id) with goal.employee_id.
manager(owner) = the Module 1 primary active mapping of owner.user_id has manager_user_id = caller user id. Denials are 404.

## Weight cap (PM-004)
Create/update/reactivate takes a transaction-scoped advisory lock keyed on (cycle_id, employee_id), sums non-cancelled weights excluding the goal itself, then writes.

## Launch reviews (PM-010)
One transaction: lock the cycle row, ensure `active`, iterate distinct owners of non-cancelled goals, resolve managers (Module 1 → Module 2 GetByUserID), insert missing reviews (pre-checked under the cycle lock; unique index as backstop), set `review`.
