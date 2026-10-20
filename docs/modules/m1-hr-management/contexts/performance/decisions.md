# HR / Performance & OKRs — Decisions

> Part of **M1 HR Management** (bounded context `performance`). Formerly the stand-alone "module-7-performance" bundle; module numbers in older text were renamed to the canonical M0–M12 / HR-context names.

- D7-01 (P1) HR/Performance = Performance Management & OKRs per the masterplan; the repo's Notifications scaffold becomes unnumbered `module-x-notifications` (same pattern as D5-01, D6-01).
- D7-02 (P1) Decimals (weight, progress, KR values, scores) use a hundredths fixed-point type with half-away-from-zero rounding; no floats (NFR-D002).
- D7-03 (P1) The direct manager is read from HR/Organization: the user's primary active mapping's manager_user_id, via the existing `organization.Module.MappingService()` accessor. No HR/Organization change is needed.
- D7-04 (P1) The 9-box performance axis is the manager review's overall score; the goal score is reported alongside, not blended (blending formulas are a calibration feature, out of scope).
- D7-05 (P1) Access denials on goals/reviews are 404, not 403, so existence is not disclosed (same as HR/Attendance, HR/Leave, HR/Payroll, HR/Recruitment self endpoints).
- D7-06 (P1) Cycle list/get are open to any authenticated tenant user (needed to pick a cycle when setting goals); they contain no personal data.
- D7-07 (P5) Launch reviews is idempotent on a cycle already in `review`: it creates only missing self/manager reviews
  and does not change the status (EC-06 "a second launch creates 0"; PM-002 forbids every other repeat transition).
- D7-08 (P5) HR/Performance reads the direct manager through its own `ManagerDirectory` port; the HR root passes
  `organization.Module.DirectManager` (primary active mapping), so performance never imports HR/Organization (import-boundary test).
- D7-09 (P6) A parent goal from another tenant is 404 (PM-016); a parent in another cycle or cancelled is 400.
- D7-10 (parity) Calibration records an override next to the manager review instead of editing it, so the submitted
  review stays immutable (PM-012) and every change is auditable with a reason; the "final rating" is derived (PM-020).
- D7-11 (parity) A separate `performance:calibrate` permission: calibration changes outcomes for people outside one's
  team, so it is narrower than `performance:manage` (which runs cycles).
- D7-12 (parity) Continuous feedback is cycle-independent and visible to the recipient's direct manager; public
  visibility is limited to praise so the wall cannot carry criticism.

## Changelog

2026-10-09 — P1 DONE: HR/Performance = Performance & OKRs (D7-01); design frozen; Notifications scaffold unnumbered.
2026-10-09 — P2 DONE: models, migrations, score + lifecycle goldens.
2026-10-20 — P3–P9 DONE: backend, unit + integration goldens, swagger, console page; context complete for its v1 scope.
2026-10-20 — Parity design: calibration (PM-018..PM-022) and continuous feedback (FB-001..FB-006), goldens G16–G20.
2026-10-20 — Parity DONE: calibration + continuous feedback built and verified (33 ops in total).
