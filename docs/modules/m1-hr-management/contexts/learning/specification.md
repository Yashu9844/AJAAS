# HR/Learning â€” contract (built 2026-10-23)

Parent requirements: `../../specification.md` Â§7 (LN-001..LN-007). This file is the precise contract the code and the
tests follow. Package: `backend/internal/hr/learning`. Tables `lrn_*` (migrations hr/000085â€“000088). Base path
`/hr/learning`. Content is never hosted: a course is metadata plus an external https link or an opaque document reference.

## 1. Permissions

| Permission | Meaning | Default roles |
|---|---|---|
| `learning:read` | read every enrolment, assignment, plan and compliance report | hr_manager, hr_admin |
| `learning:manage` | catalogue, plans, assignments, waivers, corrections, settings, recurrence run | hr_admin |

Learner endpoints (catalogue of published courses, own enrolments, start, complete, self-enrol, transcript, plan
progress) need authentication only. A manager verifies completions of their direct reports without a permission.

## 2. Rules

| ID | Rule |
|---|---|
| LN-C01 | Course: `title` 1â€“150, `description` â‰¤ 2000, `provider` â‰¤ 100, `content_kind` `link` or `document`, `duration_mins` 0â€“10000, up to 10 `tags` (lower-cased, â‰¤ 30), `verification` `self` (default), `manager` or `hr`, `self_enrol` (default false). A link needs `content_url`: https only, a host, no user-info, â‰¤ 500 characters (`javascript:`, `data:`, `http:` refused). A document needs `document_ref` (â‰¤ 200, opaque, no URL scheme). `thumbnail_ref` â‰¤ 500 is an https URL or an opaque reference, never a `data:` URL. A new course is `draft`, version 1. |
| LN-C02 | Updating title, description, provider, content, duration or tags of a draft or published course increments `version` and stores a snapshot; changing only `verification`, `self_enrol` or the thumbnail does not. A retired course cannot be edited (409 `COURSE_STATE`). `GET /courses/{id}/versions` lists snapshots. |
| LN-C03 | `publish` moves `draft â†’ published`; `retire` moves `draft | published â†’ retired`. Retiring keeps every enrolment and completion, and open enrolments can still be completed; a retired or draft course cannot be assigned or self-enrolled (409 `COURSE_STATE`). |
| LN-C04 | Plan: `name` 1â€“150 (unique, case-insensitive), `description` â‰¤ 1000, status `active | archived`. `PUT /plans/{id}/items` replaces the ordered items (1â€“50 distinct, non-retired courses, each `mandatory` or optional). A plan is complete for a worker when every mandatory item has a completed enrolment (waived counts as done). |
| LN-C05 | Assignment: `target_kind` `employee | department | job_profile | location | all` with `target_id` (absent for `all`), exactly one of `course_id` or `plan_id` (a published course, or an active plan whose courses are all published), `mandatory`, at most one of `due_date` (calendar date) or `due_in_days` (1â€“1000, counted from the enrolment date), and `recurrence_days` 30â€“3650 (needs `due_in_days`, not `due_date`). Creating it enrols every active worker who matches today and answers `{matched, created}`. Cancelling stops future enrolments and recurrence and keeps existing ones. |
| LN-C06 | Matching uses HR/Core placement on today's date (department, job profile, location) and the employee directory (`all`, `employee`). Enrolments are created idempotently under the key `assignment Â· course Â· cycle`, so running the same assignment twice, or a worker matching the same assignment through a rule and an individual assignment, never gives two enrolments for one assignment. |
| LN-C07 | Joiners and movers: when HR/Core completes a hire, rehire, transfer or promotion, the worker's new placement is matched against every active rule assignment (not `employee` ones) and the missing enrolments are created, inside the action's transaction but in a savepoint, so a learning failure can never roll back or block an HR action. A rescinded action or a termination enrols nobody. |
| LN-C08 | Self-enrolment: a published course with `self_enrol` (or an active plan whose courses all allow it) â†’ the worker gets one enrolment per course, source `self`. A worker with a live self enrolment (`not_started | in_progress`) in the same course gets 409 `ALREADY_ENROLLED`. |
| LN-C09 | Enrolment statuses: `not_started â†’ in_progress â†’ completed`, or `waived` from `not_started | in_progress`. `overdue` is not stored: an enrolment is overdue when its `due_date` is before today in the tenant's time zone and it is `not_started | in_progress`; it is returned with `overdue: true` and `status=overdue` filters on it. The time zone is a tenant setting (`PUT /settings`, IANA name, default `Asia/Kolkata`). |
| LN-C10 | Start: the learner moves their own `not_started` enrolment to `in_progress` (409 `ENROLMENT_STATE` otherwise). Complete: the learner submits `{score?, evidence_ref?, certificate_ref?}` (score 0â€“100; references â‰¤ 300). Course `verification` `self` â†’ the completion is `verified` and the enrolment `completed`; `manager` or `hr` â†’ the completion is `pending` and the enrolment stays `in_progress` until verified. A second submission while one is pending â†’ 409 `COMPLETION_PENDING`; completing a completed or waived enrolment â†’ 409 `ENROLMENT_STATE`. |
| LN-C11 | Verify / reject a pending completion. `manager`: the learner's direct manager or `learning:manage`; `hr`: `learning:manage` only; nobody verifies their own (403 `LEARNING_SELF_VERIFY`). Verify â†’ completion `verified`, enrolment `completed`. Reject needs a comment â†’ completion `rejected`, enrolment back to `in_progress`, and the learner may submit again. A decided completion â†’ 409 `COMPLETION_STATE`. |
| LN-C12 | Completions are immutable: nothing edits a completion's content. `POST /enrolments/{id}/correct` (`learning:manage`, reason required) writes a new `verified` completion with attestation `hr`, marks the earlier live one `superseded` and links `supersedes_id`; on an enrolment with no completion it completes it on the worker's behalf. A waived enrolment cannot be corrected. |
| LN-C13 | Waive (`learning:manage`, reason â‰¤ 500) â†’ `waived`; a waived or completed enrolment cannot be waived again (409 `ENROLMENT_STATE`). |
| LN-C14 | Recurrence: for each active assignment with `recurrence_days`, once today reaches `next_cycle_on` a new cycle's enrolments are created for every worker who matches then (same idempotent key), and `next_cycle_on` advances by `recurrence_days`; several missed cycles are caught up one at a time by repeated runs, never skipped silently. `POST /recurrence/run` (`learning:manage`) runs it now; a background loop runs it every hour. |
| LN-C15 | Views: `GET /enrolments/me` (own, filter `status`), `GET /transcript/me` (own completed enrolments with their verified completion), `GET /enrolments/team` (direct reports), `GET /enrolments` (`learning:read`, filters `employee_id`, `course_id`, `status`, `assignment_id`), `GET /plans/{id}/progress` (own, or `employee_id` with `learning:read`). |
| LN-C16 | Compliance report (`learning:read`): `GET /reports/compliance?department_id&course_id&mandatory` â†’ per department and course the counts `total, not_started, in_progress, completed, waived, overdue`. Department is the worker's department when they were enrolled. No personal data. |
| LN-C17 | Events and audit carry ids and statuses only (`learning.course.published`, `learning.course.retired`, `learning.assignment.created`, `learning.assignment.cancelled`, `learning.enrolment.completed`, `learning.enrolment.waived`); never scores, evidence, comments or free text (HR-009). |
| LN-C18 | Tenant isolation: every row carries `tenant_id`; a foreign tenant's id is 404. |

