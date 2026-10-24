# HR/Benefits — testing

## Gates

| Gate | Where |
|---|---|
| BG1 enrolment outside a window is rejected | `TestEnrolmentWindows`, `TestBenefits`, console scenario |
| BG2 one live election per worker and plan under concurrent requests | `TestBenefits` (six parallel requests, exactly one wins; a partial unique index backs the lock) |
| BG3 deductions stop after termination and return if it is rescinded | `TestTerminationHook`, `TestBenefits` (a real core terminate and rescind) |
| BG4 dependants and life events never leak | `TestDependantsAndLifeEvents`, `TestHTTP`, `TestBenefits` (a reader without `read_sensitive`, events and audit searched for the names) |
| BG5 payroll deducts exactly the elected level, and a cost change applies only from its new version | `TestDeductions`, `TestBenefits` (the payslip carries `BEN_<code>` at 1,200.00) |

## Evidence (2026-10-24)

- Migrations hr/000089–000093 (up/down/up verified).
- `benefits` 90% unit coverage; the gorm store is exercised by `TestBenefits`.
- Contract: 29 operations in `backend/api/swagger.yaml` (tag Benefits).
- Console: `/dev-test/hr/benefits` lists 29 operations; scenario `hr.benefits.flow`.
- Integration found one real defect: ending a waiver left a null level on an `ended` row, which the first version of the
  level CHECK rejected (500 when electing after a waiver). The constraint now only requires a level on active rows.
