# HR / Core — Decisions

- **DC-01** Core is the system of record for job data. Employee status, the organization primary mapping and the login
  are projections that core updates when an action takes effect (CO-040). Manual edits in those contexts stay
  possible for now and are overwritten by the next core action.
- **DC-02** Approval is a single step inside core until M7 Approvals exists (HR-013). Approving completes the action.
  The initiator can never approve (`CORE_SELF_APPROVAL`).
- **DC-03** Assignments never overlap. This is enforced twice: a per-worker advisory lock in the service and a
  `btree_gist` exclusion constraint in the database. `btree_gist` is a standard contrib extension, created by
  migration hr/000048.
- **DC-04** Changes per worker are sequential. A new action cannot take effect before an already completed later
  action (`LATER_CHANGE_EXISTS`), and only the latest completed action can be rescinded (`NOT_LATEST`). A correction
  is a rescind followed by a new action (HR-004).
- **DC-05** Position attributes are current-state in v1. Their history comes from `hr.core.position_changed` events
  and audit. Occupancy is effective-dated through assignments.
- **DC-06** Effects apply at completion when the effective date is today or earlier, otherwise by an hourly
  idempotent sweep (also callable on demand). A termination's effects apply after the last working day.
- **DC-07** The employee context gets a dedicated `Lifecycle` port for core (`ApplyCoreStatus`). The manual status
  matrix treats terminated as final, but core may rehire and rescind.
- **DC-08** Back-fill creates `hire` actions with `reason_code = backfill` so the history is uniform.
- **DC-09** The manager is a worker (`manager_employee_id`, HR-002). The organization projection converts it to the
  manager's user id.

## Changelog

2026-10-20 — Design: spec CO-001..CO-051, 32 operations, goldens CG1–CG12.
