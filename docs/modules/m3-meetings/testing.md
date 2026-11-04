# M3 Meetings - testing

Nothing is built yet. This is the test plan; the inventory and results are added as the steps in `plan.md` land.

## Layers

| Layer | What | Where |
|---|---|---|
| Unit | RRULE whitelist and expansion, DST, ranks, authorizer decisions, notes merge rules, iCal rendering | package `_test.go` files, fakes for ports |
| Service on Postgres | every service against a real database (`testdb`, template database per migration hash), triggers, exclusion and unique constraints | `internal/meetings/*` |
| Integration ring | router with real M0 (resource roles) and real M2 (tasks port), outbox to events, deactivation hook | `tests/integration/meetings_*_test.go`, `JAAS_REQUIRE_INTEGRATION=1` |
| Contract | swagger drift test, error envelope, 404 matrix | as M2 |
| Console | `/dev-test/meetings` scenarios (J1-J12) | `gen_console_specs.py`, `ui_walk_all.mjs` |
| Performance | PG seed (section 4 of `design.md`), p95 gates | `make perf-meetings` (to be added) |

## Golden tests (protected, `docs/ENGINEERING.md` section 8)

- `recurrence_golden_test.go`: Europe/Berlin and America/New_York DST weeks; `FREQ=MONTHLY;BYDAY=-1FR`;
  `BYMONTHDAY=31` (months without a 31st are skipped); `BYDAY=2MO`; COUNT and UNTIL boundaries; open-ended window.
- `authz_matrix_golden_test.go`: permissions by role and visibility, including auditor and confidential cases.

## Gates per step

Coverage ≥ 85% (≥ 90% on `series`, `authz` and `minutes`); `go vet`; `gofmt`; migrations up/down/up; integration ring
green; console scenario passing live; docs current.
