# HR / Core — Design

## Architecture

```text
Gin → TenantResolver → Authenticate → [Require(core:read|manage|approve)] → Controllers
→ Services: Architecture (families, grades, profiles), Positions, Actions (lifecycle + apply), Workers (history,
  as-of views), Headcount, Effects (projections + sweep), Backfill, OutboxRelay
→ Repositories (one per aggregate) → PostgreSQL schema hr (core_* tables)
→ Ports: EmployeeDirectory (HR/Employee api), EmployeeLifecycle (HR/Employee api), OrgDirectory (departments,
  locations, primary-mapping projection — HR/Organization via the HR root), Logins (identityport.Users), Auditor
```

## Data (migrations hr 000048–000053)

| Table | Notes |
|---|---|
| `core_job_families`, `core_grades`, `core_job_profiles` | unique (tenant, code); grades unique (tenant, rank) |
| `core_positions` | status `active\|frozen\|closed`; `budgeted_fte numeric(5,2)` |
| `core_job_assignments` | `effective_from date`, `effective_to date null` (exclusive); `status active\|rescinded`; `EXCLUDE USING gist (tenant_id WITH =, employee_id WITH =, daterange(effective_from, effective_to) WITH &&) WHERE (status = 'active')` (btree_gist) |
| `core_hr_actions` | `changes jsonb`; `previous_assignment_id`, `previous_effective_to`, `new_assignment_id`, `previous_employee_status`, `effects_applied_at`, `effects_reversed_at` |
| `core_headcount_plans` | `EXCLUDE … daterange(period_start, period_end, '[]') WITH &&` per (tenant, department) |
| `core_events_outbox` | standard outbox |

## Key algorithms

- **Apply (approve):** lock the worker (`pg_advisory_xact_lock(hash(core:worker:<id>))`). Re-validate (CO-032..CO-038)
  as of the effective date. For a change, set the current assignment's `effective_to = effective_date` (remember the
  old value) and insert the merged assignment. Check position occupancy for every date of the new period (CO-014),
  using the position lock `pg_advisory_xact_lock(hash(core:position:<id>))`. Mark the action completed, write the
  outbox row, and apply the effects in the same transaction when the date is today or earlier.
- **Rescind:** lock the worker, require the latest completed action, rescind its new assignment, restore the previous
  `effective_to`, reverse the applied effects (re-project the previous assignment; termination: restore the employee
  status and reactivate the login).
- **Occupancy on date d:** Σ fte of active assignments with `position_id = p AND effective_from ≤ d AND
  (effective_to IS NULL OR effective_to > d)`. The future check takes the maximum over the change dates in the period.
- **Sweep:** completed, not rescinded actions with `effects_applied_at IS NULL AND effective_date ≤ today`, oldest
  first, each in its own transaction under the worker lock.

## Connections

| # | Source → destination | Contract | Failure |
|---|---|---|---|
| C1 | Client → core (32 ops) | spec §4 | 400–409 |
| C2 | core → HR/Employee | `api.Directory` (GetByID, List), `api.Lifecycle.ApplyCoreStatus` (status + dates + timeline, inside the core tx) | 404 / rollback |
| C3 | core → HR/Organization | `DepartmentExists`, `LocationActive`, `ProjectPrimaryMapping(tx, user, dept, location, managerUser)` | 404/409 / rollback |
| C4 | core → M0 | `identityport.Users.DeactivateUser / ActivateUser` (inside the tx) | rollback |
| C5 | core → RabbitMQ | `jaas.core.events` via outbox + relay | retried |

## Security

| ID | Threat | Control |
|---|---|---|
| CS-T1 | Self-promotion | `CORE_SELF_APPROVAL`: the initiator never approves; `core:approve` is separate from `core:manage` |
| CS-T2 | Retro-editing history | assignments are never updated except `effective_to` by the apply/rescind algorithm; nothing is deleted |
| CS-T3 | Overlapping or double-filled records under concurrency | worker + position advisory locks and the DB exclusion constraint |
| CS-T4 | Leaking reasons or comments | events and audit hold ids, type, status and dates only |
| CS-T5 | Cross-tenant ids | every query is tenant-scoped; foreign ids → 404 |
