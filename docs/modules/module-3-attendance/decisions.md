# Module 3 — Decisions (ADR log)

Format: Decision / Reason / Alternatives / Tradeoff.

2026-09-30 — Scaffold: 17-file bundle created before design (docs-only).

2026-10-08 (P1 design)
- D3-01 Scope = masterplan Module 3 (Attendance, Shifts & Time Tracking); Leave moves to Module 4. Reason: newest architecture doc (SYSTEM_ARCHITECTURE_MASTERPLAN, 2026-10-06) and the frontend Phase-2 plan split them; combined module would exceed one-service-at-a-time loop size. Alternative: DEPENDENCY-GRAPH combined "Attendance & Leave". Tradeoff: DEPENDENCY-GRAPH/MASTER_PROMPT registry rows updated; Module 4 bundle must be re-scoped from Projects to Leave before G4-1 (user confirmation requested).
- D3-02 Key everything by `employee_profile_id` (Module 2), not user_id. Reason: attendance is employment data; history survives user deactivation. Tradeoff: self endpoints resolve user→employee per request (one indexed lookup).
- D3-03 Shift times as minutes-from-midnight ints + per-shift IANA timezone. Reason: integer night-shift math, no TIME-type driver quirks, correct lateness across zones. Alternative: Postgres TIME + tenant timezone setting. Tradeoff: API converts HH:MM.
- D3-04 Server clock only for punches. Reason: anti-backdating (AS-T1). Corrections go through regularization approval.
- D3-05 Controllers do not hold `*gorm.DB`; services own a `txRunner` and open real transactions. Reason: Module 0–2 controllers pass raw DB (no atomicity); Module 3 must not copy that. Tradeoff: differs from sibling modules' constructor shape (documented here, not a contract change).
- D3-06 Transactional outbox written inside the business tx + relay goroutine. Reason: FR-EV002 guarantee; Modules 1/2 publish-then-fallback can lose events on crash. Tradeoff: one extra insert per write; relay started from cmd/main.go.
- D3-07 Audit after commit, best-effort. Reason: an audit insert failing inside a Postgres tx aborts the whole tx; audit must never fail the API (docs/SECURITY.md §7).
- D3-08 Supersede punches on regularization approval (append-only). Reason: AT-019 audit trail.
- D3-09 Advisory xact lock per employee for punch/approve. Reason: no row to lock before the first punch of a day; lock is released automatically at commit.
- D3-10 Permission seed: Module 3 boot inserts `attendance:read|manage|approve` into Module 0 `permissions` (ON CONFLICT DO NOTHING) through identity `models.Permission`. Reason: AS-007 says permissions seeded at deploy and nothing seeds module permissions today. Cross-module touch recorded in module-0 decisions.md. Alternative: SQL-only seed migration. Tradeoff: dev (AutoMigrate) and prod paths both seeded.
- D3-11 Live tests behind `//go:build integration`. Reason: `go test ./...` must stay runnable without Docker; existing untagged org tests in tests/api are left untouched (Module 1 scope).
- D3-12 Thresholds (`full_day_minutes`, `half_day_minutes`) per shift with derived defaults. Reason: tenants tune tolerance without code; no hidden constants.
