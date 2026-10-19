# HR / Attendance & Time — Testing

> Part of **M1 HR Management** (bounded context `attendance`). Formerly the stand-alone "module-3-attendance" bundle; module numbers in older text were renamed to the canonical M0–M12 / HR-context names.

## Golden tests (protected)

Rules: docs/ENGINEERING.md (golden tests). Never edit a golden to make red pass. Change = reason → spec update → contract version → dependent update → full suite.
Two rings: **unit goldens** (no infra, always run by `go test ./internal/...`) and **live goldens** (`//go:build integration`, real backend + Docker PG).

| # | Golden behavior | Input | Expected | Why | Location |
|---|---|---|---|---|---|
| G1 | Tenant isolation | Tenant B admin reads tenant A record / shift / regularization ids; tenant A JWT on B subdomain | 404 each; 403 on subdomain mismatch | Isolation is the boundary | tests/api/attendance_rules_test.go (live PASS 2026-10-09) |
| G2 | Punch alternation | in, in, out, out | 201, 409 ALREADY_PUNCHED_IN, 201, 409 NOT_PUNCHED_IN | Session integrity | services/punch_golden_test.go + live attendance_flow_test.go (PASS) |
| G3 | Server clock only | punch with extra `punch_time` field in body | ignored; punch_time = server now | Anti-backdating | services/punch_golden_test.go |
| G4 | Totals engine | fixed shifts × punch sets (day, late, break, overtime, half-day, absent, no shift) | exact work/break/late/overtime/status table | Pay depends on it | calc/calc_golden_test.go |
| G5 | Night shift attribution | 22:00–06:00 shift, IN 21:55 D / 01:00 D+1, OUT 06:10 D+1, Asia/Kolkata tz | record date D, work 495, late 0 | Cross-midnight correctness | calc/calc_golden_test.go |
| G6 | Regularization lifecycle | request → approve | record regularized, old punches superseded, totals from requested window, event + audit | Correction flow | services/regularization_golden_test.go + live attendance_rules_test.go (PASS) |
| G7 | Self-approval forbidden | approver approves own request | 403 SELF_APPROVAL_FORBIDDEN, request still pending | Segregation of duties | services/regularization_golden_test.go + live attendance_rules_test.go (PASS) |
| G8 | One pending per date | second request same date | 409 REGULARIZATION_PENDING | No conflicting corrections | services/regularization_golden_test.go + live attendance_rules_test.go (PASS) |
| G9 | RBAC | member (no attendance perms) on records/summary/shifts; same member on self endpoints | 403; 200/201 | Least privilege | tests/api/attendance_rules_test.go (live PASS) |
| G10 | Assignment overlap | open-ended A from Jan 1, new B from Mar 1; then C overlapping B | A closed Feb 28, B created; C 409 ASSIGNMENT_OVERLAP | Single active shift | services/assignment_golden_test.go |
| G11 | Audit on every state change | punch, regularize, approve, shift create/assign | audit row per action, no IP/coords in metadata | Repudiation defense | services (spy) + live attendance_flow_test.go (PASS) |
| G12 | Outbox atomic with state | punch | outbox row in same tx; relay publishes; payload has no ip/device/coords | No lost/ghost events, privacy | services/outbox_golden_test.go + live attendance_flow_test.go (PASS) |
| G13 | Pagination never crashes | per_page=0, page=-1, per_page=abc | 200 with defaults | Fixes M0 Identity crash class | controllers + live attendance_flow_test.go (PASS) |

## Test strategy and commands

Strategy: RED first per item. Unit (calc 100%, services ≥ 90%, module ≥ 80%) + unit goldens (G2–G8, G10–G12) + live goldens (G1, G6, G7, G9, G11–G13) + migration up/down/up.

### Commands
```bash
export PATH="$HOME/go-sdk/go/bin:$PATH"     # local Go 1.27.1
cd backend
go build ./... && go vet ./... && gofmt -l .
go test -count=1 ./internal/attendance/...
go test -count=1 -cover ./internal/attendance/...
grep -rn "TODO" internal/attendance/            # must be empty
# live ring (Docker PG/Redis/RabbitMQ + backend on :8080):
docker compose up -d postgres redis rabbitmq     # repo root
DATABASE_PASSWORD=postgres DATABASE_DBNAME=jaas_dev go run ./cmd/main.go &
go test -count=1 -tags integration ./tests/api/ -run Attendance -v
# migrations up/down/up (psql inside container, files 000026..000031)
```
`-race` needs cgo (unavailable on this Windows box) — reported as not run, never claimed.

