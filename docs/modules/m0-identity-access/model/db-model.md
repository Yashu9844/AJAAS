# M0 — Identity & Access — Database Model & Dependencies

**Domain scope:** Platform foundation

**Purpose:** Tenants, identities, credentials, sessions, roles, permissions, invitations, security audit.

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
| `Tenant` | tenant_id, slug, status, plan, created_at |
| `UserIdentity` | user_id, tenant_id, email_norm, status, display_name |
| `Credential` | credential_id, user_id, password_hash, changed_at |
| `Role` | role_id, tenant_id, code, name |
| `Permission` | permission_id, code, resource, action |
| `RolePermission` | tenant_id, role_id, permission_id |
| `UserRole` | tenant_id, user_id, role_id |
| `Session` | session_id, user_id, expires_at, revoked_at |
| `RefreshToken` | token_id, session_id, token_hash, rotated_from_id |
| `Invitation` | invitation_id, tenant_id, email_norm, role_id, expires_at, accepted_at |
| `SecurityAuditEvent` | event_id, tenant_id, actor_id, action, resource, outcome, occurred_at |
| `IdempotencyRecord` | tenant_id, actor_id, operation, key, request_hash, status, response_ref |
| `OutboxEvent` | event_id, event_type, version, payload, published_at |

## Relationships and invariants

Tenant 1:N UserIdentity, Role, Invitation and audit events. UserIdentity 1:N Credential, Session and UserRole. Role M:N Permission through RolePermission. Session 1:N RefreshToken. Service-local IdempotencyRecord and OutboxEvent support durable retries and events.

## Service dependencies

Foundation for identity claims and authorization across all services. Must not depend on HR or business services for login. Publish identity lifecycle events; never publish passwords, tokens or credential hashes.

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
