# M3 — Meetings — Database Model & Dependencies

**Domain scope:** Meetings, participants, agendas, minutes, recurrence, action-item references

**Purpose:** Scheduling, meeting records and decisions; tasks remain owned by Projects & Tasks.

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
| `Meeting` | meeting_id, tenant_id, title, organizer_id, starts_at, ends_at, timezone, visibility, status, version |
| `MeetingParticipant` | meeting_id, user_id, response, attendance_state |
| `AgendaItem` | agenda_item_id, meeting_id, position, title, owner_id |
| `MinutesVersion` | minutes_version_id, meeting_id, version, body_ref, published_at |
| `RecurrenceRule` | meeting_id, rule, until_at, occurrence_limit |
| `MeetingActionReference` | meeting_id, task_id, title, owner_id, due_at, status_ref |
| `MeetingResource` | meeting_id, resource_ref, resource_type |
| `MeetingAudit` | audit_id, meeting_id, actor_id, action, occurred_at |

## Relationships and invariants

Meeting 1:N MeetingParticipant, AgendaItem and MinutesVersion. Meeting 0:1 RecurrenceRule. MeetingActionReference stores external task IDs, not duplicate authoritative task state. Enforce timezone-aware timestamps and visibility ACL.

## Service dependencies

M0 identity context; HR provides stable participant IDs; Projects & Tasks owns action-item tasks; Documents stores attached files/minutes artifacts; Notifications sends reminders.

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
