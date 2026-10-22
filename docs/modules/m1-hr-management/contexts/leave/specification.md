# HR / Leave & Absence — Specification

> Part of **M1 HR Management** (bounded context `leave`). Formerly the stand-alone "module-4-leave" bundle; module numbers in older text were renamed to the canonical M0–M12 / HR-context names.

Scope per masterplan (D4-01): Leave & Absence Management. Supersedes the Projects scaffold (Projects is unassigned; see decisions D4-01).
Depends on: M0 Identity (tenant, auth, RBAC, audit, rate limit), HR/Employee (employee profile, status, gender, joining date), HR/Attendance (attendance day records — `on_leave` sync port).
Consumers: HR/Attendance (attendance status), HR/Payroll (LOP via events + ledger), M10 Notifications, M6 Reports & Analytics, M9 AI Assistant (events).

### 1. Purpose
Let employees apply for leave against configurable leave types, keep an auditable balance per employee/type/year, let approvers approve or reject, keep a holiday calendar for day counting, and reflect approved leave in attendance.

### 2. Glossary
- **Leave type** — tenant policy (EL, SL, CL, LOP…): allowance, accrual mode, carry-forward cap, rules.
- **Leave year** — calendar year (UTC date). D4-04.
- **Days** — decimal with 2 places, stored/computed as integer hundredths (`1.5` = 150). D4-03.
- **Ledger** — append-only `leave_ledger` rows (accrual, carry_forward, reserve, release, consume, reversal, adjustment). Source of truth for balances. D4-02.
- **Balance** — projection row per (employee, type, year): `opening + accrued + adjusted − used − reserved = available`.
- **Working day** — Monday–Friday that is not a non-optional holiday. D4-05.
- **Sandwich rule** — when enabled on a type, non-working days strictly between the first and last working day of a request count as leave.

### 3. Functional requirements

#### Leave types (FR-LT)
- FR-LT001 Create a type: name, code (2–10, `^[A-Z0-9_]+$`, uppercased, immutable), is_paid, annual_allowance (0–365), accrual (`annual`|`monthly`), carry_forward_limit (0–365), max_consecutive_days? (1–365), min_notice_days (0–90), allow_half_day, sandwich_rule, applicable_gender (`all`|`male`|`female`).
- FR-LT002 List / get types (any authenticated tenant member — employees must see what they can apply for).
- FR-LT003 Update any field except code; changes apply to future requests and future accruals only.
- FR-LT004 Deactivate: no new requests; existing requests/balances unaffected. Name and code unique per tenant among non-deleted.

#### Holidays (FR-HD)
- FR-HD001 Create holiday {date, name, is_optional}; one per tenant per date.
- FR-HD002 List holidays by year (authenticated). FR-HD003 Delete (soft) a holiday. Holiday changes never re-price existing requests (total_days is a snapshot, LV-012).

#### Balances & ledger (FR-BL)
- FR-BL001 A balance row is materialized lazily on first touch (apply, read, adjust) for (employee, type, current year) — no scheduler (D4-06).
- FR-BL002 Accrual on materialization and on every later touch: `annual` → full allowance prorated by joining date; `monthly` → allowance/12 per month from the later of Jan and joining month through the current month, December absorbs the rounding remainder. Each increase writes an `accrual` ledger row and a `leave.accrued` event.
- FR-BL003 Carry-forward at materialization: `min(max(previous year available, 0), carry_forward_limit)` as one `carry_forward` row (opening).
- FR-BL004 HR adjustment: signed non-zero days with reason → `adjustment` row.
- FR-BL005 Self balance view; HR view by employee; ledger view (paginated).

