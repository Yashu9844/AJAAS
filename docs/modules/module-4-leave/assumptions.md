# Module 4 — Assumptions

- A4-01 Module 2 `gender` is free text; applicability compares lower-case `male`/`female`; empty/other gender matches only `all`.
- A4-02 Module 2 `employment.joining_date` drives proration; missing joining date → treated as before the leave year (full entitlement).
- A4-03 "Today" for notice, backdating and accrual = UTC date (matches Module 3 default timezone).
- A4-04 tenant_admin bypasses RBAC (Module 0 behavior) and is therefore an approver.
