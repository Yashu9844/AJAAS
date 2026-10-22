# HR / Attendance & Time — Decisions

> Part of **M1 HR Management** (bounded context `attendance`). Formerly the stand-alone "module-3-attendance" bundle; module numbers in older text were renamed to the canonical M0–M12 / HR-context names.

Format: Decision / Reason / Alternatives / Tradeoff.

2026-09-30 — Scaffold: 17-file bundle created before design (docs-only).

2026-10-08 (P1 design)
- D3-01 Scope = masterplan HR/Attendance (Attendance, Shifts & Time Tracking); Leave moves to HR/Leave. Reason: newest architecture doc (SYSTEM_ARCHITECTURE_MASTERPLAN, 2026-10-06) and the frontend Phase-2 plan split them; combined module would exceed one-service-at-a-time loop size. Alternative: DEPENDENCY-GRAPH combined "Attendance & Leave". Tradeoff: DEPENDENCY-GRAPH/MASTER_PROMPT registry rows updated; HR/Leave bundle must be re-scoped from Projects to Leave before G4-1 (user confirmation requested).
- D3-02 Key everything by `employee_profile_id` (HR/Employee), not user_id. Reason: attendance is employment data; history survives user deactivation. Tradeoff: self endpoints resolve user→employee per request (one indexed lookup).
- D3-03 Shift times as minutes-from-midnight ints + per-shift IANA timezone. Reason: integer night-shift math, no TIME-type driver quirks, correct lateness across zones. Alternative: Postgres TIME + tenant timezone setting. Tradeoff: API converts HH:MM.
- D3-04 Server clock only for punches. Reason: anti-backdating (AS-T1). Corrections go through regularization approval.
- D3-05 Controllers do not hold `*gorm.DB`; services own a `txRunner` and open real transactions. Reason: M0 Identity, HR/Organization, HR/Employee controllers pass raw DB (no atomicity); HR/Attendance must not copy that. Tradeoff: differs from sibling modules' constructor shape (documented here, not a contract change).
- D3-06 Transactional outbox written inside the business tx + relay goroutine. Reason: FR-EV002 guarantee; HR/Organization, HR/Employee publish-then-fallback can lose events on crash. Tradeoff: one extra insert per write; relay started from cmd/main.go.
- D3-07 Audit after commit, best-effort. Reason: an audit insert failing inside a Postgres tx aborts the whole tx; audit must never fail the API (docs/SECURITY.md §7).
- D3-08 Supersede punches on regularization approval (append-only). Reason: AT-019 audit trail.
- D3-09 Advisory xact lock per employee for punch/approve. Reason: no row to lock before the first punch of a day; lock is released automatically at commit.
- D3-10 Permission seed: HR/Attendance boot inserts `attendance:read|manage|approve` into M0 Identity `permissions` (ON CONFLICT DO NOTHING) through identity `models.Permission`. Reason: AS-007 says permissions seeded at deploy and nothing seeds module permissions today. Cross-module touch recorded in module-0 decisions.md. Alternative: SQL-only seed migration. Tradeoff: dev (AutoMigrate) and prod paths both seeded.
- D3-11 Live tests behind `//go:build integration`. Reason: `go test ./...` must stay runnable without Docker; existing untagged org tests in tests/api are left untouched (HR/Organization scope).
- D3-12 Thresholds (`full_day_minutes`, `half_day_minutes`) per shift with derived defaults. Reason: tenants tune tolerance without code; no hidden constants.
- D3-13 (P7) Rule precedence: AT-004 debounce is evaluated before AT-003 alternation. A second tap inside 60 s is a duplicate (409 `DUPLICATE_PUNCH`), not a state error. Found when the live run returned DUPLICATE_PUNCH for in→in within 60 s; the service order is intended, the spec was silent. Live tests backdate punch rows in SQL instead of sleeping.
- D3-14 (P7) Dev/prod schema parity is enforced by model tests: minute counters `type:integer`; `uq_shifts_tenant_name` = (tenant_id, lower(name)) WHERE deleted_at IS NULL; `uq_shifts_tenant_code` WHERE deleted_at IS NULL AND code IS NOT NULL; `uq_attendance_regularizations_pending` partial on status='pending'; `idx_attendance_outbox_published` WHERE published = false. Reason: the live diff showed AutoMigrate missing the AT-015 backstop and the case-insensitive/soft-delete-aware shift uniqueness. Note: AutoMigrate does not replace an existing same-name index, so a dev DB booted before this change needs `DROP INDEX uq_shifts_tenant_name, uq_shifts_tenant_code, idx_attendance_outbox_published` once (done on local jaas_dev 2026-10-09).
- D3-15 (P7) Live suite clears M0 Identity's per-IP login counter in Redis inside the shared `doReq` helper for login calls only. Reason: a full run bootstraps >10 tenants/users from one IP and tripped the 10/15 min limiter (also broke HR/Organization's TestOrgRBAC). Production limiter unchanged.
- D3-16 (P9) Params ≤4 (loop.md) counts business parameters; `ctx context.Context` and the transaction handle `tx *gorm.DB` are infrastructure plumbing and are not counted. All HR/Attendance functions comply under this reading (AST check 2026-10-09); five that did not were refactored (`events.Source`, `auditEntry`, `punchTx` state, `RouteDeps`). Reason: repository methods must take ctx + tx + tenantID by design (tenant isolation, D3-05), so a literal count would force struct-wrapping every query; HR/Employee repositories follow the same shape. Revisit if the human wants the literal rule.
- D3-17 (2026-10-09, from HR/Leave D4-08) HR/Attendance exposes `services.LeaveSync` (MarkLeave/ClearLeave running on the caller's tx under the HR/Attendance per-employee lock) and record source `leave`, fulfilling reserved contract C10. Leave-only records (source `leave`, no punches) are deleted on ClearLeave; records with punches are recomputed.
- D3-18 (2026-10-09) Integration onto M0 Identity, HR/Organization, HR/Employee hardening: migrations renumbered to 000026–000031 (000025 = refresh_token_session_link); module wired in internal/app/time_modules.go — relay lifecycle via app.Options.Background (nil in tests); controller fail() runs shared errors.Normalize; live tests ported to tests/integration (tests/api removed by hardening). Module-owned permission seed (D3-10) kept alongside the M0 Identity DefaultPermissions catalogue.

## Changelog

2026-09-30 — Created: 17-file doc bundle scaffold. Impact: docs only.
2026-10-08 — P1 design DONE: scope re-defined to Attendance, Shifts & Time Tracking (D3-01); specification/architecture/connections/security/goldens/testing/decisions/assumptions/files frozen. Impact: contracts frozen for P2 (19 endpoints, 6 tables, 5 events). Tests: n/a (docs).
2026-10-08 — P2 DONE: 6 GORM models + constants, models_test (table names, index contracts, append-only, UUID hooks), migrations 000026–000031. Tests: go test ./internal/attendance/models (100% cov). Impact: new tables only, no existing contract changed.
2026-10-09 — P3 DONE: calc engine (goldens G4/G5), DTOs (+G13 pagination clamp), validators. Tests: go test ./internal/attendance/... (calc/dto/validators/models 100%). Impact: none external.
2026-10-09 — P4 DONE: repositories (6 interfaces + GORM impls, advisory lock, FindOrCreate, status aggregate). Tests: build/vet (SQL verified live at P7). Impact: none external.
2026-10-09 — P5 DONE: services (shift, assignment, punch, query, regularization, outbox relay) + events; unit goldens G2/G3/G6/G7/G8/G10/G11/G12 green; services 91.8%. Impact: none external.
2026-10-09 — P6 DONE: controllers + routes (19, RBAC contract test), module.go (directory adapter, permission seed, relay, punch rate limit), cmd/main.go wiring, swagger 19 operations + 24 schemas. Tests: go build/vet ./..., go test ./internal/... PASS; controllers/routes 100%. Impact: new public API (gated GO 2026-10-08); 3 rows in M0 Identity permissions (D3-10).
2026-10-09 — P7 DONE: live goldens G1, G2, G6–G9, G11–G13 PASS on Docker Postgres; migrations up/down/up clean; AutoMigrate↔SQL parity fixed (D3-14: integer columns, partial/expression unique indexes, pending backstop); AT-004 precedence clarified (D3-13); suite-level login limiter reset (D3-15, also unblocks HR/Organization TestOrgRBAC in full runs). Impact: dev DBs created before this need 3 indexes dropped once (see D3-14).
2026-10-09 — P9 DONE (backend CLOSED): DoD §24 walked with evidence; 5 functions refactored to params ≤4 (D3-16); routes↔swagger 19/19; threat→evidence map; live AS-T6 concurrency test (10 parallel INs → 1 session). P8 frontend BLOCKED (no identity shell).
2026-10-09 — Integrated onto M0 Identity, HR/Organization, HR/Employee hardening (branch integrate/m3-m4-on-hardening): migrations renumbered 000026–000037, app wiring, error normalization, integration tests ported; 59/59 PASS.

## Parity decisions (2026-10-22)

- **DA-P1** The parity features live in `attendance/ops` and write through `attendance/services.TimeWriter`, so the
  punch, regularization and import paths share one recompute (AT-006..AT-010). Regularization approval was refactored onto the
  same `setWindow`; its goldens did not change.
- **DA-P2** The inbox decides through the existing regularization service. Managers do not get tenant-scope
  `attendance:approve`; reach is by organization mapping (primary, active, `manager_user_id`), and the routes need only a
  login. A stranger gets 404, not 403, so ids do not leak.
- **DA-P3** Rosters generate ordinary shift assignments instead of a second scheduling model. Punch resolution (AT-005)
  and AT-011 stay untouched. The cost is one assignment row per segment, bounded at 200 per worker per apply.
- **DA-P4** Auto-absent is opt-in per tenant (`auto_absent_enabled`, default off) so no tenant gets absent records it
  did not ask for. Each worker commits on its own, which makes the run resumable and idempotent; the daily sweep
  skips a tenant/day that already has a run logged.
- **DA-P5** Holidays come from HR/Leave through `leave/api.Holidays` (non-optional tenant holidays only). Per-location
  calendars arrive with the leave parity work and plug in behind the same port.
- **DA-P6** Overtime is a report computed from stored records, not a ledger: policy changes re-price history on the
  next read. Payroll will consume the report through a port in the payroll parity work (decision there).
- **DA-P7** Bulk import takes instants (RFC 3339) and requires `punch_in` within a day of `date`, so offsets of any
  timezone pass. It is all-or-nothing and logs counts only (AP-12).