### Suites
- `calc`: table tests incl. goldens G4/G5 (fixed instants, multiple time zones).
- `services`: fakes for repos, employee directory, audit spy, publisher spy, fake txRunner; goldens G2/G3/G6/G7/G8/G10/G11/G12.
- `controllers`: httptest with stub services — binding → VALIDATION_ERROR, status mapping, pagination clamp (G13), self endpoints never read employee_id.
- `models`: GORM schema parse, table names, unique index names.
- live (build tag `integration`): `attendance_flow_test.go` (punch flow G2/G11/G12, G13), `attendance_rules_test.go` (G6/G7/G8 regularization, G9 RBAC, G1 isolation), `attendance_race_test.go` (AS-T6: 10 concurrent INs → exactly one session), `attendance_helpers_test.go` (tenant+admin+member+employees bootstrap, punch backdating). Last run 2026-10-09: PASS. Not run: `go test -race` (no cgo toolchain on this Windows host).

### Integration onto M0 Identity, HR/Organization, HR/Employee hardening (2026-10-09)
Merged onto `fix/module-0-2-hardening` as branch `integrate/m3-m4-on-hardening`. Migrations renumbered 000025–000036 → 000026–000037 (SQL runner is the schema authority, D-HARDEN-3); module wired in `internal/app` (`time_modules.go`); controllers normalize persistence errors (unique violation → 409); live tests ported from the removed `tests/api` suite to `tests/integration` (in-process, real Postgres + Redis) — run with `JAAS_REQUIRE_INTEGRATION=1 go test -count=1 ./tests/integration/`. Full suite 59/59 PASS; migration cycle 37 up / 12 down / 12 up clean.

## Delivery evidence (status and Definition of Done at backend close)

Implemented:
- P1 design bundle: specification (FR-SH/SA/PU/AR/RG/EV, AT-001..AT-022, 19 endpoints, 6 tables, 5 events), architecture, connections C1–C10, security, goldens G1–G13, testing, decisions D3-01..D3-12, assumptions.
- P2: `backend/internal/attendance/models` (Shift, ShiftAssignment, AttendanceRecord, AttendancePunch, Regularization, OutboxEvent + status/type constants); models_test green, 100% coverage. Migrations 000026–000031 (up/down) incl. SQL-only partial indexes (lower(name), pending-regularization uniqueness) and CHECK constraints.
- P3: `calc` pure engine (goldens G4/G5 green), `dto` (all request/response shapes, ParsePage clamp — G13 unit), `validators` (shift code, IANA tz, dates/range, thresholds). All 100% coverage.
- P4: `repositories` — 6 interfaces + GORM impls (shift, assignment, record, punch, regularization, outbox); tenant-scoped; advisory lock; FindOrCreate; single-query status aggregate. Verified on real Postgres by the P7 live goldens.
- P5: `services` (Shift, Assignment, Punch, Query, Regularization, OutboxRelay) + `events`; real transactions via TxRunner; outbox written in-tx, published after commit, relay retries; audit after commit (best-effort). Unit goldens G2, G3, G6, G7, G8, G10, G11, G12 green; services 91.8%, events 100%.
- P6: `controllers` (100%), `routes` (19 routes, RBAC contract test, 100%), `module.go` (HR/Employee directory adapter, permission seed attendance:read|manage|approve, outbox relay, punch rate limit 30/min/IP), `cmd/main.go` wiring (AutoMigrate, seed, routes, relay start/stop), swagger (16 paths / 19 operations, 24 schemas, refs validated). Whole backend: build + vet clean, all ./internal unit tests PASS.
- P7: live ring on Docker Postgres/Redis/RabbitMQ — `tests/api/attendance_{flow,rules}_test.go` (`-tags integration`): punch flow (G2, AT-004 precedence D3-13, G11 audit, G12 outbox privacy, IP kept on punch row only), G13, regularization lifecycle (G6, G7, G8, AT-018), G9 RBAC, G1 isolation — all PASS; full tests/api suite PASS (10). Schema parity fixes (D3-14): integer columns, partial + case-insensitive shift uniqueness, pending-regularization backstop on the dev path.

