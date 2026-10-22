# HR / Leave & Absence — Decisions

> Part of **M1 HR Management** (bounded context `leave`). Formerly the stand-alone "module-4-leave" bundle; module numbers in older text were renamed to the canonical M0–M12 / HR-context names.

- D4-01 HR/Leave = Leave & Absence (masterplan), following D3-01. The Projects scaffold bundle was renamed to module-4-leave; Projects has no module number until the human assigns one (DEPENDENCY-GRAPH updated).
- D4-02 Ledger is the source of truth; `leave_balances` is a projection updated in the same tx (LV-013). No generated column (masterplan) because AutoMigrate cannot express it and dev/prod parity is enforced (D3-14 style).
- D4-03 Days are integer hundredths in Go (`calc.Days`), NUMERIC(7,2) in SQL. Reason: no float drift in balances (NFR-D002).
- D4-04 Leave year = calendar year of the UTC date; requests must lie in the current year. Reason: simple, matches masterplan `year INT`; cross-year requests are split by the employee. Fiscal-year config deferred.
- D4-05 Weekly off fixed Sat/Sun; one tenant holiday calendar; optional holidays count as working days. Reason: no weekly-off config exists yet (HR/Attendance has none); v1.1.
- D4-06 Lazy accrual/carry-forward on first touch instead of a scheduler. Reason: no job runner in the stack; deterministic and testable.
- D4-07 Single-tier approval by any `leave:approve` holder except the requester. Reason: M7 Approvals owns chains and manager routing; HR/Organization manager data is not exposed as a service.
- D4-08 HR/Attendance gains exported `services.LeaveSync` (MarkLeave/ClearLeave on the caller's tx) and record source `leave` — contract C10 reserved by HR/Attendance P1. Recorded in module-3 decisions.md.
- D4-09 Half-day leave does not change attendance status (attendance computes half_day from work minutes); full-day leave sets `on_leave`. Punches on a leave day win (recompute → present).
- D4-10 (P5) Accrual events (`leave.accrued`) are written to the outbox inside the materializing tx but published by the relay, not inline; request events publish inline after commit. Reason: accrual can happen inside any command (even a balance read) and must never delay or fail it.
- D4-11 (P7) Performance budgets NFR-P001/P002 were not verifiable on the dev host (Docker under memory pressure, ~70 ms/query, login 2–4 s; apply observed 881 ms). Query count per apply is bounded (lock + ≤ 10 statements + holidays). Re-measure on a normal host before production; not claimed as met.
- D4-12 (2026-10-09) Integration onto M0 Identity, HR/Organization, HR/Employee hardening: migrations renumbered to 000032–000037; wiring in internal/app/time_modules.go; errors.Normalize in controllers; live goldens ported to tests/integration/leave_test.go; RBAC golden G9 strengthened with an exact-grant user (leave:approve only).
- D4-13 (2026-10-09, from HR/Payroll D5-03) HR/Leave exposes read-only `services.UnpaidLeave.UnpaidDays` (approved requests of is_paid=false types, clipped to a range; holidays, sandwich and half days honoured) via `leave.Module.UnpaidLeave()`. No writes, no schema change.

## Changelog

2026-10-09 — P1 DONE: bundle re-scoped Projects → Leave (D4-01); full design frozen.
2026-10-09 — P2 DONE: calc.Days codec, 6 models, migrations 000032–000037 (cycle verified).
2026-10-09 — P3 DONE: calc engine (goldens G2–G4), DTOs (G15), validators — all 100%.
2026-10-09 — P4 DONE: six repositories + advisory lock.
2026-10-09 — P5 DONE: leave services, events, relay, HR/Attendance LeaveSync; goldens green; services 96.3%.
2026-10-09 — P6 DONE: HTTP edge, routes, wiring, permission seed, swagger.
2026-10-09 — P7 DONE: live goldens PASS (11 goldens), full suite 16/16, migration cycle + parity identical.
2026-10-09 — P9 DONE (backend CLOSED): DoD walked with evidence; perf caveat recorded (D4-11).
2026-10-09 — Integrated onto M0 Identity, HR/Organization, HR/Employee hardening (branch integrate/m3-m4-on-hardening): migrations renumbered 000026–000037, app wiring, error normalization, integration tests ported; 59/59 PASS.

## Parity decisions (2026-10-22)

- **DL-P1** Parity lives in `leave/plus`; the old services change in three places only, each behind a nil-safe hook:
  the leave year (`calc.AccruedFrom`/`YearOf`), the pricing calendar (`CalendarSource.For` plus `CountInput.WeekOff`) and
  the balance mover. With no calendar source nothing changes (the old goldens pass untouched).
- **DL-P2** The leave year is labelled by the calendar year in which it starts. It cannot be changed once any balance
  row exists (`YEAR_LOCKED`): re-labelling ledgers would rewrite history (LV-016). Tenants choose it before first use.
- **DL-P3** A worker's calendar is chosen by their core assignment's location on the first day of the range. A range
  that spans a transfer is priced with the starting location; this is simple, predictable and documented.
- **DL-P4** Location holidays and week-off patterns add to, never replace, the tenant calendar: blocked dates are the
  union, the weekly off is the location's pattern when it has one. Optional holidays never block.
- **DL-P5** Manager reach uses organization mappings (primary, active, `manager_user_id`), the same rule as the
  attendance inbox. The inbox routes only need a login; a stranger gets 404.
- **DL-P6** Comp-off and encashment adjust balances through `BalanceMover` inside the decision's transaction, so a
  failed adjustment rolls the decision back (LP-18). Comp-off is credited as an `adjustment` ledger row with a note
  rather than a new ledger kind, which keeps the ledger CHECK constraint and every report unchanged.
- **DL-P7** Comp-off expiry debits `min(credit, available)`, never below zero. It does not track which credit a day of
  comp-off leave consumed (no FIFO bookkeeping); in the worst case an old credit's expiry removes days that belonged to
  a newer credit, which is the safe direction for the employer and never produces a negative balance.
- **DL-P8** Encashment keeps `paid_out_at = null` until payroll consumes it; the payroll parity item (final settlement
  and encashment payout) adds the port that reads and marks approved, unpaid encashments.
