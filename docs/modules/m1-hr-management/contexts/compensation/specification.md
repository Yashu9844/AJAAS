# HR / Compensation — Specification

> Part of **M1 HR Management** (bounded context `compensation`). Requirements CP-001..CP-007 come from
> `../../specification.md` §6; this file is the v1 contract (2026-10-21).

## 1. Glossary

- **Pay range**: min / mid / max annual base (INR, paise) for a core grade, effective-dated.
- **Compensation record**: a worker's annual base over `[effective_from, effective_to)`, with a reason and a source
  (an off-cycle change or a merit cycle). It is never overwritten (HR-003).
- **Compa-ratio**: base ÷ range midpoint, in hundredths (1.00 = at midpoint). **Range penetration**: (base − min) ÷
  (max − min) × 100.
- **Merit cycle**: a review of base pay with per-department budgets and a guideline matrix (rating band ×
  compa-ratio band → recommended % range).

## 2. Permissions

| Permission | Allows | Default role |
|---|---|---|
| `compensation:read` | read anyone's compensation, ranges and cycles | `hr_admin`, `payroll_admin` |
| `compensation:manage` | ranges, off-cycle changes, merit cycles (create, budgets, open, close for review) | `hr_admin` |
| `compensation:approve` | approve off-cycle changes and merit cycles; never one's own (`COMP_SELF_APPROVAL`) | `executive` |

Managers act on their reports' proposals while a cycle is open, with no permission needed. Employees read their
own records.

## 3. Requirements

### Pay ranges (CP-001)

| ID | Requirement |
|---|---|
| CO-C01 | Pay range `{grade_id (core grade), min ≤ mid ≤ max (> 0), effective_from}`. A new range for the same grade closes the previous one the day before. Ranges never overlap per grade, and back-dating before the current start is refused (409). |
| CO-C02 | Compa-ratio and penetration are computed exactly (hundredths, half away from zero) against the range in effect on the date for the worker's core grade on that date. Without a grade or range they are null. |

### Compensation records and off-cycle changes (CP-002, CP-003, CP-007)

| ID | Requirement |
|---|---|
| CO-C10 | `POST /changes {employee_id, annual_base, effective_from, reason: hire\|promotion\|market\|correction, comment?}` creates an off-cycle change. If it is the worker's first record, or the increase is at most 10% of the current base, it is applied at once. Otherwise it waits for approval by a different user (`compensation:approve`). |
| CO-C11 | Applying a change closes the open record the day before `effective_from` (it must be after the current start, else 409 `BACKDATED`) and inserts the new one. It also sets the payroll CTC from that date through payroll's port, keeping the worker's payroll structure. Without a payroll assignment, `payroll_synced = false` and nothing fails. |
| CO-C12 | `GET /workers/{employee_id}` returns the history, the current record, the compa-ratio and penetration, and the pending changes. `GET /me` returns the same for the caller (own data only). |
| CO-C13 | Changes: `pending → approved (applied) \| rejected \| cancelled`. Decisions are by a user other than the requester. |

### Merit cycles (CP-004..CP-007)

| ID | Requirement |
|---|---|
| CO-C20 | Create a draft `{name, effective_date, performance_cycle_id?, guidelines: [{rating: low\|medium\|high\|unrated, compa: below\|within\|above, min_pct, max_pct}]}`. The compa bands are: below < 0.90 ≤ within ≤ 1.10 < above. |
| CO-C21 | Budgets `PUT /merit-cycles/{id}/budgets [{department_id, amount}]` (draft only). |
| CO-C22 | Open (draft → open, `compensation:manage`). This snapshots one proposal per worker who has a current compensation record and a core assignment with a department on the effective date. Each proposal stores the current base, the compa-ratio, the final rating band from performance (CP-005: the snapshot is taken now; `unrated` when none) and the manager (the core assignment's manager). |
| CO-C23 | While open, the worker's manager (or `compensation:manage`) sets `{increase_pct 0.00–50.00, justification?}`. The amount is base × pct, rounded to the rupee. Outside the guideline range for its bands, a justification is required (400) and the proposal is flagged `outside_guideline`. |
| CO-C24 | Budget usage per department is live: Σ amounts vs budget. A proposal that would push its department over budget needs `override_budget = true` with a justification (409 `BUDGET_EXCEEDED` otherwise). |
| CO-C25 | Close for review (open → in_review, manage), approve (in_review → approved, `compensation:approve`, not the user who opened it), apply (approved → applied, manage). Applying writes one record per proposal with pct > 0, effective on the cycle date (reason `merit`, linked to the cycle), and syncs payroll. A second apply is 409 `CYCLE_STATE` and writes nothing. |
| CO-C26 | Visibility (CP-006): a manager sees only proposals where they are the manager, only while the cycle is open or in review. Employees see their own applied result through their history. |

## 4. API (24 operations under `/api/v1/hr/compensation`)

| Area | Operations |
|---|---|
| Pay ranges | `POST/GET /pay-ranges` |
| Changes | `POST/GET /changes`, `GET /changes/{id}`, `POST /changes/{id}/approve\|reject\|cancel` |
| Worker views | `GET /workers/{employee_id}`, `GET /me` |
| Merit cycles | `POST/GET /merit-cycles`, `GET/PATCH /merit-cycles/{id}`, `PUT/GET /merit-cycles/{id}/budgets`, `POST /merit-cycles/{id}/open\|close\|approve\|apply` |
| Proposals | `GET /merit-cycles/{id}/proposals` (manage/read: all; manager: their reports), `GET /merit-cycles/{id}/proposals/mine` (as manager), `PATCH /proposals/{id}` |
| Budget usage | `GET /merit-cycles/{id}/usage` |

## 5. Events and privacy

Events `hr.compensation.changed` and `hr.compensation.merit_applied` go on `jaas.compensation.events`. They carry
ids, the effective date and the reason only, never amounts, percentages or ratings (HR-009). Audit metadata is the
same.

## 6. Out of scope (v1)

Bonus plans, allowances as separate plans, currencies other than INR, multi-step approvals (M7).
