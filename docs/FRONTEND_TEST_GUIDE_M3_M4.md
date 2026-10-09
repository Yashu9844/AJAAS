# JAAS — Frontend Test Guide: Module 3 (Attendance) + Module 4 (Leave)

**Branch:** `module_4_leave` (contains Module 3; `module_3_attendance` is Module 3 alone). **Backend status:** both modules' backends are done and verified against real Postgres. The frontend is still a bootstrap with no login screen, so the frontend work (P8) is open for you.

Use this guide to build frontend tests that exercise every behaviour below. The backend's own proof is linked per item: `internal/...` are unit tests, and `tests/api/...` are live HTTP tests run with `-tags integration`.

---

## 1. What's done

| Module | Scope | Endpoints | Tables | Events | Proof |
|---|---|---|---|---|---|
| 3 Attendance | shifts, shift assignments, punch in/out, daily records with computed totals (work, break, late, overtime, status), regularization (timesheet correction) approvals, daily summary | 19 | 6 (`000025–000030`) | 5 on `jaas.attendance.events` | unit ≥ 91.9%, 6 live tests |
| 4 Leave | leave types (policies), holiday calendar, ledger-backed balances with lazy accrual and carry-forward, preview/apply/approve/reject/cancel, approved leave marked `on_leave` in attendance | 20 | 6 (`000031–000036`) | 5 on `jaas.leave.events` | unit ≥ 96.3%, 5 live tests |

Whole backend: `go build`, `go vet`, `go test ./internal/...` all pass, and the live suite passes 16/16 (attendance, leave, org). Full specs: `docs/modules/module-3-attendance/specification.md` and `docs/modules/module-4-leave/specification.md`. Swagger: `backend/api/swagger.yaml`.

---

## 2. Run the backend locally

```bash
docker compose up -d postgres redis rabbitmq          # or: docker start ajaas-postgres-1 ajaas-redis-1 ajaas-rabbitmq-1
cd backend
APP_ENV=development DATABASE_PASSWORD=postgres DATABASE_DBNAME=jaas_dev go run ./cmd
curl localhost:8080/health                             # {"status":"UP"}
```

On boot it auto-migrates the tables and seeds permissions `attendance:read|manage|approve` and `leave:read|manage|approve`.

### Tenant addressing (important for the browser)
The tenant comes from the **Host subdomain only**, so the browser must call `http://<slug>.localhost:8080/api/v1/...`. Chrome and Firefox resolve `*.localhost` to 127.0.0.1. CORS allows `*` (verified preflight from `http://localhost:3000`). Send the JWT as `Authorization: Bearer <token>` and **don't** use `credentials: 'include'`, because `*` + credentials is rejected by browsers.

### Create a test tenant + admin (no bootstrap API exists yet, so this uses SQL)
```bash
SLUG=fe-demo
curl -X POST localhost:8080/api/v1/tenants -H 'Content-Type: application/json' -d "{\"name\":\"FE Demo\",\"slug\":\"$SLUG\"}"
docker exec ajaas-postgres-1 psql -U postgres -d jaas_dev \
 -c "CREATE EXTENSION IF NOT EXISTS pgcrypto" \
 -c "INSERT INTO users (id,tenant_id,email,password_hash,first_name,last_name,status,created_at,updated_at) SELECT gen_random_uuid(), id, 'admin@$SLUG.com', crypt('Secret123!', gen_salt('bf',12)), 'Admin','User','active', now(), now() FROM tenants WHERE slug='$SLUG'" \
 -c "INSERT INTO user_roles (id,tenant_id,user_id,role_id,created_at,updated_at) SELECT gen_random_uuid(), u.tenant_id, u.id, r.id, now(), now() FROM users u JOIN tenants t ON t.id=u.tenant_id JOIN roles r ON r.tenant_id=t.id AND r.name='tenant_admin' WHERE t.slug='$SLUG' AND u.email='admin@$SLUG.com'"
```
This recipe was verified on 2026-10-09. `tenant_admin` bypasses all permission checks.

