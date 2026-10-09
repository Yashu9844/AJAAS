# Module 3 — Specification (FROZEN at P1 2026-10-08 — contract for P2+)

Status: FROZEN 2026-10-08 (plan.md P1). Implementation must match exactly. Changes need decisions.md entry + plan.md gate-log row.
Scope source: docs/architecture/SYSTEM_ARCHITECTURE_MASTERPLAN.md §Module 3 (Attendance, Shifts & Time Tracking), adapted to the real Module 0/1/2 contracts (see decisions.md D3-01..D3-12). Leave is Module 4.

## 1. Purpose

Record when employees work: shift definitions, shift assignments, punch IN/OUT, daily attendance records with computed totals (work, break, late, overtime, status), timesheet regularization requests with approval, and a daily presence summary for managers. Module 3 owns time facts only — leave balances (Module 4) and pay (Module 5) consume them.

## 2. Glossary

- **Employee** — a Module 2 `employee_profiles` row (`employee_profile_id`). Every attendance fact is keyed by employee, never by user.
- **Session** — one IN punch and its matching OUT punch.
- **Open session** — an IN punch with no OUT yet.
- **Attendance date** — the calendar day a record belongs to, in the shift time zone (AT-005).
- **Expected minutes** — shift span − break (AT-008). No shift → 480.

## 3. Functional requirements

### Shifts (FR-SH)
| ID | Requirement |
|---|---|
| FR-SH001 | Create a shift: `name` (1–100), `code` (optional, `^[a-z0-9-]{2,32}$`, unique per tenant), `start_time` / `end_time` (`HH:MM`, 24h), `grace_period_mins` (0–240, default 15), `break_duration_mins` (0–480, default 60), `full_day_minutes` / `half_day_minutes` (optional, 1–1440; defaults AT-010), `timezone` (IANA, default `UTC`). |
| FR-SH002 | Shift names unique per tenant (case-insensitive). Codes unique per tenant when set. |
| FR-SH003 | `is_night_shift` is derived, never client-set: true when `end_time <= start_time` (span crosses midnight). `start_time == end_time` is rejected. |
| FR-SH004 | Get / list shifts (paginated, `status` filter, ordered by name). |
| FR-SH005 | Update name, times, grace, break, thresholds, timezone. `code` is immutable once set (ignored, not error). Updates affect only records computed afterwards; existing records are not recomputed (AT-021). |
| FR-SH006 | Deactivate a shift (`status=inactive`). Blocked with 409 while any assignment covering today or a future date references it. |

### Shift assignments (FR-SA)
| ID | Requirement |
|---|---|
| FR-SA001 | Assign an employee to a shift from `effective_from` (date) to optional `effective_to` (date, inclusive, ≥ from). Shift must be active; employee must exist in tenant. |
| FR-SA002 | Assignments for one employee never overlap. If an open-ended assignment starts before the new `effective_from`, it is closed at `effective_from − 1 day` in the same transaction; any other overlap → 409 `ASSIGNMENT_OVERLAP`. |
| FR-SA003 | List assignments by employee (newest first). The active assignment for a date is the one whose range covers it. |

### Punches & records (FR-PU / FR-AR)
| ID | Requirement |
|---|---|
| FR-PU001 | Self-service punch `POST /attendance/punch` with `type` (`in`/`out`), optional `latitude` (−90..90), `longitude` (−180..180), `device_id` (≤100), `source` (`web` default, `mobile`). Punch time is the server clock (AT-001). |
| FR-PU002 | Punch rules AT-002..AT-006. The response returns the punch and the recomputed record. |
| FR-AR001 | One record per (tenant, employee, attendance date). Created by the first IN of that date or by an approved regularization. |
| FR-AR002 | Totals recomputed on every punch and regularization: `first_punch_in`, `last_punch_out`, `total_work_minutes`, `total_break_minutes`, `late_minutes`, `overtime_minutes`, `status` (AT-006..AT-010). |
| FR-AR003 | Self views: today's state (`/attendance/me/today`: record or null, open-session flag, active shift) and own records by date range (`/attendance/me`, max 62 days). |
| FR-AR004 | Admin views (`attendance:read`): records filtered by employee / date range / status, paginated; record detail with its punches. |
| FR-AR005 | Daily summary (`attendance:read`) for a date: `total_employees`, `present`, `late`, `half_day`, `on_leave`, `absent` (AT-020). |

