# Module 4 — Golden Tests (FROZEN at P1 2026-10-09)

| # | Golden behavior | Input | Expected | Location |
|---|---|---|---|---|
| G1 | Tenant isolation | tenant B uses tenant A type/request/holiday ids; A JWT on B host | 404; 403 | live tests/api/leave_rules_test.go |
| G2 | Day counting | Mon–Fri; Fri–Mon; Fri–Mon sandwich; Sat–Sun; holiday inside; optional holiday; half day | 5; 2; 4; 0 → NO_WORKING_DAYS; one fewer; counted; 0.50 | calc/calc_golden_test.go |
| G3 | Accrual | annual 12 joined 1 Jul; monthly 18 in March; monthly 15 in December (remainder); joined next year | 6.00; 4.50; 15.00; 0 | calc/calc_golden_test.go |
| G4 | Carry-forward | prev 8 limit 5; prev −2; limit 0 | 5; 0; 0 | calc/calc_golden_test.go |
| G5 | Apply reserves | apply 2 days with available 10 | pending, reserved 2, available 8, ledger reserve +2, outbox leave.applied | services + live |
| G6 | Insufficient balance | paid available 1, apply 2; unpaid apply 2 | 409 INSUFFICIENT_BALANCE; 201 | services |
| G7 | Overlap | pending Mon–Wed then Tue; first_half + second_half same day | 409 LEAVE_OVERLAP; both 201 | services + live |
| G8 | Approve → attendance | approve 2 working days | used 2, reserved 0, ledger release −2 / consume +2, Module 3 records on_leave ×2 | services + live |
| G9 | RBAC | member on lists/type writes/approve; member on self endpoints | 403; 200/201 | live |
| G10 | Self-approval | approver approves own request | 403, still pending | services + live |
| G11 | Audit + privacy | apply/approve/reject/cancel/adjust | audit rows; reason never in audit metadata or outbox payload | services + live |
| G12 | Ledger invariant | after accrual, apply, approve, cancel, adjust | balance columns == ledger sums per kind | live |
| G13 | Cancel approved future | cancel approved leave starting after today | used restored, reversal row, attendance marks cleared; started leave → 409 | services + live |
| G14 | Concurrency | 5 parallel applies of 2 days with available 5 | exactly 2 succeed, available 1 | live |
| G15 | Pagination | per_page=0, page=-1, per_page=abc | 200 clamped | controllers + live |
