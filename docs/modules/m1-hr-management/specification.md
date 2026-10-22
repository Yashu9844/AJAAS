# M1 HR Management — Specification

> HR-wide contract. Each built context's frozen contract lives in `contexts/<context>/specification.md` and stays
> authoritative for that context. This file adds what spans contexts: actors and permissions, HR-wide rules,
> cross-context contracts, the Workday-parity scope for built contexts, and the requirements for the five planned
> contexts. Planned-context sections are **proposed**: each is frozen (with goldens) at that context's design phase
> and may be refined then, but not reduced without a recorded decision.

## 1. Purpose and quality bar

HR Management is the system of record for people in a tenant: who works here, in which job and position, reporting
to whom, on what pay, with what time, leave, benefits, learning and performance — and the full history of each.

Quality bar (applies to every context, built or planned):

- **Complete before wide.** A capability ships only when its data model, rules, permissions, audit, events, API,
  unit + integration + golden tests and test-console page are all done. No partial features behind "v2" labels for
  anything inside the agreed scope.
- **History is never lost.** Effective-dated records and append-only ledgers; corrections supersede or reverse,
  they never silently overwrite.
- **Exact numbers.** Money in integer paise, days and scores in integer hundredths, no floating point anywhere in a
  calculation path.
- **Private by default.** Statutory identifiers, bank details, salary, ratings and health/benefit data are masked
  or withheld unless the caller holds the specific permission; events and audit rows carry IDs, never those values.
- **Every write is safe to retry** (idempotency) and **safe under concurrency** (row/advisory locks, unique
  constraints, version checks).

## 2. Actors and permissions

### 2.1 Actors (HR personas)

| Actor | Typical M0 role | Does |
|---|---|---|
| HR administrator | `hr_admin` (tenant-defined) | configures structures, policies, plans; full HR data access incl. sensitive |
| HR partner / generalist | `hr_partner` | runs lifecycle actions, onboarding, leave and attendance administration for assigned orgs |
| Payroll administrator | `payroll_admin` | assigns CTC, prepares and calculates payroll runs; cannot approve own runs |
| Payroll approver / finance | `payroll_approver` | approves and finalizes runs, downloads payout files |
| Recruiter | `recruiter` | manages jobs, candidates, interviews, offers, hires |
| People manager | any role + reporting relationship | sees and acts on direct (and, where allowed, indirect) reports: approvals, reviews, check-ins, merit proposals |
| Employee (self-service) | `employee` | own profile, punches, leave, payslips, goals, learning, benefit elections |
| Auditor | `auditor` | read-only, including audit logs; no sensitive values unless granted |
| Tenant admin | `tenant_admin` | bypasses permission checks (M0 rule); still audited |

Role names are tenant data, not code. Code checks permissions only.

### 2.2 Permission model

- Permission strings are `resource:action`. Every HR resource is prefixed by its context, e.g. `leave:approve`,
  `payroll:manage`. Planned contexts add `core:*`, `onboarding:*`, `compensation:*`, `learning:*`, `benefits:*`.
- Access follows `docs/ARCHITECTURE.md` §17: every grant is a permission **with a scope** — `own`,
  `direct_reports`, `all_reports`, `department`, `tenant` — held through predefined or custom roles (with
  inheritance), the automatic `employee` and `manager` roles, or per-user grants, minus per-user blocks.
- HR evaluates the scope against its own facts: `own` = the caller's employee record (self-service endpoints need
  authentication only); `direct_reports` / `all_reports` from `organization` reporting lines; `department` = the
  caller's department and its sub-departments; `tenant` = everyone. Each HR endpoint declares which scopes it accepts.
  Manager-type scopes never grant sensitive fields on their own.
- Predefined-role defaults HR registers (examples; full table frozen per context): employee — own records;
  manager — `leave:approve`, `attendance:approve`, `performance:manage` at `direct_reports`, `employee:read` at
  `all_reports`; hr_manager — HR operations at `tenant`; hr_admin — everything in HR incl. sensitive at `tenant`;
  payroll_admin — `payroll:read|manage`; payroll_approver — `payroll:read|approve`; recruiter — `recruitment:*`;
  interviewer — own interviews only.
- **Sensitive** reads/writes need a separate action (`employee:read_sensitive`, `compensation:read`,
  `benefits:read_sensitive`, ...). Masking is the default.
- **Separation of duties:** the maker of a payroll run, merit cycle, or high-impact lifecycle action cannot approve
  it (`*_SELF_APPROVAL` error). Self-approval of one's own leave, regularization or compensation is always blocked.
