# M4 — Assets & Inventory — Database Model & Dependencies

**Domain scope:** Assets, stock, locations, movements, assignments, returns and maintenance

**Purpose:** Physical and digital asset lifecycle and inventory accounting.

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
| `AssetType` | asset_type_id, tenant_id, name, tracking_mode |
| `Asset` | asset_id, tenant_id, asset_tag, serial_no, type_id, status, version |
| `StockItem` | stock_item_id, tenant_id, sku, location_id, quantity_on_hand, version |
| `Location` | location_id, tenant_id, parent_location_id, name |
| `StockMovement` | movement_id, stock_item_id, quantity_delta, reason, idempotency_key, occurred_at |
| `AssetAssignment` | assignment_id, asset_id, employee_id, assigned_at, returned_at, status |
| `ReturnInspection` | inspection_id, assignment_id, condition, notes_ref |
| `MaintenanceTicket` | ticket_id, asset_id, status, opened_at, closed_at |
| `PurchaseRequestReference` | request_ref, approval_instance_id, status_ref |
| `AssetAudit` | audit_id, asset_id, actor_id, action, occurred_at |

## Relationships and invariants

AssetType 1:N Asset. Location is hierarchical. StockItem 1:N StockMovement; quantity is reconciled from movements. Asset 1:N historical AssetAssignment but only one active assignment unless product rules say otherwise. Use row locking/optimistic versioning for stock and assignment.

## Service dependencies

References employee IDs from HR. Approvals owns workflow decisions; Documents owns invoices/receipts; Notifications handles assignment/maintenance alerts.

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
