# HR / Employee — Design

> Part of **M1 HR Management** (bounded context `employee`). Formerly the stand-alone "module-2-employee" bundle; module numbers in older text were renamed to the canonical M0–M12 / HR-context names.

## Architecture

Status: FROZEN 2026-10-05 (plan.md P1).

### 1. Layers & Organization

HR/Employee follows Clean Architecture and Domain-Driven Design:

```
[ HTTP Layer ]       Routes → Middleware (TenantResolver, Auth, RBAC, Audit) → Controllers
       ↓
[ Application ]      Services (EmployeeProfile, EmploymentDetail, Statutory, Document, Timeline, EventConsumer)
       ↓
[ Domain Layer ]     GORM Models (Profile, Employment, Contact, Statutory, Document, Timeline, Outbox) + Events
       ↓
[ Infrastructure ]   Repositories (Tenant-scoped DB queries) ↔ PostgreSQL / Redis / RabbitMQ
```

### 2. Component Responsibilities

1. **Controllers (`internal/employee/controllers`):**
   * Thin HTTP adapters handling request parsing, DTO binding, custom validation, and response envelope formatting (`{data, meta}` or `{error}`).
2. **Services (`internal/employee/services`):**
   * Domain rules, employee code unicity, status transition validation, automatic milestone timeline generation, sensitive field masking, and transactional event outbox emission.
3. **Repositories (`internal/employee/repositories`):**
   * Tenant-scoped persistence logic with GORM preloads, pagination filters, and explicit transaction support (`*gorm.DB`).
4. **Events & Outbox (`internal/employee/events`):**
   * Produces `employee.*` events to RabbitMQ with transactional fallback to `employee_events_outbox`.

### 3. Data Flow & Concurrency

* **Atomic Onboarding:** Creating an employee atomically inserts `EmployeeProfile`, `EmploymentDetail`, and `EmployeeContact` inside a single database transaction, appends the `hired` timeline event, and writes the `employee.created` event to the outbox.
* **Sensitive Data Isolation:** Statutory details (tax ID, bank accounts) reside in a separate 1:1 table `employee_statutory` to enable strict row-level and column-level access control.
* **Zero Trust Tenant Boundaries:** Every query includes `WHERE tenant_id = ?`. Cross-tenant lookups fail with `404 Not Found`.

## Connections

Depends On: M0 Identity (Identity), 1 (Organization).
Consumed By: HR/Attendance & Leave (Attendance/Leave), 4 (Projects), 5 (Meetings), 6 (Approvals), 9 (Payroll).

### Edge Table

| Edge ID | Source | Destination | Protocol | Contract | Auth | Format | Failure Mode | Owner |
|---|---|---|---|---|---|---|---|---|
| C1 | HR/Employee | M0 Identity UserService | Go In-Process | `GetByID(tenantID, userID)` | Context | struct | 404 User not found / not in tenant | M0 Identity |
| C2 | HR/Employee | M0 Identity AuditService | Go In-Process | `LogAction(tenantID, action, ...)` | Context | struct | Non-blocking log warning | M0 Identity |
| C3 | HR/Employee | HR/Organization MappingService | Go In-Process | `GetPrimaryMapping(tenantID, userID)` | Context | struct | 404 No mapping found | HR/Organization |
| C4 | M0 Identity | HR/Employee EventConsumer | RabbitMQ / In-Process | `identity.user.deactivated` | Queue | JSON Envelope | Dead-letter queue + retry | HR/Employee |
| C5 | HR/Employee | Downstream Modules | RabbitMQ / Outbox | `employee.created / status.changed` | Queue | JSON Envelope | Outbox fallback + retry worker | HR/Employee |

## Security

### 1. Multi-Tenant Data Isolation
- Every table (`employee_profiles`, `employment_details`, `employee_contacts`, `employee_statutory`, `employee_documents`, `employee_timelines`, `employee_events_outbox`) includes `tenant_id` UUID FK.
- Repositories inject `tenant_id` into all queries. Cross-tenant access returns `404 Not Found`.

### 2. RBAC Permissions Matrix
- `employee:read`: View basic directory and profile info (masked statutory).
- `employee:create`: Onboard new employees.
- `employee:update`: Modify employee records, employment terms, and status.
- `employee:read_sensitive`: View unmasked bank account and tax IDs.
- `employee:update_sensitive`: Modify bank details and statutory identifiers.
- `employee:admin`: Verify documents and force status modifications.

### 3. PII & Data Privacy
- Tax IDs and bank account numbers are stored in `employee_statutory` and sanitized on general responses.
- Self-service endpoints (`/api/v1/employees/me`) validate `userID` directly from verified JWT claims.