### Regularizations (FR-RG)
| ID | Requirement |
|---|---|
| FR-RG001 | Employee requests a correction for a date: `attendance_date`, `requested_punch_in`, `requested_punch_out` (RFC3339), `reason` (1–500). Rules AT-014, AT-015. |
| FR-RG002 | Employee lists own requests (filter `status`) and cancels own pending requests (AT-018). |
| FR-RG003 | Approvers (`attendance:approve`) list all requests (filter `status`, `employee_id`), approve (optional `comment`) or reject (required `comment`). Rules AT-016, AT-017. |

### Events (FR-EV)
| ID | Requirement |
|---|---|
| FR-EV001 | Produce on exchange `jaas.attendance.events` (topic, durable), routing `attendance.{resource}.{action}`, standard envelope (event_id, event_type, routing_key, version `1.0`, occurred_at, producer `attendance-service`, tenant_id, correlation_id, payload): `attendance.punch.in`, `attendance.punch.out`, `attendance.regularization.requested`, `attendance.regularization.approved`, `attendance.regularization.rejected`. |
| FR-EV002 | Transactional outbox: the event row is written to `attendance_events_outbox` in the same DB transaction as the state change; published after commit; a relay retries unpublished rows (30s tick, max 10 attempts). Queue down never blocks the API. |
| FR-EV003 | Payloads carry IDs, dates, times and computed totals only — never IP, device id or coordinates. |

## 4. Business rules (AT)

