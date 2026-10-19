# M12 — Settings — Database Model & Dependencies

**Domain scope:** Tenant settings, feature flags, integration config, webhook subscriptions, config versions and secret references

**Purpose:** Tenant configuration; secret values remain in a secrets manager.

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
| `TenantSetting` | tenant_id, setting_key, typed_value, version, updated_by |
| `FeatureFlag` | flag_id, tenant_id, key, enabled, targeting_rules_ref, version |
| `IntegrationConfig` | integration_id, tenant_id, provider, status, config_ref, secret_ref |
| `WebhookSubscription` | webhook_id, tenant_id, endpoint_ref, secret_ref, event_filters, status |
| `ConfigVersion` | config_version_id, tenant_id, scope, version, checksum, created_at |
| `ChangeApproval` | change_id, config_version_id, approval_instance_id, status |
| `SecretReference` | secret_ref_id, provider_key_ref, rotated_at, status |
| `WebhookDelivery` | delivery_id, webhook_id, event_id, attempt_no, status, next_attempt_at |

## Relationships and invariants

Tenant has many TenantSetting, FeatureFlag, IntegrationConfig and WebhookSubscription. ConfigVersion records immutable revisions. WebhookDelivery is retryable/idempotent and uses signed payloads and bounded retry policies.

## Service dependencies

M0 controls access. Secret values are external in a secrets manager; DB stores references only. Domain services consume relevant versioned configuration events and validate values locally.

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
