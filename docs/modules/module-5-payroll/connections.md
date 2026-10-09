# Module 5 — Connections (FROZEN at P1 2026-10-09)

| # | Source → Destination | Contract | Failure | Tests |
|---|---|---|---|---|
| C1 | Client → Module 5 API (20 ops) | spec §5 + swagger; JWT + tenant; `payroll:*` RBAC | 400/401/403/404/409 | controllers, routes, integration |
| C2 | Module 5 → Module 2 EmployeeService | `GetByID`, `GetByUserID` (code, names, status, joining/exit dates) | 404 / skip with warning | fakes + integration |
| C3 | Module 5 → Module 4 `services.UnpaidLeave` | `UnpaidDays(ctx, tenant, employee, from, to) (calc.Days, error)` — approved requests of `is_paid=false` types, clipped to range | fail calculation | Module 4 unit + payroll integration |
| C4 | Module 5 → Module 0 Audit / middleware | `Log(...)` after commit; TenantResolver, Authenticate, RequirePermission(db, "payroll", action, …) | best-effort / 401/403 | spy + integration |
| C5 | Module 5 → PostgreSQL | migrations 000038–000044 | rollback | integration + cycle |
| C6 | Module 5 → RabbitMQ `jaas.payroll.events` | envelope as Modules 3/4; totals only | outbox + relay | unit |
| C7 | Module 5 boot → Module 0 `permissions` | `payroll:read`, `payroll:manage`, `payroll:approve` | boot fails | integration |
| C8 | Modules 7/10/11 → Module 5 | events | — | consumers |
