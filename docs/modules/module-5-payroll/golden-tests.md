# Module 5 — Golden Tests (FROZEN at P1 2026-10-09)

| # | Behaviour | Input | Expected | Location |
|---|---|---|---|---|
| G1 | Tenant isolation | B reads A's run/payslip/structure; A JWT on B host | 404; 403 | integration |
| G2 | Breakdown | CTC 12,00,000/yr; BASIC 40% CTC, HRA 50% basic, SPECIAL balance | basic 40,000.00, HRA 20,000.00, special 40,000.00 | calc golden |
| G3 | Overflow | fixed BASIC 1,20,000 with CTC 6,00,000 | STRUCTURE_OVERFLOW | calc golden + service |
| G4 | Proration | 30-day month, payable 15 | each earning × 15/30, paise-exact | calc golden |
| G5 | PF | earned basic 30,000 / 10,000 | 1,800 / 1,200 | calc golden |
| G6 | ESI | gross 20,000 / 21,001 | 150 / 0; 20,123 → 151 (round up) | calc golden |
| G7 | PT | gross 25,000 / 24,999.99 | 200 / 0 | calc golden |
| G8 | TDS | annual taxable 12,00,000 / 15,00,000 / 30,00,000 | annual 0 / 1,09,200 (1,05,000 + 4% cess) → monthly 9,100 / 4,99,200 → monthly 41,600 | calc golden |
| G9 | Net cap | deductions > gross | net 0 + NET_CAPPED warning | calc golden |
| G10 | LOP from Module 4 | 2 approved unpaid days in month | lop 2, payable dim−2 | service + integration |
| G11 | Maker-checker | calculator approves | 403 SELF_APPROVAL_FORBIDDEN | service + integration |
| G12 | State machine | finalize draft / calculate approved | 409 RUN_STATE | service + integration |
| G13 | Self visibility | employee lists payslips before / after finalize | 0 / 1; other's id → 404 | integration |
| G14 | Privacy | audit + outbox after full run | no per-employee amounts | integration |
| G15 | Recalculate | change assignment, recalc | payslips replaced, totals updated | service |
| G16 | Pagination | per_page=0 etc. | 200 clamped | controllers + integration |
