# M5 — Finance — Database Model & Dependencies

**Domain scope:** Vendors, budgets, cost centres, expenses, reimbursements, transactions and reconciliation

**Purpose:** Financial operations excluding payroll calculation, which remains HR-owned.

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
| `Vendor` | vendor_id, tenant_id, name, status, tax_ref_encrypted |
| `CostCentre` | cost_centre_id, tenant_id, code, name |
| `Budget` | budget_id, tenant_id, cost_centre_id, currency, limit_minor, period_id, version |
| `BudgetPeriod` | period_id, starts_on, ends_on, close_state |
| `ExpenseClaim` | claim_id, tenant_id, submitter_user_id, currency, total_minor, status, version |
| `ExpenseLine` | line_id, claim_id, category, amount_minor, tax_minor, occurred_on |
| `ReceiptReference` | line_id, document_id, scan_state |
| `Reimbursement` | reimbursement_id, claim_id, amount_minor, status, paid_at, provider_ref |
| `FinancialTransaction` | transaction_id, tenant_id, type, amount_minor, currency, posted_at, reversal_of_id |
| `ReconciliationBatch` | batch_id, tenant_id, period_id, status, checksum |
| `ApprovalReference` | claim_id, approval_instance_id, decision_ref |
| `ExportJob` | job_id, tenant_id, type, status, artifact_ref |

## Relationships and invariants

ExpenseClaim 1:N ExpenseLine and 0:N Reimbursement. ExpenseLine references Receipt document. Budget belongs to CostCentre and BudgetPeriod. Posted FinancialTransaction is immutable; corrections use reversal entries. Store currency and integer minor units or fixed-precision decimal.

## Service dependencies

Approvals owns workflow instance/decision history. Documents owns receipt binaries. HR can supply payroll export/reference, but payroll calculations remain HR-owned. Notifications sends status changes.

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
