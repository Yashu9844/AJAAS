# Module 5 — Architecture (FROZEN at P1 2026-10-09)

```text
Gin → Module 0 TenantResolver → Authenticate → RequirePermission(payroll:read|manage|approve)   (self endpoints: auth only)
→ Controllers (structure, assignment, run, payslip)
→ Services (StructureService, AssignmentService, RunService, PayslipService, OutboxRelay)
→ calc (pure: Money paise, Breakdown(structure, monthlyCTC), Prorate, PF/ESI/PT/TDS, Payslip assembly)
→ Repositories (structure+components, assignment, run, payslip+lines, outbox)
→ PostgreSQL · RabbitMQ (jaas.payroll.events via outbox) · Module 0 Audit · Module 2 EmployeeService · Module 4 UnpaidLeave
```

Package: `backend/internal/payroll/{models,dto,validators,calc,repositories,services,controllers,routes,events}` + `module.go`; wired in `internal/app/time_modules.go` (renamed scope: people-ops modules 3–5).

## Calculation pipeline (per employee, pure)
1. `Breakdown(spec, monthlyCTC)` → full-month component amounts (PY-001) or overflow.
2. Window + LOP (service: employee dates from Module 2, unpaid days from Module 4) → payable days (PY-003/004).
3. `Prorate(amount, payable, daysInMonth)` for earnings and fixed/percent deductions (PY-002).
4. Statutory on earned figures: PF (PY-005), ESI (PY-006, eligibility from full-month gross), PT (PY-007); TDS from full-month taxable projection (PY-008).
5. Net with cap (PY-009). Output: lines + totals.

## Run state machine
draft → calculate → calculated → (calculate again) → approve (≠ calculator) → approved → finalize → finalized. One transaction per transition; calculate deletes and re-inserts payslips (PY-011).

## Concurrency
Run transitions lock the run row (`SELECT … FOR UPDATE`); assignment writes take a per-employee advisory lock `payroll:<tenant>:<employee>`.