| Rule | Description |
|---|---|
| AT-001 | Punch time = server UTC clock. No client time field exists in the contract. |
| AT-002 | Caller must have an employee profile in the tenant (Module 2 `GetByUserID`) → else 404 `EMPLOYEE_NOT_FOUND`. Profile status must be `active`, `probation` or `notice` → else 403 `EMPLOYEE_NOT_ACTIVE`. |
| AT-003 | Sessions alternate: `in` while a session is open → 409 `ALREADY_PUNCHED_IN`; `out` with no open session → 409 `NOT_PUNCHED_IN`. |
| AT-004 | A punch within 60 s of the employee's previous punch → 409 `DUPLICATE_PUNCH`. Evaluated before AT-003 (D3-13). |
| AT-005 | Attendance date for `in` = local date (shift timezone, else UTC) of the punch; for a night shift, an `in` whose local time-of-day is before `end_time + 4h` belongs to the previous date. `out` always attaches to the record holding the open session. |
| AT-006 | An open session older than 20 h is abandoned: `out` → 409 `SESSION_EXPIRED` (regularize instead); a new `in` is allowed and starts a new session. Totals ignore abandoned sessions. Work = Σ(out − in) of closed sessions; break = Σ gaps between an `out` and the next `in` on the same record. Superseded punches (AT-017) are ignored. |
| AT-007 | `late_minutes` = max(0, first_in − (shift start + grace)) in the shift timezone on the attendance date; 0 when no shift is assigned. |
| AT-008 | Expected minutes = shift span − break, span = end − start (+1440 when night shift). No shift → 480. |
| AT-009 | `overtime_minutes` = max(0, work − expected). |
| AT-010 | Status: open session → `present`. Otherwise work ≥ full-day threshold → `present`; work ≥ half-day threshold → `half_day`; else `absent`. Defaults: full = expected, half = expected / 2 (shift fields override). Statuses `on_leave`, `holiday`, `week_off` are reserved for Module 4 / calendar writers and are never set by punches. |
| AT-011 | Assignment ranges never overlap per employee (FR-SA002). Dates are calendar dates (no time). |
| AT-012 | Shift deactivation blocked while referenced by an assignment covering today or later (FR-SH006). |
| AT-013 | `HH:MM` 00:00–23:59; start ≠ end; timezone must load via IANA database; thresholds: half ≤ full when both set. |
| AT-014 | Regularization: `attendance_date` ≤ today (requester's shift tz) and ≥ today − 30 days → else 400 `REGULARIZATION_WINDOW`; `requested_punch_out` > `requested_punch_in`; span ≤ 20 h; the local date of `requested_punch_in` must equal `attendance_date` (night shifts: or the day after attendance_date for a punch before end+4h — same AT-005 attribution). |
| AT-015 | At most one `pending` regularization per employee per date → 409 `REGULARIZATION_PENDING`. |
| AT-016 | Approve/reject only `pending` → else 409 `REGULARIZATION_NOT_PENDING`. A reviewer whose own employee profile is the requester → 403 `SELF_APPROVAL_FORBIDDEN`. Reject requires `comment` (1–500). |
| AT-017 | Approval (one transaction): lock employee; create the record if missing; mark all its punches `is_superseded=true`; insert `in`/`out` punches with source `regularization`; recompute totals; set `is_regularized=true`; set request `approved`, `reviewer_user_id`, `reviewed_at`; write outbox event. |
| AT-018 | Cancel: requester's own `pending` request only → status `cancelled`. |
| AT-019 | Records and punches are never deleted (append-only; corrections supersede). |
| AT-020 | Summary for date D: `total_employees` = Module 2 employees with status active/probation/notice; `present` = records on D with status `present` or open session; `half_day`; `late` = records with late_minutes > 0; `on_leave` = records with status `on_leave`; `absent` = max(0, total − present − half_day − on_leave). |
| AT-021 | Shift/assignment changes never rewrite existing records; recomputation happens only on that record's next punch or approved regularization. |
| AT-022 | Per-employee writes (punch, approval) serialize through a transaction-scoped advisory lock keyed on (tenant_id, employee_profile_id). |

## 5. REST API (base `/api/v1`, all JWT + tenant subdomain; envelope `{data, meta}` / `{error:{code,message,details}}`)

| # | Method & path | Permission | Request | Success |
|---|---|---|---|---|
| A1 | POST `/attendance/punch` | self (employee) | `PunchRequest{type, latitude?, longitude?, device_id?, source?}` | 201 `PunchResult{punch, record}` |
| A2 | GET `/attendance/me/today` | self | — | 200 `TodayResponse{attendance_date, record?, open_session, shift?}` |
| A3 | GET `/attendance/me?from&to` | self | dates `YYYY-MM-DD`, default last 7 days, max 62 | 200 `[]RecordResponse` |
| A4 | POST `/attendance/regularizations` | self | `CreateRegularizationRequest{attendance_date, requested_punch_in, requested_punch_out, reason}` | 201 `RegularizationResponse` |
| A5 | GET `/attendance/regularizations/me?status&page&per_page` | self | — | 200 list + meta |
| A6 | POST `/attendance/regularizations/{id}/cancel` | self (owner) | — | 200 `RegularizationResponse` |
| A7 | GET `/attendance/records?employee_id&from&to&status&page&per_page` | `attendance:read` | — | 200 list + meta |
| A8 | GET `/attendance/records/{id}` | `attendance:read` | — | 200 `RecordDetailResponse{record, punches[]}` |
| A9 | GET `/attendance/summary?date` | `attendance:read` | default today UTC | 200 `SummaryResponse` |
| A10 | GET `/attendance/regularizations?status&employee_id&page&per_page` | `attendance:read` | — | 200 list + meta |
| A11 | POST `/attendance/regularizations/{id}/approve` | `attendance:approve` | `ReviewRequest{comment?}` | 200 `RegularizationResponse` |
| A12 | POST `/attendance/regularizations/{id}/reject` | `attendance:approve` | `ReviewRequest{comment}` | 200 `RegularizationResponse` |
| S1 | POST `/shifts` | `attendance:manage` | `CreateShiftRequest` | 201 `ShiftResponse` |
| S2 | GET `/shifts?status&page&per_page` | `attendance:read` | — | 200 list + meta |
| S3 | GET `/shifts/{id}` | `attendance:read` | — | 200 `ShiftResponse` |
| S4 | PATCH `/shifts/{id}` | `attendance:manage` | `UpdateShiftRequest` (pointer fields) | 200 `ShiftResponse` |
| S5 | POST `/shifts/{id}/deactivate` | `attendance:manage` | — | 200 `ShiftResponse` |
| S6 | POST `/shifts/{id}/assignments` | `attendance:manage` | `AssignShiftRequest{employee_id, effective_from, effective_to?}` | 201 `AssignmentResponse` |
| S7 | GET `/shift-assignments?employee_id&page&per_page` | `attendance:read` | `employee_id` required | 200 list + meta |

Pagination: `page` ≥ 1 (default 1), `per_page` 1–100 (default 20); invalid or out-of-range values are clamped, never a 500. Meta: `{page, per_page, total_items, total_pages}`.

### DTO shapes (JSON snake_case)
- `ShiftResponse{id, name, code?, start_time, end_time, grace_period_mins, break_duration_mins, full_day_minutes?, half_day_minutes?, timezone, is_night_shift, expected_minutes, status, created_at, updated_at}`
- `AssignmentResponse{id, employee_id, shift_id, effective_from, effective_to?, created_at}`
- `PunchResponse{id, punch_type, punch_time, source, latitude?, longitude?, device_id?, is_superseded}` (no IP)
- `RecordResponse{id, employee_id, attendance_date, shift_id?, first_punch_in?, last_punch_out?, total_work_minutes, total_break_minutes, late_minutes, overtime_minutes, status, source, is_regularized, updated_at}`
- `RegularizationResponse{id, employee_id, attendance_date, requested_punch_in, requested_punch_out, reason, status, reviewer_user_id?, reviewed_at?, review_comment?, created_at}`
- `SummaryResponse{date, total_employees, present, late, half_day, on_leave, absent}`

## 6. Error behavior

| Status | Code | When |
|---|---|---|
| 400 | VALIDATION_ERROR | bad body/fields/dates/UUIDs/time format/timezone; details[] lists fields |
| 400 | REGULARIZATION_WINDOW | AT-014 date window |
| 401 | UNAUTHORIZED | missing/invalid JWT (Module 0) |
| 403 | FORBIDDEN | missing `attendance:*` permission (Module 0 RBAC) |
| 403 | EMPLOYEE_NOT_ACTIVE / SELF_APPROVAL_FORBIDDEN | AT-002 / AT-016 |
| 404 | NOT_FOUND / EMPLOYEE_NOT_FOUND | id not in tenant (cross-tenant → 404, never 403) / caller has no profile |
| 409 | CONFLICT, ALREADY_PUNCHED_IN, NOT_PUNCHED_IN, DUPLICATE_PUNCH, SESSION_EXPIRED, ASSIGNMENT_OVERLAP, REGULARIZATION_PENDING, REGULARIZATION_NOT_PENDING | rules above; duplicate shift name/code; shift in use |
| 500 | INTERNAL_ERROR | unexpected (no internals leaked) |

## 7. Non-functional

| ID | Target |
|---|---|
| NFR-P001 | Punch p95 < 200 ms (one tx: lock + ≤ 4 queries) |
| NFR-P002 | Record / regularization lists (100 items) p95 < 500 ms |
| NFR-P003 | Summary for 10k employees p95 < 500 ms (aggregate SQL, no N+1) |
| NFR-P004 | DB query p95 < 50 ms (indexes §8) |
| NFR-SEC001 | Every query `tenant_id`-scoped; every id resolved in caller tenant |
| NFR-SEC002 | Self endpoints derive employee from JWT user only — no employee_id parameter accepted |
| NFR-D001 | Multi-table writes in one transaction; events via outbox in the same transaction |

## 8. Data (Postgres; migrations 000026–000031; all UUID PK, timestamptz)

- `shifts` — tenant_id, name, code?, start_minute (0–1439), end_minute, grace_period_mins, break_duration_mins, full_day_minutes?, half_day_minutes?, timezone, is_night_shift, status, created_at, updated_at, deleted_at. Unique (tenant_id, lower(name)) and (tenant_id, code) among non-deleted.
- `shift_assignments` — tenant_id, employee_profile_id, shift_id → shifts, effective_from date, effective_to date?, created_by_user_id?, created_at, updated_at. Index (tenant_id, employee_profile_id, effective_from).
- `attendance_records` — tenant_id, employee_profile_id, attendance_date date, shift_id?, first_punch_in?, last_punch_out?, total_work_minutes, total_break_minutes, late_minutes, overtime_minutes, status, source, is_regularized, created_at, updated_at. Unique (tenant_id, employee_profile_id, attendance_date). Index (tenant_id, attendance_date, status).
- `attendance_punches` — tenant_id, attendance_record_id → records, employee_profile_id, punch_time, punch_type, source, latitude?, longitude?, device_id?, ip_address?, is_superseded, created_at. Index (tenant_id, employee_profile_id, punch_time DESC), (attendance_record_id). Append-only.
- `attendance_regularizations` — tenant_id, employee_profile_id, attendance_date, attendance_record_id?, requested_punch_in, requested_punch_out, reason, status, requested_by_user_id, reviewer_user_id?, reviewed_at?, review_comment?, created_at, updated_at. Unique pending (tenant_id, employee_profile_id, attendance_date) WHERE status='pending'.
- `attendance_events_outbox` — tenant_id, event_id unique, event_type, routing_key, payload jsonb, published, published_at?, attempts, last_error?, created_at.

Employee/user references are logical FKs (validated through Module 2/0 contracts; same decision as Module 1 D: no hard cross-module FKs).

## 9. Edge cases

EC-01 punch with no employee profile → 404. EC-02 terminated employee punches → 403. EC-03 double IN / OUT without IN → 409. EC-04 two taps in 60 s → 409 DUPLICATE_PUNCH. EC-05 forgot to punch out yesterday, punches IN today (>20 h) → allowed; yesterday's record keeps open session and shows `present` with no `last_punch_out` until regularized. EC-06 night shift 22:00–06:00, IN 21:55 day D, OUT 06:10 D+1 → one record dated D, work 495, late 0. EC-07 night shift IN 01:00 D+1 (before end+4h) → record dated D. EC-08 no shift assigned → expected 480, late 0. EC-09 multiple sessions (lunch out/in) → breaks summed. EC-10 regularize a date with no record → record created with source `regularization`. EC-11 two pending regularizations same date → 409. EC-12 approver regularizes own request → 403. EC-13 cross-tenant record/shift/regularization id → 404. EC-14 shift update after records exist → old records unchanged. EC-15 concurrent punches by the same employee → serialized; second sees first (AT-022). EC-16 `per_page=0` / `page=-1` / `per_page=abc` → clamped defaults, 200. EC-17 `from > to` or range > 62 days on /me → 400. EC-18 assignment to inactive shift → 409.

## 10. Out of scope (explicit)

Leave requests/balances/policies (Module 4 — writes `on_leave` via contract). Holiday calendars (Module 4). Payroll LOP computation (Module 5). Biometric device ingestion / geofence enforcement (Module 12 — coordinates stored, not enforced). Multi-level approval chains (Module 6 engine; v1 = single approver with `attendance:approve`). Manager-scoped approval visibility (v1.1). Bulk import, auto-absent nightly job, shift rotations/rosters (v1.1).
