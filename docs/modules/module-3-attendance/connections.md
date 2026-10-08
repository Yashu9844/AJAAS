# Module 3 — Connections (FROZEN at P1 2026-10-08)

Depends On: Module 0 (Tenant, Auth, RBAC, Audit), Module 2 (Employee profiles). Module 1 not required in v1 (manager-scoped approvals are v1.1).
Consumed By: Module 4 Leave (writes `on_leave` via contract C10), Module 5 Payroll (reads records), Module 10 Analytics, Module 11 AI, Module 12 IoT (future punch source).

| # | Source → Destination | Protocol | Contract | Auth | Format | Failure | Owner | Tests |
|---|---|---|---|---|---|---|---|---|
| C1 | Client → Module 3 API (19 endpoints) | HTTPS (Kong) → HTTP | specification.md §5 + swagger | JWT (Module 0 Authenticate) + tenant subdomain; `attendance:read/manage/approve` via Module 0 RequirePermission | JSON envelope | 400/401/403/404/409 per spec §6 | Module 3 | controller units + tests/api/attendance_*_test.go |
| C2 | Module 3 → Module 2 EmployeeService | In-process Go | `GetByUserID`, `GetByID`, `List(filter{Status})` | service-level, tenant passed explicitly | Go DTOs | not found → 404; error → fail closed | Module 2 provider | service units (fake directory) + live goldens |
| C3 | Module 3 → Module 0 AuditService | In-process Go | `Log(ctx, db, tenant, user, action, resource, id, meta, ip, ua)` | service-level | Go | best-effort after commit | Module 0 | service units (spy) + G11 live |
| C4 | Module 3 → Module 0 middleware | In-process Gin | `TenantResolver`, `Authenticate`, `RequirePermission(db,"attendance",action,…)` | JWT | Gin context `tenant_id`, `user_id` | 401/403 | Module 0 | G9 live |
| C5 | Module 3 → PostgreSQL | TCP/5432 (shared pool) | migrations 000025–000030 | env creds | SQL | rollback, 500 | Module 3 | repo via live goldens; migration up/down/up |
| C6 | Module 3 → RabbitMQ `jaas.attendance.events` | AMQP topic | spec FR-EV001 envelope v1.0 | env creds | JSON | outbox + relay; never blocks API | Module 3 | outbox unit + G12 live |
| C7 | Module 3 boot → Module 0 `permissions` table | SQL (idempotent insert) | rows `attendance:read`, `attendance:manage`, `attendance:approve` | DB | — | boot fails loudly | Module 3 (seed) / Module 0 (table) | boot + G9 |
| C8 | Module 3 → shared queue/logger/errors | In-process | `queue.EventPublisher`, `errors.AppError` | — | — | — | shared | — |
| C9 | Modules 5/10/11 → Module 3 reads | In-process Go (v1) | `AttendanceQueryService` list methods + records table schema (read-only) | service-level | DTOs | — | Module 3 provider | consumer contract tests (their P1) |
| C10 | Module 4 → Module 3 | In-process Go (v1, Module 4 P2) | `MarkLeave(ctx, tenant, employee, date, leaveRequestID)` → sets status `on_leave` (reserved, not built until Module 4) | service-level | — | — | Module 3 provider | Module 4 P2 |

Trust boundary: everything outside the handler is untrusted. Employee for self endpoints is derived from JWT `user_id` → Module 2 profile; never from request input. Every id parameter is resolved inside the caller's tenant; foreign ids return 404.
