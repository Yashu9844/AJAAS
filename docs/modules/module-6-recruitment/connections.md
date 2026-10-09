# Module 6 — Connections (FROZEN at P1 2026-10-09)

| # | Source → Destination | Contract | Failure |
|---|---|---|---|
| C1 | Client → Module 6 (20 ops) | spec §5; JWT + tenant; recruitment:* RBAC | 400–409 |
| C2 | → Module 0 UserService | `InviteUser(ctx, tx, tenant, {email, first_name, last_name}, corr)` | 409 CONFLICT if the email is taken |
| C3 | → Module 0 Audit / middleware | Log after commit; RequirePermission(db, "recruitment", action, …) | best-effort / 401/403 |
| C4 | → Module 1 Department/Designation services | `GetDepartment`, `GetDesignation` via new read accessors (D6-03) | 404 |
| C5 | → Module 2 EmployeeService | `GetByID`, `GetByUserID`, `CreateEmployee` | 404 / 409 |
| C6 | → PostgreSQL | migrations 000045–000050 | rollback |
| C7 | → RabbitMQ `jaas.recruitment.events` | ids and stages only | outbox + relay |
| C8 | boot → Module 0 permissions | recruitment:read, recruitment:manage, recruitment:hire | boot fails |
