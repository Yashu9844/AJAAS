# HR / Core — Specification

> Part of **M1 HR Management** (bounded context `core`). Core HCM backbone: job architecture, positions, effective-dated
> job assignments and the HR actions (business processes) that change them. Requirements CH-001..CH-012 come from
> `../../specification.md` §4; this file refines them into the v1 contract (2026-10-20).

## 1. Glossary

- **Worker**: an employee profile (`employee_profile_id`, HR-002). A worker may have no login.
- **Job family / grade / job profile**: the job architecture. A profile has a family, an optional default grade, a
  category, and probation days.
- **Position**: a budgeted seat (job profile, department, optional location, budgeted FTE, `shared` flag).
  Its lifecycle status is `active | frozen | closed`. Occupancy (filled FTE) is derived from assignments as of a date.
- **Job assignment**: what a worker does over `[effective_from, effective_to)` (`effective_to` null = open). It holds
  the job profile, position, department, location, manager (a worker), grade, worker type, employment type, FTE and,
  from a hire, the probation end date.
- **HR action**: one business-process instance (`hire | promote | demote | transfer | change_job | change_manager |
  change_location | probation_confirm | terminate | rehire`) with an effective date, changes and a reason.

## 2. Actors and permissions

- `core:read`: job architecture, positions, assignments, actions, headcount and worker history.
- `core:manage`: job architecture, positions, headcount plans; creating, submitting and cancelling actions;
  back-fill; running the effective-date sweep.
- `core:approve`: approve, reject or rescind actions. Approval is never by the action's initiator
  (`CORE_SELF_APPROVAL`, 403).
- **Self**: `GET /workers/me`, the caller's own assignment and history (authentication only).

## 3. Requirements

### Job architecture (CH-001)

| ID | Requirement |
|---|---|
| CO-001 | Job families `{code, name, description?}`; grades `{code, name, rank ≥ 1}`; job profiles `{code, title, family_id, default_grade_id?, category: staff\|management\|executive, probation_days 0–365 (default 90), description?}`. |
| CO-002 | Codes are 2–32 characters of `A–Z 0–9 _ -` (upper-cased), unique per tenant and immutable. Grade ranks are unique per tenant. |
| CO-003 | Nothing is deleted. `status: active\|inactive` via PATCH; an inactive family, grade or profile cannot be newly referenced (409 `INACTIVE_REFERENCE`). Existing references stay valid. |

### Positions (CH-002)

| ID | Requirement |
|---|---|
| CO-010 | Create `{code, title, job_profile_id, department_id, location_id?, budgeted_fte 0.01–99.99 (default 1), shared (default false)}`. The department and location are checked through HR/Organization. |
| CO-011 | List or get with occupancy as of `as_of` (default today): `filled_fte`, `incumbents`, `open_fte`, and `vacant` (no incumbent). Filters: `department_id`, `job_profile_id`, `status`, `vacant`. |
| CO-012 | Update title, job profile, department, location, budgeted FTE and shared. The budgeted FTE cannot drop below the current or any future filled FTE (409 `POSITION_OVERFILLED`). |
| CO-013 | Status `active → frozen → active`, `active\|frozen → closed`. Closed is final. Closing needs no current or future incumbent (409 `POSITION_OCCUPIED`). |
| CO-014 | Occupancy: a non-shared position holds at most one worker on any date. A shared one holds Σ FTE ≤ budgeted FTE on every date. Violations → 409 `POSITION_FILLED`. Only `active` positions take new incumbents (409 `POSITION_STATE`). |

### Assignments (CH-003, CH-011)

| ID | Requirement |
|---|---|
| CO-020 | A worker's active assignments never overlap. A database exclusion constraint backs this, and a per-worker lock serialises writes. |
| CO-021 | `GET /workers/{employee_id}?as_of`: the assignment in effect on the date (or none), the full history (rescinded rows flagged) and the action log. `GET /workers/me` is the same for the caller. |
| CO-022 | `GET /assignments?as_of&department_id&position_id&page&per_page`: who held what on a date (the "as of" org view). |

### HR actions (CH-004..CH-009)