## 3. Operations (35)

Catalogue â€” `POST /courses`, `GET /courses`, `GET /courses/{id}`, `PATCH /courses/{id}`, `POST /courses/{id}/publish`,
`POST /courses/{id}/retire`, `GET /courses/{id}/versions`, `GET /catalog`.
Plans â€” `POST /plans`, `GET /plans`, `GET /plans/{id}`, `PATCH /plans/{id}`, `PUT /plans/{id}/items`,
`GET /plans/{id}/progress`.
Assignments â€” `POST /assignments`, `GET /assignments`, `GET /assignments/{id}`, `POST /assignments/{id}/cancel`.
Enrolments â€” `POST /enrolments/self`, `GET /enrolments/me`, `GET /enrolments/team`, `GET /enrolments`,
`GET /enrolments/{id}`, `POST /enrolments/{id}/start`, `POST /enrolments/{id}/complete`,
`POST /enrolments/{id}/waive`, `POST /enrolments/{id}/correct`, `GET /transcript/me`.
Completions â€” `GET /completions/pending`, `POST /completions/{id}/verify`, `POST /completions/{id}/reject`.
Operations â€” `POST /recurrence/run`, `GET /reports/compliance`, `GET /settings`, `PUT /settings`.

`GET /completions/pending` lists what the caller can verify (their reports' for a manager, every one for
`learning:manage`).

## 4. Errors

`NOT_FOUND`, `VALIDATION_ERROR`, `COURSE_STATE`, `PLAN_STATE`, `ALREADY_ENROLLED`, `ENROLMENT_STATE`, `COMPLETION_PENDING`,
`COMPLETION_STATE`, `LEARNING_SELF_VERIFY`, `EMPLOYEE_NOT_FOUND`, `FORBIDDEN`.

## 5. Golden tests

`LG1` an assignment rule enrols a new joiner exactly once however often it runs; `LG2` a retired course keeps its
completions; `LG3` overdue follows the tenant's time zone; `LG4` unsafe URLs are refused; `LG5` a completion needing
verification cannot complete the enrolment until a different person verifies it; `LG6` events and audit hold ids only.

