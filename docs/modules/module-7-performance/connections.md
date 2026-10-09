# Module 7 — Connections (FROZEN at P1 2026-10-09)

| # | Source → Destination | Contract | Failure |
|---|---|---|---|
| C1 | Client → Module 7 (22 ops) | spec §5; JWT + tenant; performance:* RBAC + PermissionFlag | 400–409 |
| C2 | → Module 0 Audit / middleware | Log after commit; RequirePermission + PermissionFlag(db, "performance", action, key, …) | best-effort / 401/403 |
| C3 | → Module 1 MappingService | `ListUserMappings(ctx, db, tenant, user, 1, 100)` → primary active mapping's manager_user_id (D7-03) | error propagates (500) |
| C4 | → Module 2 EmployeeService | `GetByID`, `GetByUserID` | 404 EMPLOYEE_NOT_FOUND |
| C5 | → PostgreSQL | migrations 000051–000056 | rollback |
| C6 | → RabbitMQ `jaas.performance.events` | ids, statuses, progress only | outbox + relay |
| C7 | boot → Module 0 permissions | performance:read, performance:manage | boot fails |