#### Requests (FR-LR)
- FR-LR001 Preview (quote): compute total_days and balance impact without saving.
- FR-LR002 Apply: validates LV rules, reserves days (`reserve` row), status `pending`, event `leave.applied`.
- FR-LR003 Approve (`leave:approve`): reserved → used (`release` + `consume` rows), status `approved`, attendance days marked `on_leave` (HR/Attendance port, same tx), event `leave.approved`.
- FR-LR004 Reject (`leave:approve`, comment required): reservation released, event `leave.rejected`.
- FR-LR005 Cancel (owner): pending → release; approved and start date in the future → `reversal` row, attendance leave marks cleared, event `leave.cancelled`. Approved leave that has started cannot be cancelled by the owner (LV-011).
- FR-LR006 Lists: own requests; all requests (filters status, employee_id, from, to); detail.

#### Events (FR-EV)
- FR-EV001 Exchange `jaas.leave.events`, routing keys `leave.applied|approved|rejected|cancelled|accrued`; envelope identical to HR/Attendance (event_id, event_type, routing_key, version, occurred_at, producer=`leave-service`, tenant_id, correlation_id, payload).
- FR-EV002 Written to `leave_events_outbox` inside the business transaction; published after commit; relay retries unpublished rows.
- FR-EV003 Payloads carry ids, dates, days, status only — never the free-text reason or review comment.

### 4. Business rules (LV)

| ID | Rule |
|---|---|
| LV-001 | Caller must have an employee profile (404 `EMPLOYEE_NOT_FOUND`) in a working status `active`/`probation`/`notice` (403 `EMPLOYEE_NOT_ACTIVE`). |
| LV-002 | Type must be active (409 `LEAVE_TYPE_INACTIVE`) and match `applicable_gender` (case-insensitive vs HR/Employee gender; 403 `LEAVE_TYPE_NOT_APPLICABLE`). |
| LV-003 | `start_date ≤ end_date`, both in the current leave year (400 `INVALID_LEAVE_RANGE`). |
| LV-004 | Half day only when `allow_half_day` and `start_date == end_date`; `half_day` ∈ `first_half`/`second_half` (400 `VALIDATION_ERROR`). |
| LV-005 | Notice: if `min_notice_days > 0`, `start_date ≥ today + min_notice_days`; if `min_notice_days == 0`, `start_date ≥ today − 30` (backdating window). Else 400 `LEAVE_NOTICE`. |
| LV-006 | total_days = working days in range (D4-05); with sandwich rule, non-working days strictly between first and last working day are added; half day = 0.50. 0 days → 400 `NO_WORKING_DAYS`. |
| LV-007 | `max_consecutive_days` (if set) ≥ total_days, else 400 `LEAVE_TOO_LONG`. |
| LV-008 | No overlap with the employee's own `pending`/`approved` requests (409 `LEAVE_OVERLAP`). Two half days on the same date with different halves do not overlap. |
| LV-009 | Paid types: `available ≥ total_days` at apply time, else 409 `INSUFFICIENT_BALANCE`. Unpaid types (`is_paid = false`) skip the balance check; days still recorded in `used` on approval (LOP for payroll). |
| LV-010 | Approver ≠ requester (403 `SELF_APPROVAL_FORBIDDEN`); only `pending` can be approved/rejected (409 `LEAVE_NOT_PENDING`). Reject requires a comment. |
| LV-011 | Owner cancel: `pending` any time; `approved` only when `start_date > today` (else 409 `LEAVE_ALREADY_STARTED`); other statuses 409 `LEAVE_NOT_CANCELLABLE`. |
| LV-012 | `total_days` and type rules are snapshotted at apply; later type/holiday edits never change existing requests. |
| LV-013 | Every balance change writes ledger rows in the same transaction; invariant: balance columns == sums of ledger rows per kind. |
| LV-014 | Apply/approve/reject/cancel/adjust take a per-employee advisory xact lock (`leave:<employee_id>`) so balances cannot race. |
| LV-015 | Approve marks each full-day working date `on_leave` in HR/Attendance (creating the day record with source `leave` if missing). Half-day leave does not change attendance status. Cancel of approved leave clears those marks (leave-only records are removed; records with punches are recomputed). |
| LV-016 | Leave days, balances and ledger rows are never hard-deleted; cancellation is a status + reversal row. |
| LV-017 | Adjustment days: non-zero, |days| ≤ 365, reason 1–500 chars; may make available negative only via adjustment (HR correction). |

