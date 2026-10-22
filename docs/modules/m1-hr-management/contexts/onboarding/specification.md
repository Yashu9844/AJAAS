# HR / Onboarding — Specification

> Part of **M1 HR Management** (bounded context `onboarding`). Requirements ON-001..ON-008 in
> `../../specification.md` §5; this file is the frozen detail.

## 1. Scope

Onboarding and offboarding checklists built from templates, started by HR events (core hire/rehire/terminate,
recruitment hire) or by HR by hand. Tasks have owners, due dates, optional document requirements and a blocking
flag. Offboarding adds exit clearance sign-offs and an exit interview.

## 2. Permissions

| Permission | Grants | Default |
|---|---|---|
| `onboarding:read` | all checklists, progress report | `hr_admin` |
| `onboarding:manage` | templates, manual start, pool tasks (hr/it/finance), skip/reopen, complete/cancel, clearance | `hr_admin` |
| `onboarding:exit_interview` (sensitive) | read anyone's exit interview | `hr_admin` |

Without a permission: the worker sees and completes their own `employee` tasks (pre-boarding, ON-007) and submits
their exit interview; the worker's manager sees their reports' checklists and completes `manager` tasks and the
`manager` clearance.

## 3. Requirements

| ID | Requirement |
|---|---|
| OB-01 | Template: `kind onboarding|offboarding`, name, optional criteria `department_id`, `job_profile_id`, `location_id`, `worker_type`, status `active|archived`. Tasks: title, description, `owner_role employee|manager|hr|it|finance`, `due_offset_days` (−90..365, relative to the anchor date), optional `document_type`, `blocking`. A task list is replaced as a whole (`PUT .../tasks`, 1–50 tasks). |
| OB-02 | Applicability (ON-001): a template applies when every criterion it sets matches the worker's placement on the anchor date. Among matching active templates of the kind, only those with the most criteria apply; their tasks are merged in template then task order. No matching template → a checklist with no tasks. |
| OB-03 | Start (ON-002, ON-003): a completed core `hire`/`rehire` starts onboarding anchored on the effective date; a completed `terminate` starts offboarding anchored on the last working day; a recruitment hire starts onboarding anchored on the joining date. Starting is idempotent: the same source never creates a second checklist, and a worker never has two open checklists of one kind (the second start is a no-op). Manual start `POST /checklists` by `onboarding:manage` returns 409 `CONFLICT` instead. Rescinding the core action cancels the checklist it started. |
| OB-04 | Tasks: `employee` tasks are assigned to the worker, `manager` tasks to the worker's manager on the anchor date (no manager → pool), `hr|it|finance` tasks to the pool (any `onboarding:manage` holder). Due date = anchor + offset. |
| OB-05 | Completing a task (`done`): its assignee or `onboarding:manage`. A task with `document_type` needs a **verified** employee document of that type (409 `DOCUMENT_REQUIRED`); its id is stored as evidence (ON-005). Skipping: `onboarding:manage`, with a reason, never a blocking task (409 `TASK_BLOCKING`). Reopen: `onboarding:manage`. Tasks of a closed checklist cannot change (409 `CHECKLIST_STATE`). |
| OB-06 | Completion: a checklist completes automatically when no task is open and (offboarding) clearance is complete. `onboarding:manage` may complete it early only when no **blocking** task is open (409 `BLOCKING_OPEN`) and clearance is complete (409 `CLEARANCE_PENDING`); remaining open tasks become `skipped`. Cancel: `onboarding:manage`, with a reason. |
| OB-07 | Exit clearance (ON-006): an offboarding checklist gets sign-offs `manager`, `hr`, `it`, `finance`. `manager` is signed by the worker's manager or `onboarding:manage`; the others by `onboarding:manage`. Each records actor, time and note; signing twice is 409 `ALREADY_CLEARED`. When the last one is signed the checklist records `clearance_completed_at` and emits `hr.onboarding.clearance_completed`. Payroll reads `ClearanceCompleted` through `onboarding/api` before releasing final settlement. |
| OB-08 | Exit interview: the departing worker submits answers once (`PUT`, 409 `ALREADY_SUBMITTED`), only on an open offboarding checklist. Readable by the worker and `onboarding:exit_interview` only (managers and `onboarding:read` get 403/404). Never in events or audit metadata. |
| OB-09 | Views (ON-004): `GET /my-tasks` (assigned to me, plus pool tasks for `onboarding:manage`; open first, by due date); `GET /me` (my checklists with tasks); `GET /checklists` (`onboarding:read`: all with filters; otherwise those where I am the worker or the manager); `GET /checklists/{id}` with tasks, clearance and progress `{total, done, skipped, open, overdue}`. |
| OB-10 | Progress report (ON-008): `GET /reports/progress?kind=` per department: open and completed checklists, open and overdue tasks. |

## 4. API (19 operations under `/api/v1/hr/onboarding`)

| Area | Operations |
|---|---|
| Templates | `POST/GET /templates`, `GET/PATCH /templates/{id}`, `PUT /templates/{id}/tasks` |
| Checklists | `POST/GET /checklists`, `GET /checklists/{id}`, `POST /checklists/{id}/complete\|cancel` |
| Tasks | `GET /my-tasks`, `POST /tasks/{id}/done\|skip\|reopen` |
| Clearance | `POST /checklists/{id}/clearances/{area}/clear` |
| Exit interview | `PUT/GET /checklists/{id}/exit-interview` |
| Views | `GET /me`, `GET /reports/progress` |

## 5. Events and privacy

`hr.onboarding.checklist_started`, `hr.onboarding.checklist_completed`, `hr.onboarding.clearance_completed` carry
ids, kind and dates only. Exit-interview answers and task notes never leave the context.

## 6. Golden tests

| # | Behaviour |
|---|---|
| OG1 | Isolation: foreign ids 404 |
| OG2 | Duplicate start (core hire then recruitment hire, or the same action twice) creates one checklist |
| OG3 | Most specific templates win |
| OG4 | Blocking task holds completion; skip refuses a blocking task |
| OG5 | Document task needs a verified document |
| OG6 | Clearance: checklist cannot complete before all sign-offs; event on completion; payroll port reports it |
| OG7 | Exit interview visible only to the worker and `onboarding:exit_interview` |
| OG8 | Rescinded hire cancels its checklist |
