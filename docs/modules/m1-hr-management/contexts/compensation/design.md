# HR / Compensation — Design

## Architecture

```text
Gin → TenantResolver → Authenticate → [Require(compensation:read|manage|approve)] → handlers
→ Service (ranges, changes, worker views, merit cycles, proposals, budget usage)
→ Store (gorm, schema hr, comp_* tables)
→ Ports: core/api (grade, department and manager on a date), performance/api (final rating band per worker for a
  cycle), payroll/api (SetCTC from a date), employee/api (directory), identityport (audit)
```

## Data (migrations hr 000058–000062)

| Table | Notes |
|---|---|
| `comp_pay_ranges` | per (tenant, core grade): min/mid/max paise, `[effective_from, effective_to)`; exclusion constraint against overlap |
| `comp_records` | per worker: annual base paise, `[effective_from, effective_to)`, reason, `change_id` or `merit_cycle_id`, `payroll_synced`; exclusion constraint against overlap |
| `comp_changes` | off-cycle change requests: pending, approved, rejected or cancelled; requester, decider |
| `comp_merit_cycles` | status draft → open → in_review → approved → applied; guidelines jsonb; `opened_by`, `approved_by` |
| `comp_merit_budgets`, `comp_merit_proposals` | budgets per department; one proposal per (cycle, worker) with snapshots |

## Key rules

- **Apply a record:** lock the worker (advisory lock). The new start must be after the open record's start
  (`BACKDATED`). Close the open record at the new start, insert the new record, then call `payroll.SetCTC` in the
  same transaction.
- **Budget:** usage for a department is Σ round(base × pct) over its proposals, checked under the cycle row lock
  (`FOR UPDATE`).
- **Compa-ratio:** base × 100 ÷ mid, rounded half away from zero (hundredths). Band: below < 0.90 ≤ within ≤ 1.10
  < above.

## Security

| ID | Threat | Control |
|---|---|---|
| CS-C1 | A manager reading peers' pay | proposals are filtered by the core manager; other compensation reads need `compensation:read` |
| CS-C2 | Self-raise | approvals need a different user from the requester or opener (`COMP_SELF_APPROVAL`) |
| CS-C3 | Amount leakage | events and audit carry ids, dates and reasons only |
| CS-C4 | Double application | apply is a state transition under a row lock; a second apply is 409 |