### 5. REST API (base `/api/v1`, JWT + tenant subdomain; envelope `{data, meta}` / `{error:{code,message,details}}`)

| # | Method & path | Permission | Request | Success |
|---|---|---|---|---|
| L1 | POST `/leave/types` | `leave:manage` | `CreateLeaveTypeRequest` | 201 `LeaveTypeResponse` |
| L2 | GET `/leave/types?status&page&per_page` | authenticated | — | 200 list + meta |
| L3 | GET `/leave/types/{id}` | authenticated | — | 200 `LeaveTypeResponse` |
| L4 | PATCH `/leave/types/{id}` | `leave:manage` | `UpdateLeaveTypeRequest` (pointers; no code) | 200 `LeaveTypeResponse` |
| L5 | POST `/leave/types/{id}/deactivate` | `leave:manage` | — | 200 `LeaveTypeResponse` |
| H1 | POST `/leave/holidays` | `leave:manage` | `CreateHolidayRequest{date, name, is_optional?}` | 201 `HolidayResponse` |
| H2 | GET `/leave/holidays?year` | authenticated | default current year | 200 `[]HolidayResponse` |
| H3 | DELETE `/leave/holidays/{id}` | `leave:manage` | — | 200 `HolidayResponse` |
| B1 | GET `/leave/balances/me` | self | — | 200 `[]BalanceResponse` (all active types, current year) |
| B2 | GET `/leave/balances?employee_id` | `leave:read` | `employee_id` required | 200 `[]BalanceResponse` |
| B3 | POST `/leave/balances/adjust` | `leave:manage` | `AdjustBalanceRequest{employee_id, leave_type_id, days, reason}` | 200 `BalanceResponse` |
| B4 | GET `/leave/ledger?employee_id&leave_type_id&page&per_page` | `leave:read` | `employee_id` required | 200 `[]LedgerEntryResponse` + meta |
| R1 | POST `/leave/requests/preview` | self | `ApplyLeaveRequest` | 200 `PreviewResponse{total_days, available, available_after, working_dates[]}` |
| R2 | POST `/leave/requests` | self | `ApplyLeaveRequest{leave_type_id, start_date, end_date, half_day?, reason}` | 201 `LeaveRequestResponse` |
| R3 | GET `/leave/requests/me?status&page&per_page` | self | — | 200 list + meta |
| R4 | POST `/leave/requests/{id}/cancel` | self (owner) | — | 200 `LeaveRequestResponse` |
| R5 | GET `/leave/requests?status&employee_id&from&to&page&per_page` | `leave:read` | — | 200 list + meta |
| R6 | GET `/leave/requests/{id}` | `leave:read` | — | 200 `LeaveRequestResponse` |
| R7 | POST `/leave/requests/{id}/approve` | `leave:approve` | `ReviewRequest{comment?}` | 200 `LeaveRequestResponse` |
| R8 | POST `/leave/requests/{id}/reject` | `leave:approve` | `ReviewRequest{comment}` | 200 `LeaveRequestResponse` |

20 operations. Pagination identical to HR/Attendance (`page` ≥ 1 default 1, `per_page` 1–100 default 20, clamped, never 500).

#### DTO shapes (JSON snake_case; days as JSON numbers with ≤ 2 decimals)
- `LeaveTypeResponse{id, name, code, is_paid, annual_allowance, accrual, carry_forward_limit, max_consecutive_days?, min_notice_days, allow_half_day, sandwich_rule, applicable_gender, status, created_at, updated_at}`
- `HolidayResponse{id, date, name, is_optional, created_at}`
- `BalanceResponse{leave_type_id, leave_type_code, leave_type_name, year, opening, accrued, adjusted, used, reserved, available, is_paid}`
- `LedgerEntryResponse{id, leave_type_id, year, kind, days, leave_request_id?, note?, actor_user_id?, created_at}`
- `LeaveRequestResponse{id, employee_id, leave_type_id, leave_type_code, start_date, end_date, half_day?, total_days, reason, status, reviewer_user_id?, reviewed_at?, review_comment?, cancelled_at?, created_at, updated_at}`

