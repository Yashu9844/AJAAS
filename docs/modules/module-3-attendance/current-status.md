# Module 3 — Current Status (updated: 2026-10-09, P4 DONE, P5 CURRENT)

Implemented:
- P1 design bundle: specification (FR-SH/SA/PU/AR/RG/EV, AT-001..AT-022, 19 endpoints, 6 tables, 5 events), architecture, connections C1–C10, security, goldens G1–G13, testing, decisions D3-01..D3-12, assumptions.
- P2: `backend/internal/attendance/models` (Shift, ShiftAssignment, AttendanceRecord, AttendancePunch, Regularization, OutboxEvent + status/type constants); models_test green, 100% coverage. Migrations 000025–000030 (up/down) incl. SQL-only partial indexes (lower(name), pending-regularization uniqueness) and CHECK constraints.
- P3: `calc` pure engine (goldens G4/G5 green), `dto` (all request/response shapes, ParsePage clamp — G13 unit), `validators` (shift code, IANA tz, dates/range, thresholds). All 100% coverage.
- P4: `repositories` — 6 interfaces + GORM impls (shift, assignment, record, punch, regularization, outbox); tenant-scoped; advisory lock; FindOrCreate; single-query status aggregate. Compile-verified; SQL behavior pending live ring (P7).

Partially Implemented:
- Migrations not yet executed against Postgres (scheduled T3-18, P7).

Not Implemented:
- services, HTTP, wiring, live tests, frontend (P5–P8).

Known Issues:
- Module 4 bundle still describes Projects; must be re-scoped to Leave (D3-01) before G4-1.
- Inherited from Module 0 (not fixed here): RequirePermission grants permissions of soft-deleted roles; `/tenants` unauthenticated. Module 3 relies on RequirePermission as-is.

Blocked:
- P8 frontend needs an identity login shell (no frontend code exists yet).

Technical Debt:
- None yet.
