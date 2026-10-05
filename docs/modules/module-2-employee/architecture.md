# Module 2 — Architecture

Status: FROZEN 2026-10-05 (plan.md P1).

## 1. Layers & Organization

Module 2 follows Clean Architecture and Domain-Driven Design:

```
[ HTTP Layer ]       Routes → Middleware (TenantResolver, Auth, RBAC, Audit) → Controllers
       ↓
[ Application ]      Services (EmployeeProfile, EmploymentDetail, Statutory, Document, Timeline, EventConsumer)
       ↓
[ Domain Layer ]     GORM Models (Profile, Employment, Contact, Statutory, Document, Timeline, Outbox) + Events
       ↓
[ Infrastructure ]   Repositories (Tenant-scoped DB queries) ↔ PostgreSQL / Redis / RabbitMQ
```

## 2. Component Responsibilities

1. **Controllers (`internal/employee/controllers`):**
   * Thin HTTP adapters handling request parsing, DTO binding, custom validation, and response envelope formatting (`{data, meta}` or `{error}`).
2. **Services (`internal/employee/services`):**
   * Domain rules, employee code unicity, status transition validation, automatic milestone timeline generation, sensitive field masking, and transactional event outbox emission.
3. **Repositories (`internal/employee/repositories`):**
   * Tenant-scoped persistence logic with GORM preloads, pagination filters, and explicit transaction support (`*gorm.DB`).
4. **Events & Outbox (`internal/employee/events`):**
   * Produces `employee.*` events to RabbitMQ with transactional fallback to `employee_events_outbox`.

## 3. Data Flow & Concurrency

* **Atomic Onboarding:** Creating an employee atomically inserts `EmployeeProfile`, `EmploymentDetail`, and `EmployeeContact` inside a single database transaction, appends the `hired` timeline event, and writes the `employee.created` event to the outbox.
* **Sensitive Data Isolation:** Statutory details (tax ID, bank accounts) reside in a separate 1:1 table `employee_statutory` to enable strict row-level and column-level access control.
* **Zero Trust Tenant Boundaries:** Every query includes `WHERE tenant_id = ?`. Cross-tenant lookups fail with `404 Not Found`.
