# M2 — Projects & Tasks — Database Model & Dependencies

**Domain scope:** Projects, milestones, boards, tasks, sprints, dependencies, comments and activity

**Purpose:** Project delivery and work tracking; employee identity is referenced by IDs.

**Status:** Proposed target model. Validate against current migrations and OpenAPI before implementation.
## Common persistence conventions
- PostgreSQL is authoritative. Use UUID primary keys; tenant-owned rows include `tenant_id`.
- Include `created_at`, `updated_at`, and where applicable `created_by`, `updated_by`, `version`, `deleted_at`.
- Enforce tenant-scoped uniqueness with composite constraints such as `(tenant_id, normalized_name)`.
- Every query, cache key, export, worker and event consumer must be tenant-scoped.
- Use database constraints for invariants; use transactions for multi-row state changes.
- Durable idempotency uses a unique DB constraint; Redis is not the correctness store.
- Outbox records are committed atomically with domain changes; consumers maintain a durable inbox keyed by `(consumer_name, event_id)`.
- Do not write to another service's tables. A shared PostgreSQL cluster may be used initially, but schema/migration ownership and credentials are isolated per service.

## Proposed tables / entities

| Entity/table | Key fields (illustrative) |
|---|---|
| `Project` | project_id, tenant_id, name, status, owner_user_id, version |
| `ProjectMember` | tenant_id, project_id, user_id, role |
| `Milestone` | milestone_id, project_id, due_at, status |
| `Board` | board_id, project_id, name |
| `Task` | task_id, tenant_id, project_id, parent_task_id, status, priority, due_at, version |
| `TaskAssignee` | tenant_id, task_id, user_id |
| `TaskDependency` | tenant_id, predecessor_task_id, successor_task_id, dependency_type |
| `TaskComment` | comment_id, task_id, author_user_id, body, created_at |
| `TaskActivity` | activity_id, task_id, actor_id, action, occurred_at |
| `Sprint` | sprint_id, project_id, start_at, end_at, status |
| `TaskLabel` | tenant_id, task_id, label_id |
| `AttachmentReference` | task_id, document_id, version_ref |

## Relationships and invariants

Project 1:N Milestone, Board, Task and Sprint. Project M:N User via ProjectMember. Task can have parent/child hierarchy and M:N assignees; TaskDependency has unique ordered pair and cycle detection. TaskComment and TaskActivity are append-oriented.

## Service dependencies

References users/employees by stable IDs only; never reads HR tables. Documents owns attachments and ACLs. Publishes task/project events consumed by Notifications and Reports.

## Local reliability tables

- `idempotency_records`: unique `(tenant_id, actor_id, operation, idempotency_key)`; persist request hash, processing/completed state, stable result reference, timestamps and expiry.
- `outbox_events`: durable event ID, type/version, aggregate ID/version, tenant, correlation/causation IDs, payload, publish attempts and published timestamp.
- `inbox_events`: unique `(consumer_name, event_id)` and processing result; commit with local state changes before acknowledging broker delivery.
- `audit_events`: append-oriented business audit with actor, action, target, outcome and correlation ID.

## Suggested indexes and constraints

- Tenant-scoped unique keys for business identifiers.
- Index every hot query by `tenant_id` plus the actual filter/order fields; validate using query plans and load tests.
- Use unique constraints for idempotency, event deduplication, active assignment/decision invariants and version numbers.
- Add FK constraints within the service's owned schema; use external IDs without cross-service foreign keys.
- Use optimistic `version` checks or row locks for contested state transitions.

## Mermaid ER diagram

See matching `.mmd` file. This is a logical model, not a final migration script.
