# M8 — Documents — Database Model & Dependencies

**Domain scope:** Document metadata, versions, storage references, ACLs, share grants, scan results, retention and legal holds

**Purpose:** Secure file lifecycle; binaries stored in object storage.

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
| `Document` | document_id, tenant_id, owner_user_id, document_type, classification, status |
| `DocumentVersion` | version_id, document_id, version_no, object_key, checksum, size_bytes, scan_state, created_at |
| `DocumentACL` | document_id, principal_type, principal_id, permission |
| `ShareGrant` | share_id, document_id, token_hash, expires_at, revoked_at |
| `ScanResult` | scan_id, version_id, scanner, result, completed_at |
| `RetentionPolicy` | policy_id, tenant_id, classification, retention_days |
| `LegalHold` | hold_id, document_id, reason, placed_at, released_at |
| `AccessAudit` | audit_id, document_id, actor_id, action, occurred_at |

## Relationships and invariants

Document 1:N DocumentVersion, DocumentACL and AccessAudit. DocumentVersion references immutable object key/checksum and scan state. ShareGrant stores only token hash; legal hold overrides normal deletion. ACL must be checked before issuing short-lived signed URLs.

## Service dependencies

Object storage contains binaries; KMS/secrets manager protects encryption keys. Domain services own the business meaning of attachments. M0 supplies identity/authorization context; publish lifecycle events.

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
