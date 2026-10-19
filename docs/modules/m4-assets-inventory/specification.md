# M4 Assets & Inventory — Specification (PROPOSED)

> Not frozen. This is the target-state contract from `docs/ARCHITECTURE.md` section 5, combined with the logical data
> model from `model/db-model.md`. API paths are proposed shapes. Universal rules (tenancy, idempotency, outbox/inbox,
> envelopes, security, latency) in `docs/ARCHITECTURE.md` section 3 apply in full.

**Purpose:** track equipment, inventory, assignment, return, and
maintenance.

**Data model:** AssetType, Asset, StockItem, Location, StockMovement,
AssetAssignment, ReturnInspection, MaintenanceTicket,
PurchaseRequestReference, AuditHistory.

**Representative APIs:**
- `GET/POST /api/v1/assets`
- `GET/PATCH /api/v1/assets/{id}`
- `POST /api/v1/assets/{id}/assign`
- `POST /api/v1/assets/{id}/return`
- `POST /api/v1/assets/{id}/maintenance`
- `GET /api/v1/inventory/stock`
- `POST /api/v1/inventory/movements`

**Rules:** serialized assets have one active assignment unless
explicitly modeled otherwise; stock cannot go negative without an
approved exception; every stock change is a ledger/movement;
assignment/return commands are idempotent; employee references resolve
through HR IDs without direct HR table access.

**Events:** asset registered, assigned, returned, maintenance status
changed, stock threshold breached. Notifications can notify assignees
and custodians.

**Acceptance tests:** two concurrent assignments cannot claim one asset;
duplicate movement key is replayed; stock invariants; tenant/location
scope; lost/stolen state workflow; audit trail.

## Proposed entities

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
