# Module 2 — Connections

Depends On: Modules 0 (Identity), 1 (Organization).
Consumed By: Modules 3 (Attendance/Leave), 4 (Projects), 5 (Meetings), 6 (Approvals), 9 (Payroll).

## Edge Table

| Edge ID | Source | Destination | Protocol | Contract | Auth | Format | Failure Mode | Owner |
|---|---|---|---|---|---|---|---|---|
| C1 | Module 2 | Module 0 UserService | Go In-Process | `GetByID(tenantID, userID)` | Context | struct | 404 User not found / not in tenant | Module 0 |
| C2 | Module 2 | Module 0 AuditService | Go In-Process | `LogAction(tenantID, action, ...)` | Context | struct | Non-blocking log warning | Module 0 |
| C3 | Module 2 | Module 1 MappingService | Go In-Process | `GetPrimaryMapping(tenantID, userID)` | Context | struct | 404 No mapping found | Module 1 |
| C4 | Module 0 | Module 2 EventConsumer | RabbitMQ / In-Process | `identity.user.deactivated` | Queue | JSON Envelope | Dead-letter queue + retry | Module 2 |
| C5 | Module 2 | Downstream Modules | RabbitMQ / Outbox | `employee.created / status.changed` | Queue | JSON Envelope | Outbox fallback + retry worker | Module 2 |