- Permissions are registered in the M0 catalogue by the HR service at boot (idempotent). Target: registration through
  M0's contract, not by writing M0 tables (see `design.md` §4).

## 3. HR-wide business rules

| ID | Rule |
|---|---|
| HR-001 | Every HR row carries `tenant_id`; every query, cache key, event, export and job is tenant-scoped. Foreign-tenant IDs return 404. |
| HR-002 | The worker identity inside HR is `employee_profile_id`. `user_id` (M0) is only for authentication and self-service resolution. A worker may exist without a login (e.g. pre-hire, terminated). |
| HR-003 | Effective dating: records that describe a worker's situation over time (job, position, compensation, benefits, shift assignment) have `effective_from` and optional `effective_to` (exclusive). For one worker and one kind of record, periods never overlap. A future-dated change is stored now and becomes current on its date without a batch job (reads resolve "as of" a date). |
| HR-004 | Corrections: a mistaken effective-dated record is **corrected** (new version, `supersedes_id`, reason, audit) or **rescinded** (marked rescinded, reason, audit, downstream compensation via events). Nothing is deleted. |
| HR-005 | Ledgers (leave balances, benefit deductions, payroll) are append-only; balances are derived or cached from the ledger and reconciled by tests. |
| HR-006 | Money: integer paise (`Money`), currency INR in v1, rounding half-up at defined points only. Days and percentages: integer hundredths (`Days`, `Fixed`). |
| HR-007 | Dates are calendar dates in the tenant's time zone; instants are UTC `timestamptz`. Conversions happen at the edge with the tenant (or shift) zone. |
| HR-008 | Lifecycle state machines are explicit (allowed transitions table); illegal transitions return 409 `INVALID_STATE` and change nothing. |
| HR-009 | Every state change writes an audit record (actor, action, resource, id, outcome, correlation ID) after commit, and an outbox event inside the same transaction. Audit and events never contain salaries, statutory IDs, bank data, ratings, health/benefit data, free-text feedback or personal contact data. |
| HR-010 | Every retryable command accepts `Idempotency-Key`; replays return the original result; a reused key with a different payload returns 409 `IDEMPOTENCY_KEY_REUSED` (shared primitive, `docs/ARCHITECTURE.md` §3.6). |
| HR-011 | Concurrency: contested aggregates (balance, run, candidate, job seat, position, plan enrolment) are locked per aggregate inside the transaction; uniqueness invariants are also database constraints. |
| HR-012 | Lists are paginated (`page` ≥ 1, `per_page` 1–100, default 20) with allowlisted filters and sorts. |
| HR-013 | HR business processes (lifecycle actions, compensation changes, enrolments, regularizations, leave) follow one request shape: `draft → submitted → approved → completed`, or `rejected` / `cancelled`; approvals run in-context (single step) until M7 Approvals exists, then delegate to it without changing the HR API. |
| HR-014 | Deactivating a worker's login (M0) never deletes HR data; termination in HR triggers M0 deactivation, not the reverse, once `core` exists. |

## 4. Context `core` — job architecture, positions and lifecycle (built 2026-10-21; contract in `contexts/core/specification.md`)

**Purpose:** the Workday "Core HCM" backbone: what jobs exist, which positions are budgeted, which worker holds
which job/position over time, and the business processes that change it.

**Entities (proposed):** `JobFamily`, `JobProfile` (title, family, level, default grade, FLSA-like exempt flag not
needed in India → `category`), `Grade` (code, rank), `Location` (if not delivered in `organization`, see §9),
`Position` (job profile, department, location, budgeted FTE, status `open|filled|frozen|closed`, effective-dated),
`JobAssignment` (worker, position or job profile, department, manager, location, grade, worker type
`employee|contractor|intern`, employment type, FTE, `effective_from/to`, reason), `HRAction` (business process
instance: `hire|promote|demote|transfer|change_job|change_manager|change_location|probation_confirm|terminate|rehire`,
proposed changes, effective date, status per HR-013, reason codes), `TerminationDetail` (type voluntary/involuntary,
reason, notice dates, last working day, eligible for rehire), `HeadcountPlan` (department, period, planned FTE).

