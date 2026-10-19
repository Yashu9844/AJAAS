# M11 — Company Knowledge Base — Database Model & Dependencies

**Domain scope:** Articles, versions, categories, tags, audience rules, publication state and search-index metadata

**Purpose:** Versioned company knowledge and ACL-aware search.

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
| `Article` | article_id, tenant_id, owner_user_id, status, current_version, classification |
| `ArticleVersion` | article_id, version, title, body_ref, created_by, created_at |
| `Category` | category_id, tenant_id, parent_id, name |
| `ArticleTag` | article_id, tag_id |
| `AudienceRule` | article_id, principal_type, principal_id, permission |
| `ReviewAssignment` | review_id, article_id, reviewer_id, status |
| `PublicationRecord` | publication_id, article_id, version, published_at, published_by |
| `SearchIndexRecord` | article_id, version, index_version, indexed_at, status |
| `ArticleFeedback` | feedback_id, article_id, user_id, score, comment_ref |

## Relationships and invariants

Article 1:N ArticleVersion, ReviewAssignment and PublicationRecord. Article M:N Tag through ArticleTag. Published versions are immutable; draft changes create a new version. Index records refer to exact article version and can be rebuilt.

## Service dependencies

M0 identity and ACL context; Documents owns linked files. Search index is a derived projection and must filter by tenant and audience at query time. AI may retrieve only through the same ACL-aware interface.

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