### Then, through the API (as admin, on `http://$SLUG.localhost:8080`)
1. `POST /api/v1/auth/login` `{email, password:"Secret123!", tenant_slug}` returns `data.access_token` (15 min), `data.refresh_token` and `data.user.id`.
2. Create a plain member with `POST /api/v1/users` `{email, password, first_name, last_name}`. The member has **no** attendance/leave permissions, which makes them a good RBAC test user.
3. Create employee profiles for **both** users (attendance and leave need one): `POST /api/v1/employees` `{user_id, employee_code, first_name, last_name, employment_type:"full_time", joining_date:"2026-01-01T00:00:00Z"}`.

**Login rate limit:** 10 logins per 15 min per IP. If tests hit `429`, clear it with:
`docker exec ajaas-redis-1 sh -c "redis-cli --scan --pattern 'ratelimit:/api/v1/auth/login:*' | xargs -r redis-cli DEL"`

### Response envelope (all endpoints)
- Success: `{"data": ...}`. Lists return `{"data": [...], "meta": {page, per_page, total_items, total_pages}}`.
- Error: `{"error": {"code", "message", "details"?: [{field, message}]}}`. Bad body fields return `400 VALIDATION_ERROR` with one detail per field.
- Pagination: `page` ≥ 1, `per_page` 1–100 (default 20). Garbage values are clamped and **never** produce a 500.
- Another tenant's id returns **404** (never 403). A JWT from tenant A used on tenant B's subdomain returns **403**.

---

## 3. Module 3 — Attendance API

| # | Method & path | Who | Notes |
|---|---|---|---|
| A1 | POST `/attendance/punch` `{type:"in"\|"out", latitude?, longitude?, device_id?, source?}` | any employee | Server clock only; client time fields are ignored. Rate limit 30/min/IP. Returns `201 {punch, record}` |
| A2 | GET `/attendance/me/today` | self | `{attendance_date, record?, open_session, shift?}` |
| A3 | GET `/attendance/me?from&to` | self | default last 7 days, max 62 |
| A4 | POST `/attendance/regularizations` `{attendance_date, requested_punch_in, requested_punch_out, reason}` | self | date within the last 30 days, not in the future |
| A5 | GET `/attendance/regularizations/me?status` | self | |
| A6 | POST `/attendance/regularizations/{id}/cancel` | owner | pending only |
| A7 | GET `/attendance/records?employee_id&from&to&status` | `attendance:read` | |
| A8 | GET `/attendance/records/{id}` | `attendance:read` | includes superseded punches |
| A9 | GET `/attendance/summary?date` | `attendance:read` | `{total_employees, present, late, half_day, on_leave, absent}` |
| A10 | GET `/attendance/regularizations?status&employee_id` | `attendance:read` | |
| A11 | POST `/attendance/regularizations/{id}/approve` `{comment?}` | `attendance:approve` | rebuilds the day from the requested window |
| A12 | POST `/attendance/regularizations/{id}/reject` `{comment}` | `attendance:approve` | comment required |
| S1–S5 | POST/GET `/shifts`, GET/PATCH `/shifts/{id}`, POST `/shifts/{id}/deactivate` | manage / read | times `"HH:MM"`, IANA `timezone`; night shifts supported |
| S6 | POST `/shifts/{id}/assignments` `{employee_id, effective_from, effective_to?}` | `attendance:manage` | auto-closes the previous open assignment |
| S7 | GET `/shift-assignments?employee_id` | `attendance:read` | |

**Record statuses:** `present`, `half_day`, `absent`, `on_leave`, `holiday`, `week_off`.
**Error codes to assert:** `ALREADY_PUNCHED_IN`, `NOT_PUNCHED_IN`, `DUPLICATE_PUNCH` (< 60 s since the last punch; checked **before** the in/out rule), `SESSION_EXPIRED` (open > 20 h), `EMPLOYEE_NOT_FOUND`, `EMPLOYEE_NOT_ACTIVE`, `REGULARIZATION_WINDOW`, `REGULARIZATION_PENDING`, `REGULARIZATION_NOT_PENDING`, `SELF_APPROVAL_FORBIDDEN`, `ASSIGNMENT_OVERLAP`, `SHIFT_INACTIVE`, `SHIFT_IN_USE`, `CONFLICT`.