| ID | Requirement |
|---|---|
| CH-001 | CRUD for job families, job profiles and grades; codes unique per tenant; profiles cannot be deleted while referenced, only inactivated. |
| CH-002 | Positions: create, edit (effective-dated), freeze, close. A position is filled by at most one worker at a time (or by FTE sum ≤ budgeted FTE if `shared=true`). |
| CH-003 | Every worker has exactly one primary `JobAssignment` covering any date between hire and termination; periods never overlap (HR-003). |
| CH-004 | `HRAction` types above, each with required fields and validation (e.g. promote requires a higher grade rank or explicit override reason; transfer requires a different department or location). |
| CH-005 | Completing an action writes the new `JobAssignment` effective on the action's date, closes the previous one, updates position occupancy, and publishes `hr.core.action_completed` with IDs only. Future-dated actions complete immediately but take effect on their date. |
| CH-006 | Terminate: sets last working day, closes the assignment and position occupancy, moves employee status to `terminated` on that date, starts offboarding (`onboarding` context), and requests M0 login deactivation effective that day. |
| CH-007 | Rehire: reuses the worker record (`employee_profile_id`), keeps history, new assignment and employee code policy per tenant setting. |
| CH-008 | Rescind/correct an action per HR-004, with downstream events so payroll, leave and benefits react. |
| CH-009 | Probation: end date derived from job profile/tenant default; confirm or extend via `probation_confirm` action. |
| CH-010 | Headcount: per department and period, planned vs filled vs open positions; read API for reports. |
| CH-011 | Worker history API: full effective-dated job history and action log for a worker; "as of date" view of the org (who held which position on a date). |
| CH-012 | Migration: existing `employee` employment fields and `organization` mappings are back-filled into one initial `JobAssignment` per worker (effective from joining date) without changing their APIs; those APIs become projections of `core`. |

**Events:** `hr.core.action_submitted`, `hr.core.action_completed`, `hr.core.action_rescinded`, `hr.core.position_changed`.
**Integrations:** `organization` (departments, managers), `employee` (profile, status), `payroll` (assignment effective
dates, termination for final settlement), `onboarding` (hire and terminate triggers), `compensation` (grade, salary on
promotion), M0 (login deactivation/reactivation).
**Golden candidates:** no overlapping assignments under concurrent actions; future-dated promotion visible only from
its date; rescinded termination restores assignment, position and login; position never double-filled; full history
reproducible "as of" any date.

## 5. Context `onboarding` — onboarding and offboarding (built 2026-10-22; detail in `contexts/onboarding/`)

**Entities (proposed):** `ChecklistTemplate` (kind `onboarding|offboarding`, applicability by department/job
profile/location/worker type), `TemplateTask` (title, description, owner role `employee|manager|hr|it|finance`,
due offset in days relative to start / last working day, requires document type, blocking flag), `Checklist` (worker,
kind, triggering action, status), `ChecklistTask` (assignee, due date, status `open|done|skipped|blocked`, evidence
reference), `DocumentRequirement` (type, mandatory, verification state — metadata now, file lifecycle in M8 later),
`ExitClearance` (department sign-offs, asset return status placeholder for M4), `ExitInterview` (structured answers,
restricted visibility).

| ID | Requirement |
|---|---|
| ON-001 | Templates with tasks and applicability rules; the most specific matching template(s) apply. |
| ON-002 | A hire (`recruitment` hire or `core` hire action) creates the onboarding checklist idempotently, with due dates from the start date. |
| ON-003 | A termination creates the offboarding checklist from the last working day. |
| ON-004 | Task assignees see "my tasks" across workers; managers see their reports' checklists; HR sees all. |
| ON-005 | Document requirements are satisfied by employee document records (verified by HR); blocking tasks prevent checklist completion. |
| ON-006 | Exit clearance: each required sign-off recorded with actor and time; final settlement is released to `payroll` only when clearance is complete (event `hr.onboarding.clearance_completed`). |
| ON-007 | Pre-boarding: the new hire can complete their own tasks once their login is active (invited by M0 at hire). |
| ON-008 | Progress and overdue reporting per checklist and per department. |

**Golden candidates:** duplicate hire event creates one checklist; blocking task holds completion; final settlement not
released before clearance; employee cannot see exit-interview answers of others.

## 6. Context `compensation` (built 2026-10-21; detail in `contexts/compensation/`)

**Entities (proposed):** `PayGrade` (grade, currency, min/mid/max annual amounts, effective-dated), `CompensationPlan`
(kind `salary|allowance|bonus_target`), `CompensationRecord` (worker, annual base, plan, effective-dated, reason
`hire|promotion|merit|market|correction`, linked HR action), `MeritCycle` (period, budget pool by department, guideline
matrix: performance rating × compa-ratio band → recommended % range, status `draft|open|in_review|approved|applied`),
`MeritProposal` (worker, manager, current, proposed %, amount, justification, status).

