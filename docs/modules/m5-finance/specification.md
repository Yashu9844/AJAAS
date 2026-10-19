# M5 Finance — Specification (PROPOSED)

> Not frozen. This is the target-state contract from `docs/ARCHITECTURE.md` section 5, combined with the logical data
> model from `model/db-model.md`. API paths are proposed shapes. Universal rules (tenancy, idempotency, outbox/inbox,
> envelopes, security, latency) in `docs/ARCHITECTURE.md` section 3 apply in full.

**Purpose:** expense claims, reimbursements, budgets, vendors, cost
centres, and financial records. Decide and document whether statutory
payroll accounting remains in HR Payroll or exports into Finance;
Finance must not recalculate payroll.

**Data model:** Vendor, CostCentre, Budget, BudgetPeriod, ExpenseClaim,
ExpenseLine, ReceiptReference, Reimbursement, FinancialTransaction,
ReconciliationBatch, ApprovalReference, ExportJob.

**Representative APIs:**
- `GET/POST /api/v1/finance/vendors`
- `GET/POST /api/v1/finance/budgets`
- `POST /api/v1/finance/expenses`
- `POST /api/v1/finance/expenses/{id}/submit`
- `POST /api/v1/finance/expenses/{id}/decision`
- `POST /api/v1/finance/reimbursements/{id}/mark-paid`
- `POST /api/v1/finance/exports`

**Rules:** exact decimal currency; explicit currency and rounding;
separation of submitter/approver/payer; duplicate receipt/claim
detection as a risk control; immutable posted transactions with reversal
entries; period close prevents silent backdating; approval service owns
workflow decisions while Finance owns the expense/reimbursement state.

**Events:** expense submitted/approved/rejected, reimbursement
initiated/paid, budget threshold exceeded, reconciliation completed.

**Acceptance tests:** duplicate payment command; same idempotency key
with changed amount rejected; maker-checker; ledger balancing where
double-entry is in scope; export reproducibility; tenant and cost-centre
permissions.

## Proposed entities

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