---

## 4. Module 4 — Leave API

| # | Method & path | Who | Notes |
|---|---|---|---|
| L1 | POST `/leave/types` | `leave:manage` | `{name, code (2–10, upper-cased, immutable), is_paid=true, annual_allowance (0–365), accrual "annual"\|"monthly", carry_forward_limit, max_consecutive_days?, min_notice_days=0, allow_half_day=true, sandwich_rule=false, applicable_gender "all"\|"male"\|"female"}` |
| L2/L3 | GET `/leave/types`, `/leave/types/{id}` | any logged-in user | employees need these to apply |
| L4 | PATCH `/leave/types/{id}` | `leave:manage` | `max_consecutive_days: 0` clears the limit |
| L5 | POST `/leave/types/{id}/deactivate` | `leave:manage` | no new requests; existing ones still approvable |
| H1–H3 | POST `/leave/holidays` `{date, name, is_optional}`, GET `/leave/holidays?year`, DELETE `/leave/holidays/{id}` | manage / any / manage | one holiday per date; optional holidays count as working days |
| B1 | GET `/leave/balances/me` | self | one row per active type for the current year: `{opening, accrued, adjusted, used, reserved, available, is_paid}` |
| B2 | GET `/leave/balances?employee_id` | `leave:read` | |
| B3 | POST `/leave/balances/adjust` `{employee_id, leave_type_id, days (±, non-zero), reason}` | `leave:manage` | |
| B4 | GET `/leave/ledger?employee_id&leave_type_id` | `leave:read` | rows of kind `accrual`, `carry_forward`, `adjustment`, `reserve`, `release`, `consume`, `reversal` |
| R1 | POST `/leave/requests/preview` | self | same body as R2; returns `{total_days, available, available_after, working_dates[]}` and saves nothing |
| R2 | POST `/leave/requests` `{leave_type_id, start_date, end_date, half_day?:"first_half"\|"second_half", reason}` | self | reserves the days; status `pending` |
| R3 | GET `/leave/requests/me?status` | self | |
| R4 | POST `/leave/requests/{id}/cancel` | owner | pending at any time; approved only if it starts after today |
| R5/R6 | GET `/leave/requests?status&employee_id&from&to`, `/leave/requests/{id}` | `leave:read` | |
| R7 | POST `/leave/requests/{id}/approve` `{comment?}` | `leave:approve` | reserved becomes used; **attendance days become `on_leave`** |
| R8 | POST `/leave/requests/{id}/reject` `{comment}` | `leave:approve` | comment required |

**Days** are JSON numbers with at most 2 decimals (`1.5`); 3 decimals are rejected.
**Day counting:** Monday–Friday minus non-optional holidays; a half day is 0.5. With the sandwich rule, weekends and holidays *between* leave days are counted too.
**Balance:** `available = opening + accrued + adjusted − used − reserved`. Annual accrual is granted up front, prorated by joining month; monthly accrual is allowance/12 per month so far. Carry-forward is `min(last year's available, limit)`.
**Error codes to assert:** `INVALID_LEAVE_RANGE` (start after end, or outside the current year), `LEAVE_NOTICE`, `NO_WORKING_DAYS`, `LEAVE_TOO_LONG`, `LEAVE_TYPE_INACTIVE`, `LEAVE_TYPE_NOT_APPLICABLE` (gender), `LEAVE_OVERLAP`, `INSUFFICIENT_BALANCE` (paid types only; unpaid types skip the check), `LEAVE_NOT_PENDING`, `LEAVE_ALREADY_STARTED`, `LEAVE_NOT_CANCELLABLE`, `SELF_APPROVAL_FORBIDDEN`, `EMPLOYEE_NOT_FOUND`, `EMPLOYEE_NOT_ACTIVE`, `CONFLICT`.

---

## 5. Frontend test checklist (each item is verified on the backend; your UI should prove the same)

