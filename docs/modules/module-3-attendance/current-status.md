# Module 3 — Current Status (updated: 2026-10-09, P7 DONE, P9 CURRENT)

Implemented:
- P1 design bundle: specification (FR-SH/SA/PU/AR/RG/EV, AT-001..AT-022, 19 endpoints, 6 tables, 5 events), architecture, connections C1–C10, security, goldens G1–G13, testing, decisions D3-01..D3-12, assumptions.
- P2: `backend/internal/attendance/models` (Shift, ShiftAssignment, AttendanceRecord, AttendancePunch, Regularization, OutboxEvent + status/type constants); models_test green, 100% coverage. Migrations 000025–000030 (up/down) incl. SQL-only partial indexes (lower(name), pending-regularization uniqueness) and CHECK constraints.
- P3: `calc` pure engine (goldens G4/G5 green), `dto` (all request/response shapes, ParsePage clamp — G13 unit), `validators` (shift code, IANA tz, dates/range, thresholds). All 100% coverage.
- P4: `repositories` — 6 interfaces + GORM impls (shift, assignment, record, punch, regularization, outbox); tenant-scoped; advisory lock; FindOrCreate; single-query status aggregate. Verified on real Postgres by the P7 live goldens.
- P5: `services` (Shift, Assignment, Punch, Query, Regularization, OutboxRelay) + `events`; real transactions via TxRunner; outbox written in-tx, published after commit, relay retries; audit after commit (best-effort). Unit goldens G2, G3, G6, G7, G8, G10, G11, G12 green; services 91.8%, events 100%.
- P6: `controllers` (100%), `routes` (19 routes, RBAC contract test, 100%), `module.go` (Module 2 directory adapter, permission seed attendance:read|manage|approve, outbox relay, punch rate limit 30/min/IP), `cmd/main.go` wiring (AutoMigrate, seed, routes, relay start/stop), swagger (16 paths / 19 operations, 24 schemas, refs validated). Whole backend: build + vet clean, all ./internal unit tests PASS.
- P7: live ring on Docker Postgres/Redis/RabbitMQ — `tests/api/attendance_{flow,rules}_test.go` (`-tags integration`): punch flow (G2, AT-004 precedence D3-13, G11 audit, G12 outbox privacy, IP kept on punch row only), G13, regularization lifecycle (G6, G7, G8, AT-018), G9 RBAC, G1 isolation — all PASS; full tests/api suite PASS (10). Schema parity fixes (D3-14): integer columns, partial + case-insensitive shift uniqueness, pending-regularization backstop on the dev path.

Partially Implemented:
- Migrations verified live: 30 up → 6 down → 6 up on scratch DB; AutoMigrate and SQL schemas identical (columns + index definitions).

Not Implemented:
- frontend (P8, blocked on identity login shell); DoD close (P9).

Known Issues:
- Module 4 bundle still describes Projects; must be re-scoped to Leave (D3-01) before G4-1.
- Inherited from Module 0 (not fixed here): RequirePermission grants permissions of soft-deleted roles; `/tenants` unauthenticated. Module 3 relies on RequirePermission as-is.

Blocked:
- P8 frontend needs an identity login shell (no frontend code exists yet).

Technical Debt:
- None yet.
