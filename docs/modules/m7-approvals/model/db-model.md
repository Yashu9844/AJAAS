# M7 — Approvals — Database Model & Dependencies

**Domain scope:** Workflow definitions/versions, instances, steps, assignments, decisions, delegation and escalation

**Purpose:** Reusable workflow state; originating service owns final business record.

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
| `WorkflowDefinition` | workflow_id, tenant_id, name, status |
| `WorkflowVersion` | workflow_id, version, schema, published_at |
| `WorkflowInstance` | instance_id, tenant_id, workflow_id, version, source_type, source_id, status, correlation_id |
| `StepInstance` | step_instance_id, instance_id, step_key, status, sequence |
| `ApproverAssignment` | assignment_id, step_instance_id, approver_user_id, assigned_at |
| `ApprovalDecision` | decision_id, step_instance_id, actor_id, decision, comment_ref, decided_at |
| `Delegation` | delegation_id, tenant_id, from_user_id, to_user_id, starts_at, ends_at |
| `EscalationTimer` | timer_id, step_instance_id, due_at, status |
| `WorkflowAudit` | audit_id, instance_id, actor_id, action, occurred_at |

## Relationships and invariants

WorkflowDefinition 1:N WorkflowVersion and WorkflowInstance. Instance pins a version and has 1:N StepInstance. StepInstance has assignments and immutable decisions. Unique decision/idempotency constraints prevent conflicting duplicate actions; timer workers are replay-safe.

## Service dependencies

M0 supplies actor/role context; Notifications receives assignment/outcome events. HR, Finance and Assets create workflow requests and consume decisions, then independently validate their own domain transitions.

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
