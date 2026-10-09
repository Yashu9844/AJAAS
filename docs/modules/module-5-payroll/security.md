# Module 5 — Security

Law: docs/SECURITY.md + Module 0 security.md.

## Permissions
`payroll:read` (structures, assignments, runs, any payslip, payout CSV), `payroll:manage` (structures, assignments, create/calculate runs), `payroll:approve` (approve, finalize). Self endpoints (own assignment, own finalized payslips) need authentication only.

## Data sensitivity
Salary is confidential: amounts appear only in API responses to `payroll:read` holders or the owning employee. Audit metadata holds ids, period and counts; events hold run totals and counts only (NFR-SEC003).

## Threats
| ID | Threat | Control |
|---|---|---|
| PS-T1 | Reading another employee's payslip | self endpoints resolve the employee from JWT; foreign ids → 404 (PY-013) |
| PS-T2 | Seeing unfinalized pay | self lists only finalized runs |
| PS-T3 | One person pushing a run through | maker-checker: approver ≠ calculator (PY-012) |
| PS-T4 | Tampering after approval | approved/finalized runs reject calculate (RUN_STATE); finalized immutable |
| PS-T5 | Cross-tenant IDOR | tenant-scoped repos, 404 |
| PS-T6 | Salary leakage via logs/events | whitelisted audit/event payloads; tests assert no per-employee amounts |
| PS-T7 | Rounding drift / float errors | integer paise, golden-tested statutory math |
| PS-T8 | CSV injection in payout export | cells beginning with `= + - @` are prefixed with `'` |
