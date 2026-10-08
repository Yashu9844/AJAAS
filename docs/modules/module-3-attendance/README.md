# Module 3 — Attendance, Shifts & Time Tracking

Purpose: record when employees work — shifts, shift assignments, punch IN/OUT, daily records with computed totals, regularization (timesheet correction) approvals, daily presence summary.
Responsibilities: specification.md §3 (FR-SH, FR-SA, FR-PU, FR-AR, FR-RG, FR-EV) and rules AT-001..AT-022.
Inputs / Outputs: specification.md §5 (19 REST endpoints) + FR-EV001 (5 events on `jaas.attendance.events`).
Dependencies: Module 0 (auth, RBAC, audit), Module 2 (employee profiles). See connections.md.
Consumed By: Module 4 Leave, Module 5 Payroll, Modules 10/11/12.
Contracts exposed: REST (spec §5), events (FR-EV001), in-process query service (C9), leave hook (C10, reserved).
Important files: files.md.
How to run / test: testing.md.
Current state: P1 design DONE, P2 models+migrations CURRENT — current-status.md.
Current goal: G3-2 — current-goal.md.
Known limitations: single approver (no chains), no geofence enforcement, no nightly auto-absent job, frontend pending (todo.md).