Partially Implemented:
- Migrations verified live: 30 up → 6 down → 6 up on scratch DB; AutoMigrate and SQL schemas identical (columns + index definitions).

Not Implemented:
- frontend (P8) — BLOCKED: frontend has no identity login shell yet.

Known Issues:
- HR/Leave bundle still describes Projects; must be re-scoped to Leave (D3-01) before G4-1.
- Inherited from M0 Identity (not fixed here): RequirePermission grants permissions of soft-deleted roles; `/tenants` unauthenticated. HR/Attendance relies on RequirePermission as-is.

Blocked:
- P8 frontend needs an identity login shell (no frontend code exists yet).

Technical Debt:
- None yet.

### Definition of Done (docs/ENGINEERING.md, Definition of Done) — backend, 2026-10-09
- [x] Requirements understood — FR-SH/SA/PU/AR/RG/EV + AT-001..AT-022 cited in code comments and tests
- [x] Dependencies checked — connections.md C1–C10; M0 Identity (auth, RBAC, audit, rate limit), HR/Employee (employee directory)
- [x] Implementation complete (goal scope) — 19 endpoints, 6 tables, 5 events, outbox relay
- [x] Code quality — AST check: func ≤50, file ≤300, depth ≤4, params ≤4 (D3-16), no panic/TODO/nolint
- [x] Unit pass — `go test ./internal/...` (attendance: 100% all pkgs except services 91.8%)
- [x] Integration pass — `go test -tags integration ./tests/api/` 11/11 (6 attendance + 5 org)
- [x] Contract pass — live Gin routes ↔ swagger operations 19/19 identical; router contract test
- [x] Golden pass — G1–G13 (unit and/or live, see golden-tests.md)
- [x] Security verified — security.md threat → evidence table AS-T1..T9
- [x] Dependent tests pass — HR/Organization live suite green in the same run
- [x] No unrelated modules modified — outside touches are integration points only: cmd/main.go, api/swagger.yaml, tests/api shared helpers (D3-15), module-0 decisions.md (D3-10), DEPENDENCY-GRAPH.md
- [x] Docs updated — spec (AT-004 note), status, todo, plan, handoff, changelog, files, decisions D3-13..16, security, testing
- [x] Diff reviewed · [x] No secrets/config leaks (scan clean)
- Not run: `go test -race` (no cgo on this host) — compensated by the live AS-T6 concurrency test.

## Manual verification checklist

From the partner frontend test guide (2026-10-09). Paths are the pre-restructure ones; after Phase C they are prefixed `/api/v1/hr`. These become the context's test-console scenarios in Phase D.

### 3. HR/Attendance — Attendance API

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

#### Attendance
- [ ] **Punch widget:** IN gives 201 and today shows `open_session: true`. A second IN within 60 s gives `DUPLICATE_PUNCH`; after 60 s it gives `ALREADY_PUNCHED_IN`. OUT closes the session. OUT with no session gives `NOT_PUNCHED_IN`. *(tests/integration/attendance_test.go TestAttendancePunchFlow)*
- [ ] **Concurrent taps:** 10 rapid IN clicks produce exactly one punch. *(attendance_test.go TestAttendanceConcurrentPunches)*
- [ ] **Totals and status:** a day record shows work, break, late and overtime minutes and its status; a night shift is attributed to its start date. *(internal/attendance/calc goldens G4/G5)*
- [ ] **Regularization:** the member submits, a second request for the same date gives `REGULARIZATION_PENDING`, the admin approves, and the record shows `is_regularized` with 540 work minutes for 09:00–18:00. The admin approving their own request gives 403. *(attendance_test.go TestAttendanceRegularization)*
- [ ] **Shifts:** create, assign, list assignments; an overlapping assignment auto-closes the old one; deactivating a shift in use gives `SHIFT_IN_USE`.
- [ ] **Summary card:** the counts match the records.
- [ ] **RBAC:** the member gets 403 on records, summary, shifts and the regularization list, but `/me`, punch and own regularizations work. *(G9)*
- [ ] **Privacy:** no IP address is ever shown in any response.
