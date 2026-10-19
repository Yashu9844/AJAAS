# M10 — Notifications — Database Model & Dependencies

**Domain scope:** Templates, preferences, inbox items, dispatch jobs, attempts, provider receipts and dead letters

**Purpose:** Reliable multi-channel dispatch and delivery tracking.

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
| `NotificationTemplate` | template_id, tenant_id, channel, key, active_version |
| `TemplateVersion` | template_id, version, body_ref, locale, created_at |
| `UserPreference` | tenant_id, user_id, channel, category, enabled |
| `NotificationInboxItem` | notification_id, tenant_id, user_id, event_key, read_at, created_at |
| `DispatchJob` | job_id, tenant_id, event_id, recipient_ref, channel, status, idempotency_key |
| `DispatchAttempt` | attempt_id, job_id, attempt_no, provider_request_ref, outcome, attempted_at |
| `ProviderReceipt` | receipt_id, job_id, provider_message_id, state, received_at |
| `SuppressionEntry` | tenant_id, destination_hash, reason, created_at |
| `DeadLetterRecord` | dead_letter_id, job_id, failure_code, failed_at |

## Relationships and invariants

NotificationTemplate 1:N TemplateVersion. DispatchJob 1:N DispatchAttempt and 0:N ProviderReceipt. Inbox item unique by tenant/user/event key to deduplicate. Delivery outcome is not equivalent to business success.

## Service dependencies

Consumes domain events from RabbitMQ and calls email/SMS/push providers. Provider credentials in secret manager. Delivery events go to Reports and source modules where useful; dispatch failure never rolls back a committed source transaction.

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