| ID | Requirement |
|---|---|
| CO-030 | Lifecycle (HR-013): `draft → submitted → completed`, `draft\|submitted → cancelled`, `submitted → rejected`, `completed → rescinded`. Approval completes the action at once (single step until M7). Illegal moves → 409 `INVALID_STATE`. |
| CO-031 | Create `{type, employee_id, effective_date, reason_code, comment?, changes}` as draft. Only drafts can be edited. Submit validates fully. Approve re-validates under the worker lock. |
| CO-032 | Sequencing: an action's effective date must be after the start of the worker's latest active assignment, and no completed action with a later effective date may exist (409 `LATER_CHANGE_EXISTS`). |
| CO-033 | `hire`: the worker has no active assignment. It needs a job profile, department, worker type and employment type, plus optionally a position, location, manager, grade (default: the profile's default grade) and FTE (default 1). The probation end is the effective date + the profile's probation days (none when 0). |
| CO-034 | `promote` / `demote`: needs a grade whose rank is higher / lower than the current one, unless `grade_override_reason` is given. `transfer`: a different department or location. `change_job`: a different job profile or position. `change_manager`: a different manager. `change_location`: a different location. Any of these may carry other field changes. |
| CO-035 | Completing a change closes the current assignment at the effective date and opens a new one with the changes merged. Future-dated actions complete now and take effect on their date. Reads "as of" a date see the change only from its date. |
| CO-036 | `probation_confirm` `{outcome: confirm\|extend, probation_end_date (extend only, after the current end)}`. It needs a current assignment with an open probation. Extending writes a new assignment row with the new end; confirming clears it. |
| CO-037 | `terminate` `{termination_type: voluntary\|involuntary, last_working_day ≥ effective_date, notice_date?, eligible_for_rehire}` plus the reason code. The assignment closes after the last working day (`effective_to = LWD + 1`), which frees the position. |
| CO-038 | `rehire`: the worker's status is terminated or resigned and there is no open assignment. It needs the same fields as hire. The worker record and history are kept. |
| CO-039 | Rescind `{reason}`: only the worker's latest completed action. It marks the action's assignment rescinded, restores the previous assignment's `effective_to`, and reverses effects already applied (CO-040). |

### Effects and projections (CH-005, CH-006, CH-012)

| ID | Requirement |
|---|---|
| CO-040 | Core is the system of record. When an action takes effect (at completion if its date is today or earlier, otherwise by the effective-date sweep), core projects onto the other contexts: the worker's primary organization mapping (department, location, manager), the employee status (hire / rehire → probation or active, probation confirmation → active, termination → terminated with exit date and reason) and the login (termination deactivates it, rehire reactivates it, rescinding a termination restores both). |
| CO-041 | The sweep runs hourly and on demand (`POST /effects/run`). It is idempotent: an action's effects are applied once (`effects_applied_at`). |
| CO-042 | Back-fill `POST /backfill`: every worker without an assignment gets one initial assignment from their joining date (department, location and manager from their primary mapping, employment type from HR/Employee). It records a completed `hire` action with `reason_code = backfill`. It is idempotent. |

### Headcount (CH-010)

| ID | Requirement |
|---|---|
| CO-050 | Plans `{department_id, period_start, period_end, planned_fte}`. Periods do not overlap per department (409). |
| CO-051 | `GET /headcount?as_of&department_id`: per department, the planned FTE (the plan covering the date), the budgeted FTE and filled FTE of active positions, open positions, and the workers with an assignment in effect. |

## 4. API (32 operations under `/api/v1/hr/core`)

| Area | Operations |
|---|---|
| Job architecture | `POST/GET /job-families`, `PATCH /job-families/{id}`; `POST/GET /grades`, `PATCH /grades/{id}`; `POST/GET /job-profiles`, `GET/PATCH /job-profiles/{id}` |
| Positions | `POST/GET /positions`, `GET/PATCH /positions/{id}`, `POST /positions/{id}/status` |
| Actions | `POST/GET /actions`, `GET/PATCH /actions/{id}`, `POST /actions/{id}/submit\|approve\|reject\|cancel\|rescind` |
| Workers and assignments | `GET /workers/me`, `GET /workers/{employee_id}`, `GET /assignments` |
| Headcount | `POST/GET /headcount-plans`, `GET /headcount` |
| Operations | `POST /backfill`, `POST /effects/run` |

## 5. Errors

| Status | Codes |
|---|---|
| 400 | `VALIDATION_ERROR` |
| 403 | `FORBIDDEN`, `CORE_SELF_APPROVAL` |
| 404 | `NOT_FOUND`, `EMPLOYEE_NOT_FOUND` |
| 409 | `CONFLICT`, `INVALID_STATE`, `INACTIVE_REFERENCE`, `LATER_CHANGE_EXISTS`, `NO_ASSIGNMENT`, `ALREADY_ASSIGNED`, `POSITION_FILLED`, `POSITION_STATE`, `POSITION_OCCUPIED`, `POSITION_OVERFILLED`, `GRADE_RULE`, `NO_CHANGE`, `PROBATION_STATE`, `NOT_TERMINATED`, `NOT_LATEST` |

## 6. Events (`jaas.core.events`, HR-009)

`hr.core.action_submitted`, `hr.core.action_completed`, `hr.core.action_rescinded`, `hr.core.position_changed`.
They carry ids, the type, status and effective date only. They never carry the comment, the termination reason or
compensation.

## 7. Out of scope (v1)

- Effective-dated position attributes; position history is read from events and audit (DC-05).
- Multi-step approval chains (M7).
- Employee-code policy on rehire (§11 open question): v1 keeps the code.
- The secondary/concurrent assignments of one worker.

## 8. Implementation notes (2026-10-21)

- Calendar dates are UTC in v1. Tenant time zones (HR-007) arrive with the tenant settings.
- Terminating a worker revokes their sessions through M0. Reactivating the login on rescind or rehire does not
  revive them, so the worker signs in again.
