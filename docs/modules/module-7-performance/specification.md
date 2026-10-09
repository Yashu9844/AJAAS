# Module 7 — Specification: Performance Management & OKRs (FROZEN at P1 2026-10-09)

Scope per masterplan (D7-01): appraisal cycles, goals/OKRs with key results and cascading, continuous check-ins, self/manager/peer reviews against the cycle's criteria, and the 9-box talent matrix.
- **Depends on:**
  - Module 0: tenant, auth, RBAC + `PermissionFlag`, audit.
  - Module 1: reporting manager via the user's primary active mapping (D7-03).
  - Module 2: employee profiles and status.
- **Consumers (events):** Modules 10 and 11.

## 1. Glossary
- **Cycle** — an appraisal period with ordered status `draft → active → review → completed` and a fixed list of rating **criteria**.
- **Goal** — an objective owned by one employee in one cycle, with a weight (share of the employee's cycle) and progress 0–100. An optional **parent goal** gives cascading.
- **Key result (KR)** — a measurable target of a goal (start → target, current value). When a goal has KRs, its progress is derived from them.
- **Check-in** — an append-only progress update (who, when, before/after, note).
- **Review** — one reviewer's assessment of one employee in one cycle; type `self | manager | peer`.
- **Manager** — the `manager_user_id` on the employee's user's primary active Module 1 mapping.
- **Goal score** — the weighted mean of goal progress for an employee in a cycle.

## 2. Actors and permissions
- `performance:read` — view all cycles' goals, reviews and the 9-box.
- `performance:manage` — create/run cycles, manage anyone's goals, assign reviewers.
- **Self** (authenticated, with an employee profile): own goals, reviews assigned to me, my completed reviews.
- **Manager** (resolved at request time, D7-03): read and manage the goals of direct reports.
- Cycle list/get needs authentication only: every employee must see active cycles to set goals.

## 3. Functional requirements
### Cycles (FR-CY)
- FR-CY001 Create `{title 1–150, start_date, end_date ≥ start, criteria? (1–10 distinct names 1–50 chars; default Quality, Delivery, Collaboration, Ownership)}` → `draft`.
- FR-CY002 List (status filter) / get; update title, dates and criteria only in `draft`.
- FR-CY003 Activate `draft → active` (event `performance.cycle_started`).
- FR-CY004 Launch reviews `active → review`: creates self + manager reviews (PM-010).
- FR-CY005 Complete `review → completed`: pending reviews become `expired` (event `performance.cycle_completed`).
- FR-CY006 9-box for a cycle in `review` or `completed` (PM-015).
### Goals (FR-GL)
- FR-GL001 Create `{cycle_id, employee_id?, title 1–200, description?, weight 0.01–100, parent_goal_id?}`. `employee_id` defaults to the caller's own profile.
- FR-GL002 List by cycle/employee; my goals; get with key results and check-ins (newest 50).
- FR-GL003 Update title, description and weight (PM-003/PM-004).
- FR-GL004 Status: `active → completed | cancelled`, `completed → active`. Cancelled is final.
- FR-GL005 Add key result `{title 1–200, unit? ≤ 20, start_value, target_value ≠ start}`. Current value starts at `start_value`.
- FR-GL006 Check-in `{progress}` (goal without KRs) or `{key_result_id, value}` (goal with KRs), plus `note? ≤ 1000` → updates progress and appends a check-in (event `performance.goal_updated`).
### Reviews (FR-RV)
- FR-RV001 Assign a reviewer `{cycle_id, employee_id, reviewer_employee_id, type: peer|manager}` (manage, cycle in `review`).
- FR-RV002 List all (read), assigned to me, mine (subject, completed cycles only), get one.
- FR-RV003 Submit `{ratings: [{criterion, score 1–5}], strengths?, improvements?, potential? (low|medium|high)}` → `submitted` (event `performance.review_submitted`).
### Events (FR-EV)
- FR-EV001 Exchange `jaas.performance.events` (outbox in the same transaction, relay):
  - `performance.cycle_started`
  - `performance.goal_updated`
  - `performance.review_submitted`
  - `performance.cycle_completed`
- FR-EV002 Payloads carry ids, statuses and progress only — never ratings, scores, potential or review text.

## 4. Business rules (PM)
| ID | Rule |
|---|---|
| PM-001 | Criteria: 1–10 names, trimmed, 1–50 chars, unique ignoring case; the order is kept. Cycle edits only in `draft` (409 `CYCLE_STATE`). |
| PM-002 | Cycle transitions are only `draft→active→review→completed` (409 `CYCLE_STATE`). |
| PM-003 | Goals are created/edited/status-changed only while the cycle is `draft` or `active` (409 `CYCLE_STATE`). The owner must be a working employee (`active|probation|notice`; else 409 `EMPLOYEE_STATE`). |
| PM-004 | Weight is 0.01–100.00. The sum of an owner's non-cancelled goal weights in a cycle is ≤ 100.00 (409 `WEIGHT_EXCEEDED`), checked under a per-(cycle, owner) lock. |
| PM-005 | The parent goal must be in the same tenant and cycle, not cancelled, and the chain depth ≤ 3 (goal → parent → grandparent; 409 `GOAL_DEPTH`). The parent is fixed at creation. |
| PM-006 | KR progress = clamp((current − start) / (target − start) × 100, 0, 100), rounded half away from zero to 0.01. A goal with ≥ 1 KR has progress = mean of its KRs' progress (same rounding). Direct progress on such a goal → 409 `HAS_KEY_RESULTS`; a KR check-in on a goal without KRs is impossible (KR must belong to the goal, else 404). |
| PM-007 | Every progress change appends a check-in (from → to, author, note) in the same transaction. Check-ins are allowed only on `active` goals while the cycle is `active` or `review` (409 `GOAL_STATE` / `CYCLE_STATE`). KRs can be added only while the cycle is `draft` or `active`. |
| PM-008 | Access to a goal (read/write): owner, owner's manager, or `performance:manage` (write) / `performance:read` (read). Anyone else → 404. |
| PM-009 | Goal score = Σ(weight × progress) / Σ weight over non-cancelled goals (0 when none), rounded to 0.01. |
| PM-010 | Launch reviews: for each employee owning ≥ 1 non-cancelled goal in the cycle, create a `self` review (reviewer = employee) and, when a manager with an employee profile exists, a `manager` review. Idempotent: unique (cycle, employee, reviewer, type). The response reports counts, including employees without a manager. |
| PM-011 | Assign reviewer: cycle in `review`; reviewer ≠ subject; reviewer is a working employee; duplicates → 409 `CONFLICT`. |
| PM-012 | Submit: only the assigned reviewer (others 404); review `pending` (409 `REVIEW_STATE`); cycle `review` (409 `CYCLE_STATE`). Ratings must name exactly the cycle criteria, once each, with integer scores 1–5 (400). `potential` is required for `manager` reviews and forbidden otherwise (400). overall_score = mean of scores, rounded half away from zero to 0.01. Submitted reviews are immutable. |
| PM-013 | Completing a cycle marks its pending reviews `expired`. |
| PM-014 | A subject sees their own **submitted** reviews only after the cycle is `completed`. `reviewer_employee_id` is omitted for peer reviews (anonymity). |
| PM-015 | 9-box: per employee, performance band from the latest submitted manager review's overall_score (< 2.50 `low`, < 3.50 `medium`, else `high`) × that review's potential. Employees without a submitted manager review are listed as `unrated`. Each entry also carries the goal score (PM-009). |
| PM-016 | All ids are tenant-scoped; foreign ids → 404. |
| PM-017 | Ratings, scores, potential and review text never appear in events or audit metadata. |

## 5. API (22 operations, under `/api/v1/performance`)
| # | Method & path | Access |
|---|---|---|
| C1 | POST `/cycles` | manage |
| C2 | GET `/cycles?status&page&per_page` | authenticated |
| C3 | GET `/cycles/{id}` | authenticated |
| C4 | PATCH `/cycles/{id}` | manage |
| C5 | POST `/cycles/{id}/activate` | manage |
| C6 | POST `/cycles/{id}/launch-reviews` | manage |
| C7 | POST `/cycles/{id}/complete` | manage |
| C8 | GET `/cycles/{id}/nine-box` | read |
| G1 | POST `/goals` | self / manager / manage (PM-008) |
| G2 | GET `/goals?cycle_id&employee_id&status&page&per_page` | read: any filter. Without read: `employee_id` defaults to self and must be self or a direct report (else 404, D7-05) |
| G3 | GET `/goals/me?cycle_id&page&per_page` | self |
| G4 | GET `/goals/{id}` | PM-008 read |
| G5 | PATCH `/goals/{id}` | PM-008 write |
| G6 | POST `/goals/{id}/status` `{status: completed|cancelled|active}` | PM-008 write |
| G7 | POST `/goals/{id}/key-results` | PM-008 write |
| G8 | POST `/goals/{id}/check-ins` | PM-008 write |
| V1 | POST `/reviews` | manage |
| V2 | GET `/reviews?cycle_id&employee_id&status&page&per_page` | read |
| V3 | GET `/reviews/assigned?status&page&per_page` | self (reviewer) |
| V4 | GET `/reviews/me?cycle_id` | self (subject, PM-014) |
| V5 | GET `/reviews/{id}` | reviewer or read |
| V6 | POST `/reviews/{id}/submit` | reviewer |

The success envelope, pagination (page ≥ 1, per_page 1–100, default 20) and error envelope are the same as Modules 0–6. Decimals (weight, progress, values, scores) are JSON numbers with at most 2 decimals.

## 6. Errors
- 400: `VALIDATION_ERROR`.
- 401: unauthenticated.
- 403: `FORBIDDEN` (RBAC).
- 404: `NOT_FOUND`, `EMPLOYEE_NOT_FOUND`.
- 409: `CYCLE_STATE`, `GOAL_STATE`, `REVIEW_STATE`, `WEIGHT_EXCEEDED`, `GOAL_DEPTH`, `HAS_KEY_RESULTS`, `EMPLOYEE_STATE`, `CONFLICT`.
- 500: opaque.

## 7. Non-functional
- NFR-SEC001: tenant scoping on every query.
- NFR-SEC002: self endpoints resolve the employee from the JWT, never from input.
- NFR-SEC003: PM-017 privacy.
- NFR-D001: progress + check-in + outbox in one transaction.
- NFR-D002: exact decimal arithmetic (hundredths integers, no floats).
- NFR-P001: list endpoints p95 < 300 ms at 1k goals per tenant (indexed by cycle + employee).

## 8. Data (migrations 000051–000056)
- `performance_cycles` (criteria jsonb)
- `performance_goals` (index cycle+employee; parent FK)
- `performance_key_results`
- `performance_checkins` (append-only)
- `performance_reviews` (unique cycle+employee+reviewer+type)
- `performance_events_outbox`

## 9. Edge cases
- EC-01: Goal weights 60 + 40 OK; a third goal of 0.01 → 409. After cancelling one, it fits again.
- EC-02: KR with a decreasing target (start 50 → target 10, current 30) → 50.00%. A value beyond the target clamps to 100; below the start clamps to 0.
- EC-03: Direct progress on a goal with KRs → 409.
- EC-04: A colleague (not manager, no permission) reads a goal → 404.
- EC-05: Create a goal in a `review` cycle → 409 `CYCLE_STATE`.
- EC-06: Launch reviews twice → second call creates 0.
- EC-07: Submit with a missing or extra criterion → 400. A manager review without potential → 400.
- EC-08: Submit after the cycle completed → 409 (review expired).
- EC-09: Subject lists reviews before completion → empty. After completion, peer reviews have no reviewer id.
- EC-10: A cascading chain of depth 4 → 409 `GOAL_DEPTH`.
- EC-11: Cross-tenant ids → 404.

## 10. Out of scope (v1)
- Calibration and rating overrides.
- 360 review templates per role.
- Automatic peer nomination.
- Review reminders (Module 7 Notifications is unnumbered).
- Compensation linkage (Module 5 is manual).
- Goal alignment visualisation (frontend).
- Continuous feedback/kudos.
- Multi-level manager chains (direct manager only).