### 6. Error behavior

| Status | Code | When |
|---|---|---|
| 400 | VALIDATION_ERROR | body/fields/dates/UUIDs; details[] per field |
| 400 | INVALID_LEAVE_RANGE, LEAVE_NOTICE, NO_WORKING_DAYS, LEAVE_TOO_LONG | LV-003/005/006/007 |
| 401 | UNAUTHORIZED | M0 Identity |
| 403 | FORBIDDEN, EMPLOYEE_NOT_ACTIVE, LEAVE_TYPE_NOT_APPLICABLE, SELF_APPROVAL_FORBIDDEN | RBAC / LV-001 / LV-002 / LV-010 |
| 404 | NOT_FOUND, EMPLOYEE_NOT_FOUND | foreign or unknown ids (cross-tenant → 404) / no profile |
| 409 | CONFLICT, LEAVE_TYPE_INACTIVE, LEAVE_OVERLAP, INSUFFICIENT_BALANCE, LEAVE_NOT_PENDING, LEAVE_ALREADY_STARTED, LEAVE_NOT_CANCELLABLE | duplicates / LV rules |
| 500 | INTERNAL_ERROR | unexpected (opaque) |

### 7. Non-functional

| ID | Target |
|---|---|
| NFR-P001 | Apply/approve p95 < 200 ms for ranges ≤ 31 days (one tx: lock + ≤ 8 queries; attendance sync batched per request) |
| NFR-P002 | Lists p95 < 500 ms at 100 items |
| NFR-SEC001 | Every query tenant-scoped; foreign ids → 404 |
| NFR-SEC002 | Self endpoints derive the employee from the JWT user only |
| NFR-D001 | Request + balance + ledger + attendance + outbox writes in one transaction |
| NFR-D002 | Day arithmetic in integer hundredths; no floating-point in balances |

### 8. Data (migrations 000032–000037; UUID PK; timestamptz; days NUMERIC(7,2))

- `leave_types` — tenant_id, name, code, is_paid, annual_allowance, accrual, carry_forward_limit, max_consecutive_days?, min_notice_days, allow_half_day, sandwich_rule, applicable_gender, status, created_at, updated_at, deleted_at. Unique (tenant_id, lower(name)) and (tenant_id, code) WHERE deleted_at IS NULL.
- `leave_holidays` — tenant_id, holiday_date date, name, is_optional, created_at, updated_at, deleted_at. Unique (tenant_id, holiday_date) WHERE deleted_at IS NULL.
- `leave_balances` — tenant_id, employee_profile_id, leave_type_id, year int, opening, accrued, adjusted, used, reserved, created_at, updated_at. Unique (tenant_id, employee_profile_id, leave_type_id, year). (No generated column: available computed in code — D4-02.)
- `leave_requests` — tenant_id, employee_profile_id, leave_type_id, start_date, end_date, half_day?, total_days, reason, status, requested_by_user_id, reviewer_user_id?, reviewed_at?, review_comment?, cancelled_at?, created_at, updated_at. Index (tenant_id, employee_profile_id, start_date), (tenant_id, status).
- `leave_ledger` — tenant_id, employee_profile_id, leave_type_id, year, kind, days (signed), leave_request_id?, actor_user_id?, note?, created_at. Index (tenant_id, employee_profile_id, leave_type_id, year). Append-only.
- `leave_events_outbox` — same shape as `attendance_events_outbox`.