| ID | Requirement |
|---|---|
| CP-001 | Pay grades with ranges; compa-ratio = base / midpoint and range penetration computed exactly (hundredths). |
| CP-002 | Compensation history is effective-dated (HR-003) and never overwritten; every change has a reason and is linked to an action or a merit cycle. |
| CP-003 | A compensation change effective on a date updates the `payroll` CTC assignment from that date through a port (payroll keeps calculating; compensation never computes payroll). |
| CP-004 | Merit cycle: HR opens a cycle with budgets and guidelines; managers propose for their reports within guidelines (outside needs justification); budget usage shown live; HR approves; applying the cycle writes compensation records effective on the cycle date. |
| CP-005 | Inputs from `performance` (final rating of the linked review cycle) are read-only snapshots taken when the merit cycle opens. |
| CP-006 | Visibility: managers see reports' compensation only during an open cycle and only for their reports; employees see their own; everything else needs `compensation:read`. |
| CP-007 | Maker-checker on applying a cycle and on off-cycle changes above a tenant threshold. |

**Golden candidates:** budget never exceeded without override; applying a cycle twice is a no-op; payroll CTC from the
effective date only; compa-ratio exact; manager cannot see non-reports.

## 7. Context `learning` (planned)

Content is **not hosted**: a course points to an external URL or a document (metadata here, file in M8 later), with a
thumbnail image reference. No video streaming or SCORM in scope.

**Entities (proposed):** `Course` (title, description, provider, content kind `link|document`, URL or document
reference, thumbnail reference, duration estimate, tags, status `draft|published|retired`, version), `LearningPlan`
(ordered courses, optional due offset, custom per tenant), `Assignment` (target worker, or a rule: department / job
profile / location / all, due date, recurrence for compliance e.g. yearly, mandatory flag), `Enrolment` (worker,
course or plan, source `assigned|self`, status `not_started|in_progress|completed|overdue|waived`, completed at,
evidence), `Completion` (attestation `self|manager|hr`, optional score, certificate reference).

| ID | Requirement |
|---|---|
| LN-001 | Catalogue CRUD with versioning; retiring a course keeps history and existing completions. URLs validated (https only, no `javascript:`/data URLs); thumbnails are image references, never inline blobs. |
| LN-002 | Learning plans built from courses in order; custom plans per tenant; a plan is complete when all mandatory items are complete. |
| LN-003 | Assignments to individuals or rules; rule assignments materialize enrolments for current matches and for new joiners/movers via `core` events, idempotently. |
| LN-004 | Self-enrolment in published courses/plans where allowed by the course. |
| LN-005 | Completion: self-attest, or manager/HR verification when the course requires it; completions are immutable (corrections supersede). |
| LN-006 | Recurring compliance: a new enrolment is generated on the recurrence date; overdue status is computed from due date. |
| LN-007 | Learner transcript (own), team view (manager), compliance report by department and course (HR). |

**Golden candidates:** rule assignment enrolls a new joiner exactly once; retired course keeps completions; overdue
computed correctly across time zones; unsafe URL rejected.

## 8. Context `benefits` (planned)

**Entities (proposed):** `BenefitPlan` (tenant-defined: name, category `health|life|accident|retirement|wellness|other`,
provider name, coverage levels, employee and employer cost per level in paise per pay period, plan year, status),
`EligibilityRule` (worker type, grade range, location, minimum tenure days, employment type), `EnrolmentWindow`
(`new_hire` with N days from start, `open_enrolment` dates, `life_event` with N days from event), `Dependant` (name,
relationship, date of birth — sensitive), `Election` (worker, plan, coverage level, dependants covered, effective
dates, status `pending|active|waived|ended`), `LifeEvent` (type `marriage|birth|adoption|divorce|death|other`, date,
evidence reference).

| ID | Requirement |
|---|---|
| BN-001 | Plan CRUD per plan year with coverage levels and costs; plans cannot change costs mid-year except by a new effective-dated version. |
| BN-002 | Eligibility evaluated per worker as of a date from `core` assignment data; ineligible plans are not offered. |
| BN-003 | Enrolment only inside an open window for that worker; outside a window changes are rejected with `ENROLMENT_WINDOW_CLOSED`. |
| BN-004 | Elections are effective-dated; one active election per worker per plan; waiving is an explicit election. |
| BN-005 | Employee cost of active elections is handed to `payroll` as a per-period deduction through a port; employer cost is reported, not deducted. |
| BN-006 | Termination ends elections on the termination date (or plan rule) and stops deductions. |
| BN-007 | Dependants and life events are sensitive: visible to the worker and to `benefits:read_sensitive` only; never in events or audit. |
| BN-008 | Reports: enrolment counts and costs per plan (no personal data). |