### Attendance
- [ ] **Punch widget:** IN gives 201 and today shows `open_session: true`. A second IN within 60 s gives `DUPLICATE_PUNCH`; after 60 s it gives `ALREADY_PUNCHED_IN`. OUT closes the session. OUT with no session gives `NOT_PUNCHED_IN`. *(tests/api/attendance_flow_test.go)*
- [ ] **Concurrent taps:** 10 rapid IN clicks produce exactly one punch. *(attendance_race_test.go)*
- [ ] **Totals and status:** a day record shows work, break, late and overtime minutes and its status; a night shift is attributed to its start date. *(internal/attendance/calc goldens G4/G5)*
- [ ] **Regularization:** the member submits, a second request for the same date gives `REGULARIZATION_PENDING`, the admin approves, and the record shows `is_regularized` with 540 work minutes for 09:00–18:00. The admin approving their own request gives 403. *(attendance_rules_test.go)*
- [ ] **Shifts:** create, assign, list assignments; an overlapping assignment auto-closes the old one; deactivating a shift in use gives `SHIFT_IN_USE`.
- [ ] **Summary card:** the counts match the records.
- [ ] **RBAC:** the member gets 403 on records, summary, shifts and the regularization list, but `/me`, punch and own regularizations work. *(G9)*
- [ ] **Privacy:** no IP address is ever shown in any response.

### Leave
- [ ] **Leave types admin:** create (code shown upper-cased), edit, deactivate; a duplicate name or code gives `CONFLICT`.
- [ ] **Holidays admin:** add, list by year, delete; a duplicate date gives `CONFLICT`.
- [ ] **Apply form + preview:** preview shows days and available-after; Fri–Mon counts 2, or 4 with the sandwich rule; a weekend-only range gives `NO_WORKING_DAYS`; a holiday inside the range is excluded. *(leave_flow_test.go, calc golden G2)*
- [ ] **Apply:** the balance card shows `reserved` up and `available` down. *(G5)*
- [ ] **Approve:** used +, reserved −, and the employee's attendance for those dates shows `on_leave`. *(G8, verified live in the attendance tables)*
- [ ] **Reject:** without a comment gives 400; with a comment the reservation is released.
- [ ] **Cancel:** a pending request is released; an approved future request is reversed and the attendance `on_leave` marks are removed. *(G13)*
- [ ] **Overlap:** an overlapping range gives `LEAVE_OVERLAP`; first half and second half of the same day are both allowed. *(G7)*
- [ ] **Insufficient balance** on a paid type gives 409; an unpaid (LOP) type lets the balance go negative.
- [ ] **Adjust + ledger:** an HR adjustment shows in the ledger with its note; the ledger rows sum to the balance. *(G12)*
- [ ] **Self-approval** gives 403. **Member** gets 403 on type/holiday writes, the request list, others' balances, the ledger, adjust and approve, but self endpoints work. *(G9/G10)*
- [ ] **Isolation:** with two tenants, ids from one return 404 in the other. *(G1)*
- [ ] **Pagination:** `per_page=0` and `page=-1` render page 1 with 20 rows and no error. *(G15)*

---

## 6. Known limitations (expected behaviour, not bugs)
- Leave has a single approver: anyone with `leave:approve` except the requester. Manager routing and multi-step chains are planned for Module 6.
- Weekly off is fixed to Saturday and Sunday; the leave year is the calendar year (UTC); requests must fall in the current year.
- Half-day leave doesn't change attendance status. Punching in on a leave day turns that day back to `present`.
- No file attachments (proof) yet. No first-admin bootstrap API (use the SQL in §2).
- Performance targets (p95) weren't measurable on the dev machine (Docker under memory pressure); re-check on a normal machine.
- Inherited from Module 0, unfixed: `/api/v1/tenants` is unauthenticated; CORS is `*`; a soft-deleted role's permissions still apply.

## 7. Where things live
- Backend: `backend/internal/attendance/**`, `backend/internal/leave/**`
- Live tests: `backend/tests/api/{attendance,leave}_*_test.go` (run: `go test -tags integration -count=1 ./tests/api/`)
- Module docs: `docs/modules/module-3-attendance/`, `docs/modules/module-4-leave/` (specification, golden tests, decisions, handoff)
