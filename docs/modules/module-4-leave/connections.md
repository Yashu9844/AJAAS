# Module 4 — Connections (FROZEN at P1 2026-10-09)

| # | Source → Destination | Protocol | Contract | Failure | Tests |
|---|---|---|---|---|---|
| C1 | Client → Module 4 API (20 ops) | HTTP | spec §5 + swagger; JWT + tenant; `leave:*` RBAC | 400/401/403/404/409 | controllers + routes + live |
| C2 | Module 4 → Module 2 EmployeeService | in-process | `GetByUserID`, `GetByID` (status, gender, joining date) | 404 / fail closed | fakes + live |
| C3 | Module 4 → Module 0 AuditService | in-process | `Log(...)` after commit | best-effort | spy + live G11 |
| C4 | Module 4 → Module 0 middleware | Gin | TenantResolver, Authenticate, RequirePermission(db, "leave", action, …) | 401/403 | live G9 |
| C5 | Module 4 → PostgreSQL | SQL | migrations 000032–000037 | rollback | live + migration cycle |
| C6 | Module 4 → RabbitMQ `jaas.leave.events` | AMQP | FR-EV001 envelope | outbox + relay | unit + live outbox rows |
| C7 | Module 4 boot → Module 0 `permissions` | SQL idempotent insert | `leave:read`, `leave:manage`, `leave:approve` | boot fails loudly | boot + G9 |
| C8 | Module 4 → Module 3 `services.LeaveSync` | in-process, caller's tx | `MarkLeave(ctx, tx, tenant, employee, dates)`, `ClearLeave(ctx, tx, tenant, employee, dates)` | rollback whole approve/cancel | Module 3 unit + live G8 |
| C9 | Modules 5/10/11 → Module 4 | events (+ ledger reads later) | FR-EV001 payloads | — | consumers' P1 |

Trust boundary as Module 3: employee for self endpoints from JWT user only; every id resolved in the caller's tenant.
