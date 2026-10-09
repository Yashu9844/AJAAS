# Module 5 — Assumptions

- A5-01 Module 2 `employment.joining_date` and `exit_date` define the employment window; missing joining date = employed for the whole month.
- A5-02 Payroll period is the calendar month in UTC dates.
- A5-03 Indian statutory figures (PF 12% / ₹15,000 ceiling, ESI 0.75% / ₹21,000, PT ₹200 ≥ ₹25,000, FY 2025-26 new-regime slabs) are encoded as constants with rule IDs and golden tests; changing them is a code change with a decision entry.
- A5-04 tenant_admin bypasses RBAC (Module 0) and can therefore approve — but never their own calculation (maker-checker applies to everyone).
