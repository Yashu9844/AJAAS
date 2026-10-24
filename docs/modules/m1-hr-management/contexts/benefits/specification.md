# HR/Benefits — contract (built 2026-10-23)

Parent requirements: `../../specification.md` §8 (BN-001..BN-008). This file is the precise contract the code and the
tests follow. Package `backend/internal/hr/benefits`, tables `ben_*` (migrations hr/000089–000093), base path
`/hr/benefits`. Money is rupees with two decimals (`kernel.Money`) and **per month**, because payroll runs monthly.

## 1. Permissions

| Permission | Meaning | Default roles |
|---|---|---|
| `benefits:read` | plans, windows, settings and the aggregate enrolment report | hr_manager, hr_admin, payroll_admin |
| `benefits:manage` | plans, versions, windows, settings; ending a worker's election; recording a life event for a worker | hr_admin |
| `benefits:read_sensitive` | a worker's elections, dependants and life events (**sensitive**) | hr_admin |

Self-service (own options, elections, dependants, life events, expected deductions) needs authentication only. A manager
gets no access to a report's benefits.

## 2. Rules

| ID | Rule |
|---|---|
| BN-C01 | Plan: `code` (2–30, letters, digits, `_`, `-`; unique per tenant, case-insensitive; stored upper-case), `name` 1–150, `category` `health | life | accident | retirement | wellness | other`, `provider` ≤ 100, `plan_year` 2000–2100, `eligibility`, status `draft | active | retired`. The first **version** is created with the plan. |
| BN-C02 | A version is `{effective_from, levels}` with 1–10 coverage levels `{code, name, employee_cost, employer_cost, max_dependants}` (costs ≥ 0 per month, `max_dependants` 0–10, level codes unique within the version). A plan's costs never change in place: `POST /plans/{id}/versions` adds a version that starts strictly after the current latest version and not in the past; the previous version ends where the new one starts. Names, provider and eligibility can be patched; levels cannot. |
| BN-C03 | Eligibility `{worker_types, employment_types, grade_ids, location_ids, min_tenure_days}`; an empty list means no restriction; tenure is whole days from the joining date. It is evaluated from HR/Core placement and the employee profile on the date of the election. A plan the worker is not eligible for is not offered (`GET /options`) and electing it is refused (409 `NOT_ELIGIBLE`). |
| BN-C04 | An enrolment window is open for a worker when any of: an **open enrolment** window covers today (tenant-wide, `starts_on`–`ends_on` inclusive); the worker is within `new_hire_days` of their joining date (inclusive, default 30); a **life event** of theirs is within `life_event_days` of its date (inclusive, default 30). Otherwise an election, change or end by the worker is refused with 409 `ENROLMENT_WINDOW_CLOSED`. HR (`benefits:manage`) may end an election at any time. |
| BN-C05 | Election `{plan_id, level_code, dependant_ids?, effective_from?}`: the plan is `active`, the version in force on `effective_from` has the level, the dependants are the worker's own active ones and not more than the level's `max_dependants`, `effective_from` is today or up to 90 days ahead (default today). One live election per worker and plan: electing again while one is `active` or `waived` is 409 `ELECTION_EXISTS` (change or end it instead). |
| BN-C06 | Change `POST /elections/{id}/change {level_code, dependant_ids?, effective_from?}` ends the current election on the new one's start date and writes the new election, in one transaction. Waive `POST /elections/waive {plan_id}` records an explicit `waived` election from today and ends any active one; electing later replaces the waiver. End `POST /elections/{id}/end` ends it today. Statuses `active`, `waived` (no cost), `ended` (has an `effective_to`, exclusive; it still counts until that date). Ended rows are history and are never deleted. |
| BN-C07 | Payroll hand-off: `Deductions(tenant, employee, from, to)` returns one line `BEN_<PLAN CODE>` per election that is not waived, started on or before `to` and has no end or ends after `to`, at the employee cost of its level in the version in force on `to`. A whole month is charged if the election covers the last day of the period, nothing otherwise. Employer cost is reported, never deducted. |
| BN-C08 | Termination: when HR/Core completes a `terminate` action, every live election of the worker ends the day after the last working day (reason `termination`, linked to the action), so deductions stop with the month containing that day; rescinding the termination reinstates exactly the elections that termination ended. A failure here rolls the action back (unlike learning), because a missed end would keep deducting. |
| BN-C09 | Dependants `{name 1–100, relationship spouse | child | parent | other, date_of_birth}` (not in the future, not before 1900) belong to a worker and are managed only by them; removing one that an active election covers is 409 `DEPENDANT_IN_USE`. Life events `{type marriage | birth | adoption | divorce | death | other, event_date, evidence_ref?}` (not more than 90 days in the future) are recorded by the worker or by HR for a worker. Both are **sensitive**: visible to the worker and to `benefits:read_sensitive` only, never in events or audit metadata (BN-007). |
| BN-C10 | Report `GET /reports/enrolment` (`benefits:read`): per plan and level the number of active elections and waivers on today's date and the total monthly employee and employer cost. No personal data. |
| BN-C11 | Events and audit carry ids and statuses only (`benefits.plan.activated|retired`, `benefits.election.created|ended|waived`); never dependants, life events or costs. Tenant isolation: foreign ids are 404. |

## 3. Operations (29)

Plans — `POST /plans`, `GET /plans`, `GET /plans/{id}`, `PATCH /plans/{id}`, `POST /plans/{id}/versions`,
`POST /plans/{id}/activate`, `POST /plans/{id}/retire`.
Windows and settings — `POST /windows`, `GET /windows`, `POST /windows/{id}/cancel`, `GET /settings`, `PUT /settings`.
Elections — `GET /options`, `GET /elections/me`, `POST /elections`, `POST /elections/{id}/change`,
`POST /elections/waive`, `POST /elections/{id}/end`, `GET /elections`, `GET /deductions/me`.
Dependants — `POST /dependants`, `GET /dependants`, `PATCH /dependants/{id}`, `POST /dependants/{id}/remove`,
`GET /employees/{id}/dependants`.
Life events — `POST /life-events`, `GET /life-events`, `GET /employees/{id}/life-events`.
Reports — `GET /reports/enrolment`.

## 4. Errors

`NOT_FOUND`, `VALIDATION_ERROR`, `CONFLICT`, `PLAN_STATE`, `NOT_ELIGIBLE`, `ENROLMENT_WINDOW_CLOSED`, `ELECTION_EXISTS`,
`ELECTION_STATE`, `DEPENDANT_IN_USE`, `EMPLOYEE_NOT_FOUND`, `FORBIDDEN`.

## 5. Golden tests

`BG1` enrolment outside a window is rejected; `BG2` one live election per worker and plan under concurrent requests;
`BG3` deductions stop after termination and return if it is rescinded; `BG4` dependants and life events never leak
(not to managers, not into events or audit); `BG5` payroll deducts exactly the elected level's employee cost, and a cost
change applies only from its new version's start.
