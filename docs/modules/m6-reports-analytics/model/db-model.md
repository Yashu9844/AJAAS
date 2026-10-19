# M6 — Reports & Analytics — Database Model & Dependencies

**Domain scope:** Report definitions, schedules, executions, artifacts, projections and refresh checkpoints

**Purpose:** Read models and analytics; never the transactional source of truth.

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
| `ReportDefinition` | report_id, tenant_id, owner_id, name, definition_version, status |
| `ReportVersion` | report_id, version, query_spec_ref, created_at |
| `ReportSchedule` | schedule_id, report_id, cron, timezone, enabled |
| `ReportExecution` | execution_id, report_id, idempotency_key, status, started_at, finished_at |
| `ResultArtifactReference` | execution_id, object_key_ref, expires_at, checksum |
| `ProjectionCheckpoint` | projection_name, partition_key, last_event_id, updated_at |
| `ReadModelVersion` | model_name, tenant_id, source_watermark, refreshed_at |
| `DataSourceGrant` | tenant_id, role_id, dataset, allowed_fields |
| `ProjectionRecord` | tenant_id, projection_key, payload, source_version, refreshed_at |

## Relationships and invariants

ReportDefinition 1:N ReportVersion, ReportSchedule and ReportExecution. ReportExecution 0:1 ResultArtifactReference. ProjectionCheckpoint tracks consumer progress. ProjectionRecord is rebuildable and tenant-scoped; access control is rechecked at query/export time.

## Service dependencies

Consumes versioned events or bounded APIs; uses M0 authorization context; object storage holds exports. Must not write into source-domain databases or expose stale projections as live truth.

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
