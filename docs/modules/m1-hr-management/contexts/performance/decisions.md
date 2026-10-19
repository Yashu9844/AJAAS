# HR / Performance & OKRs — Decisions

> Part of **M1 HR Management** (bounded context `performance`). Formerly the stand-alone "module-7-performance" bundle; module numbers in older text were renamed to the canonical M0–M12 / HR-context names.

- D7-01 (P1) HR/Performance = Performance Management & OKRs per the masterplan; the repo's Notifications scaffold becomes unnumbered `module-x-notifications` (same pattern as D5-01, D6-01).
- D7-02 (P1) Decimals (weight, progress, KR values, scores) use a hundredths fixed-point type with half-away-from-zero rounding; no floats (NFR-D002).
- D7-03 (P1) The direct manager is read from HR/Organization: the user's primary active mapping's manager_user_id, via the existing `organization.Module.MappingService()` accessor. No HR/Organization change is needed.
- D7-04 (P1) The 9-box performance axis is the manager review's overall score; the goal score is reported alongside, not blended (blending formulas are a calibration feature, out of scope).
- D7-05 (P1) Access denials on goals/reviews are 404, not 403, so existence is not disclosed (same as HR/Attendance, HR/Leave, HR/Payroll, HR/Recruitment self endpoints).
- D7-06 (P1) Cycle list/get are open to any authenticated tenant user (needed to pick a cycle when setting goals); they contain no personal data.

## Changelog

2026-10-09 — P1 DONE: HR/Performance = Performance & OKRs (D7-01); design frozen; Notifications scaffold unnumbered.
2026-10-09 — P2 DONE: models, migrations, score + lifecycle goldens.
