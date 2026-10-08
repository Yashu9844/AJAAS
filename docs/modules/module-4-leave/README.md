# Module 4 — Leave & Absence Management

Purpose: leave types (policies), holiday calendar, ledger-backed balances with lazy accrual and carry-forward, leave requests with preview/apply/approve/reject/cancel, and approved leave reflected in attendance.
Responsibilities: specification.md §3 (FR-LT, FR-HD, FR-BL, FR-LR, FR-EV) and rules LV-001..LV-017.
Inputs / Outputs: specification.md §5 (20 REST operations) + FR-EV001 (5 events on `jaas.leave.events`).
Dependencies: Module 0 (auth, RBAC, audit), Module 2 (employee profiles), Module 3 (attendance `LeaveSync` port). See connections.md.
Consumed By: Module 3 (attendance status), Module 5 Payroll (LOP), Modules 7/10/11 (events).
Important files: files.md. How to run / test: testing.md.
Current state: current-status.md. Current goal: current-goal.md.
Known limitations: single approver (Module 6 owns chains), fixed Sat/Sun weekly off, calendar leave year, no attachments (spec §10).
