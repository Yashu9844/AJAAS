# Module 6 — Golden Tests (FROZEN at P1 2026-10-09)

| # | Behaviour | Expected | Location |
|---|---|---|---|
| G1 | Isolation | foreign ids 404; foreign JWT 403 | integration |
| G2 | Stage graph | every allowed/forbidden RC-002 move | pipeline unit |
| G3 | Job status graph | FR-JB003 table | pipeline unit |
| G4 | Closed job | add candidate → JOB_NOT_OPEN | service |
| G5 | Duplicate email per job (case-insensitive) | 409; another job OK | service + integration |
| G6 | Stage history | each move adds an event with actor | service + integration |
| G7 | Interview gate | screening candidate → INVALID_STAGE; past time → 400 | service |
| G8 | Feedback ownership | other employee 404; completed twice 409 | service + integration |
| G9 | One open offer | second offer 409; after decline allowed | service |
| G10 | Hire | invite + employee + hired + count; filled at headcount | service + integration |
| G11 | Hire idempotency | failure after invite → retry uses the same user, one profile | service |
| G12 | Privacy | no PII in events/audit | integration |
| G13 | RBAC | member 403; interviewer self works | integration |
| G14 | Pagination | clamped | controllers + integration |