HR/Attendance change (cross-module, D4-08): attendance record `source` gains value `leave`; new exported port `attendance/services.LeaveSync` (MarkLeave / ClearLeave taking the caller's tx).

### 9. Edge cases
EC-01 apply with no profile → 404. EC-02 terminated employee → 403. EC-03 range Fri–Mon, no sandwich → 2 days; with sandwich → 4. EC-04 range Sat–Sun only → 400 NO_WORKING_DAYS. EC-05 holiday inside range → excluded (sandwich counts it if between working days). EC-06 optional holiday → counts as working day. EC-07 balance 2, apply 3 paid → 409; unpaid → allowed. EC-08 overlapping pending request → 409; first_half + second_half same date → allowed. EC-09 self-approve → 403, stays pending. EC-10 approve twice → 409. EC-11 cancel approved future leave → balance restored, attendance marks removed. EC-12 cancel approved leave already started → 409. EC-13 monthly accrual 18/yr in March, joined before Jan → accrued 4.50. EC-14 annual 12/yr, joined 1 Jul → accrued 6.00. EC-15 carry-forward limit 5, previous available 8 → opening 5; previous −2 → opening 0. EC-16 concurrent applies draining the same balance → exactly the affordable ones succeed (LV-014). EC-17 cross-tenant ids → 404. EC-18 per_page=0 → clamped 200. EC-19 type deactivated after apply → approve still works. EC-20 employee punches on an approved leave day → attendance recompute sets present (attendance wins; documented).

### 10. Out of scope (explicit)
Multi-tier / manager-routed approval chains and team inbox (M7 Approvals; v1 = any holder of `leave:approve`). Proof attachments (needs M8 Documents file storage). Tenant-configurable weekly-off pattern and per-location holiday calendars (v1.1; v1 = Sat/Sun + one tenant calendar). Encashment, comp-off, leave year ≠ calendar year, cross-year requests (split by employee). Payroll LOP deduction (HR/Payroll consumes `leave.approved` + ledger). Notifications (M10 Notifications).

## Assumptions

- A4-01 HR/Employee `gender` is free text; applicability compares lower-case `male`/`female`; empty/other gender matches only `all`.
- A4-02 HR/Employee `employment.joining_date` drives proration; missing joining date → treated as before the leave year (full entitlement).
- A4-03 "Today" for notice, backdating and accrual = UTC date (matches HR/Attendance default timezone).
- A4-04 tenant_admin bypasses RBAC (M0 Identity behavior) and is therefore an approver.

## Parity (2026-10-22): manager routing, location calendars, leave year, comp-off, encashment

Package `internal/hr/leave/plus`, routes under `/api/v1/hr/leave`. Permissions are the existing
`leave:read | manage | approve`; manager and worker endpoints need only a login with an employee profile.

| ID | Requirement |
|---|---|
| LP-01 | Policy: `PUT/GET /policy` (`manage` / `read`): `year_start_month` (1–12, default 1), `comp_off_leave_type_id` (an active leave type, optional), `comp_off_expiry_days` (1–365, default 90). Changing `year_start_month` after balances exist is refused (409 `YEAR_LOCKED`) unless no balance row exists yet. |
| LP-02 | Leave year: the leave year is labelled by the calendar year in which it starts. With `year_start_month = 4`, 2026-05-10 and 2027-02-01 are both in leave year 2026. LV-003 (range in the current leave year), balance rows, accrual and carry-forward all use the leave year. Accrual credits months from the later of the year start and the joining month through the year end (annual) or through the current month (monthly); month 12 of the year still lands exactly on the allowance. |
| LP-03 | Location holidays: `POST/GET /location-holidays`, `DELETE /location-holidays/{id}` (`manage` / `read`): a date, name and optional flag for one organization location. Unique per location and date. An employee's location is their core assignment on the date; employees without one use the tenant calendar only. |
| LP-04 | Week-off patterns: `PUT /week-offs/{location_id}` (`manage`) sets the weekly-off weekdays (0 = Sunday … 6; empty list allowed) for a location, `GET /week-offs` (`read`) lists them, `DELETE /week-offs/{location_id}` removes it. Without a pattern the location (and every employee without one) uses Saturday and Sunday. |
| LP-05 | Pricing (LV-006) and attendance marking use the employee's calendar: tenant holidays ∪ their location's non-optional holidays as blocked dates, and the location's week-off weekdays instead of Saturday/Sunday. Existing requests keep their snapshotted `total_days` (LV-012). |
| LP-06 | Manager routing: `GET /inbox` lists pending requests of the caller's direct reports (organization mappings); `POST /inbox/{id}/approve|reject` decide them through LV-010. A request of anyone else is 404; tenant-scope `leave:approve` holders pass `scope=all` to see every pending request. |
| LP-07 | Team calendar: `GET /team-calendar?from=&to=` (range ≤ 62 days) returns, for the caller's direct reports (all workers for `leave:read`), their pending and approved leave overlapping the range plus the dates blocked for the tenant. Rejected and cancelled leave is never shown; reasons are never shown. |
| LP-08 | Comp-off request: `POST /comp-off {worked_date, days, reason}` (days 0.5 or 1; the worked date within the last 30 days, a holiday or week-off for the worker, with attendance work minutes > 0; one non-cancelled request per worker and date; needs `comp_off_leave_type_id` in the policy, else 409 `COMP_OFF_NOT_CONFIGURED`). Errors: 409 `NOT_A_NON_WORKING_DAY`, 409 `NO_WORK_RECORDED`, 409 `CONFLICT`. |
| LP-09 | Decision: `POST /comp-off/{id}/approve|reject|cancel`. The worker's manager or a `leave:approve` tenant holder decides; never the requester (403 `SELF_APPROVAL_FORBIDDEN`); the requester cancels their own pending request. Approval credits the comp-off leave type through a balance adjustment (ledger row, note "comp-off") in the same transaction and sets `expires_on = worked_date + comp_off_expiry_days`. |
| LP-10 | Expiry: `POST /comp-off/expire` (`manage`) and a daily sweep debit each approved, unexpired-flag credit whose `expires_on` has passed by `min(credit days, balance available)` (never below zero) and mark it expired with the debited days. Idempotent. |
| LP-11 | Lists: `GET /comp-off/me`, `GET /comp-off` (`read`, filters status/employee_id). |
| LP-12 | Encashment rules: `PUT /encashment-rules/{leave_type_id}` (`manage`) sets `max_days_per_year` (0–365) and `min_balance_after` (≥ 0), `GET /encashment-rules` (`read`) lists them. A type without a rule is not encashable (409 `NOT_ENCASHABLE`). |
| LP-13 | Encashment request: `POST /encashments {leave_type_id, days, reason}` (days a multiple of 0.5 > 0). Rules: type active and encashable; `days` ≤ remaining annual allowance (max − approved − pending in the leave year); the balance available now, minus pending encashments, minus `days` must stay ≥ `min_balance_after` (409 `INSUFFICIENT_BALANCE`); at most one pending request per worker and type (409 `CONFLICT`). |
| LP-14 | Decision: `POST /encashments/{id}/approve|reject|cancel`. The worker's manager or a `leave:approve` tenant holder decides; never the requester; the requester cancels own pending. Approval re-checks LP-13 and debits the balance with an adjustment (ledger note "encashment"); the request keeps `paid_out_at = null` until payroll consumes it (payroll parity). |
| LP-15 | Lists: `GET /encashments/me`, `GET /encashments` (`read`, filters status/employee_id/leave_type_id). |
| LP-16 | Every write takes the per-employee leave lock (LV-014) so balances cannot race with applications. |
| LP-17 | Events `leave.comp_off_credited`, `leave.encashment_approved` carry ids and day counts only; audit metadata never includes reasons. |
| LP-18 | Failure semantics: a failed balance adjustment rolls the decision back; nothing is half-applied. |

Errors added: `YEAR_LOCKED`, `COMP_OFF_NOT_CONFIGURED`, `NOT_A_NON_WORKING_DAY`, `NO_WORK_RECORDED`, `NOT_ENCASHABLE`.

**Golden tests:** `LG1` a non-calendar leave year labels dates and accrues exactly; `LG2` a location holiday and week-off
pattern change pricing for that location only; `LG3` the inbox shows only reports and strangers cannot decide;
`LG4` comp-off needs real work on a non-working day, credits once and expires without going negative; `LG5` encashment
respects the annual cap and minimum balance and debits exactly once.