**Golden candidates:** enrolment outside window rejected; one active election per plan under concurrent requests;
deduction stops after termination; dependant data never leaks.

## 9. Workday-parity scope for built contexts

Items marked **P1** are part of "HR complete" and are scheduled in Phase E; **P2** items wait for a dependency or a
product decision. Each is added to its context's specification (with goldens) when scheduled.

| Context | Item | Priority | Note |
|---|---|---|---|
| organization | Locations (address, time zone, state for PT/holidays) | P1 | done 2026-10-20 (FR-L001..FR-L007) |
| organization | Effective-dated reporting changes via `core` actions | P1 | mappings become a projection of `core` assignments (CH-012) |
| organization | Bulk import (CSV) with validation report | P2 | |
| organization | Cost-centre references | P2 | cost centres are owned by M5 Finance |
| employee | Personal-data change requests with approval (bank, address, name) | P1 | done 2026-10-21 (FR-CR) |
| employee | Emergency contacts, education, prior employment | P1 | done 2026-10-21 (FR-ER) |
| employee | Document files (upload/download) | P2 | M8 Documents; metadata exists today |
| attendance | Manager-scoped approval inbox | P1 | done 2026-10-22 (AP-01, AP-02) |
| attendance | Overtime calculation and rosters / shift rotations | P1 | done 2026-10-22 (AP-03..AP-06) |
| attendance | Nightly auto-absent job, bulk import | P1 | done 2026-10-22 (AP-07..AP-11) |
| attendance | Geofence enforcement (coordinates already stored) | P2 | |
| leave | Manager-routed approvals and team calendar | P1 | done 2026-10-22 (LP-06, LP-07); multi-step through M7 later |
| leave | Per-location holiday calendars and weekly-off patterns | P1 | done 2026-10-22 (LP-03..LP-05) |
| leave | Comp-off, encashment, non-calendar leave year | P1 | done 2026-10-22 (LP-01, LP-02, LP-08..LP-15) |
| leave | Proof attachments | P2 | M8 Documents |
| payroll | Employer PF/ESI contributions, state-wise PT slabs | P1 | |
| payroll | Arrears and mid-month CTC change split | P1 | needed by compensation and core |
| payroll | Benefits deductions, final settlement (gratuity, leave encashment) | P1 | needed by benefits and offboarding |
| payroll | Investment declarations, old tax regime, payslip PDF | P2 | |
| recruitment | Requisitions tied to positions (headcount) | P1 | depends on core |
| recruitment | Competency scorecards, multi-step offer approval | P1 | |
| recruitment | Careers portal, resume upload/parsing | P2 | M8 Documents |
| performance | Calibration and rating override with audit | P1 | done 2026-10-20 (PM-018..022) |
| performance | Continuous feedback | P1 | done 2026-10-20 (FB-001..006) |
| performance | Templates per role, peer nomination | P2 | |

## 10. Cross-context contracts

Current in-process contracts (to be re-expressed as each context's `api` package in Phase C, `design.md` §3):

| Provider → consumer | Contract | Purpose |
|---|---|---|
| employee → attendance, leave, payroll, recruitment, performance | `GetByID`, `GetByUserID`, `List` (directory) | resolve workers |
| employee → recruitment | `CreateEmployee` | hire saga step |
| organization → recruitment | `DepartmentService`, `DesignationService` reads | validate job references |
| organization → performance | reporting line (manager of a worker) | manager access and reviews |
| attendance ← leave | `LeaveSync.MarkLeave/ClearLeave` | approved leave marks `on_leave` days |
| leave → payroll | `UnpaidLeave.UnpaidDays` | loss-of-pay days |
| M0 → all contexts | authentication, authorization, user invite, audit | through `identityport` after Phase C |

Planned additions: core → employee/organization (projection), core → payroll (assignment dates, termination),
compensation → payroll (CTC), benefits → payroll (deductions), onboarding → payroll (clearance), performance →
compensation (ratings), core → learning/benefits/onboarding (worker events).

## 11. Open questions (resolve at the owning context's design phase)

- Employee code policy on rehire (reuse vs new) — tenant setting proposed.
- Whether positions are mandatory (position management) or optional (job management) per tenant — proposed tenant
  setting, default optional.
- Benefit costs per pay period vs per month — proposed per month (payroll is monthly).
- Which certificate format (if any) learning completions produce — proposed: none in v1, evidence reference only.
