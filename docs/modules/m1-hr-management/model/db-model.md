# M1 — HR Management — Database Model & Dependencies

**Domain scope:** Organization, Employee, Attendance, Leave, Payroll, Recruitment, Performance & OKRs

**Purpose:** Departments, teams, designations, employees, employment, attendance, leave ledger, payroll runs, recruitment pipeline, goals and reviews.

**Status:** Proposed target model. Validate against current migrations and OpenAPI before implementation.
## Common persistence conventions
- PostgreSQL is authoritative. Use UUID primary keys; tenant-owned rows include `tenant_id`.
- Include `created_at`, `updated_at`, and where applicable `created_by`, `updated_by`, `version`, `deleted_at`.
- Enforce tenant-scoped uniqueness with composite constraints such as `(tenant_id, normalized_name)`.
- Every query, cache key, export, worker and event consumer must be tenant-scoped.
- Use database constraints for invariants; use transactions for multi-row state changes.
- Durable idempotency uses a unique DB constraint; Redis is not the correctness store.
- Outbox records are committed atomically with domain changes; consumers maintain a durable inbox keyed by `(consumer_name, event_id)`.
- Do not write to another service's tables. A shared PostgreSQL cluster may be used initially, but schema/migration ownership and credentials are isolated per service.

## Proposed tables / entities

| Entity/table | Key fields (illustrative) |
|---|---|
| `Department` | department_id, tenant_id, parent_id, name, status |
| `Team` | team_id, tenant_id, department_id, name |
| `Designation` | designation_id, tenant_id, name, level |
| `Employee` | employee_id, tenant_id, user_id, employee_no, status, manager_employee_id |
| `Employment` | employment_id, employee_id, start_date, end_date, type, location |
| `EmployeeContact` | contact_id, employee_id, type, value_encrypted, is_primary |
| `StatutoryProfile` | employee_id, encrypted_fields, verification_state |
| `Shift` | shift_id, tenant_id, timezone, start_time, end_time |
| `ShiftAssignment` | employee_id, shift_id, effective_from, effective_to |
| `AttendancePunch` | punch_id, employee_id, occurred_at, source, idempotency_key |
| `DailyAttendance` | employee_id, work_date, status, minutes_worked, version |
| `LeaveType` | leave_type_id, tenant_id, code, accrual_policy |
| `LeaveRequest` | leave_request_id, employee_id, leave_type_id, from_date, to_date, status |
| `LeaveLedgerEntry` | entry_id, employee_id, leave_type_id, delta_days, source_ref, effective_at |
| `PayrollRun` | payroll_run_id, tenant_id, period, status, version |
| `PayrollLine` | payroll_run_id, employee_id, gross_minor, deductions_minor, net_minor |
| `JobOpening` | job_id, tenant_id, title, status |
| `Candidate` | candidate_id, tenant_id, job_id, stage, version |
| `CandidateStageHistory` | candidate_id, from_stage, to_stage, changed_at |
| `Offer` | offer_id, candidate_id, status, compensation_ref |
| `ReviewCycle` | cycle_id, tenant_id, period, status |
| `Goal` | goal_id, cycle_id, employee_id, weight, status |
| `KeyResult` | key_result_id, goal_id, target, current_value |
| `PerformanceReview` | review_id, cycle_id, employee_id, reviewer_id, status |
| `SagaOperation` | operation_id, tenant_id, type, aggregate_id, state, last_error |

## Relationships and invariants

Department 1:N Team; Employee reports to Employee and belongs to Department/Team via mappings. Employee 1:N Employment, ShiftAssignment, AttendancePunch, LeaveRequest, LeaveLedgerEntry and PayrollLine. LeaveLedgerEntry is append-only. PayrollRun 1:N PayrollLine. JobOpening 1:N Candidate; Candidate 1:N stage history and offers. ReviewCycle 1:N Goal/Review; Goal 1:N KeyResult. Use composite tenant constraints.

## Service dependencies

Uses M0 actor/user identity claims. References Documents for files and Approvals for workflow decisions; emits domain events for Notifications and Reports. Keep existing Organization/Employee/Attendance/Leave/Payroll/Recruitment/Performance package boundaries internally at first.

## Local reliability tables

- `idempotency_records`: unique `(tenant_id, actor_id, operation, idempotency_key)`; persist request hash, processing/completed state, stable result reference, timestamps and expiry.
- `outbox_events`: durable event ID, type/version, aggregate ID/version, tenant, correlation/causation IDs, payload, publish attempts and published timestamp.
- `inbox_events`: unique `(consumer_name, event_id)` and processing result; commit with local state changes before acknowledging broker delivery.
- `audit_events`: append-oriented business audit with actor, action, target, outcome and correlation ID.

## Suggested indexes and constraints

- Tenant-scoped unique keys for business identifiers.
- Index every hot query by `tenant_id` plus the actual filter/order fields; validate using query plans and load tests.
- Use unique constraints for idempotency, event deduplication, active assignment/decision invariants and version numbers.
- Add FK constraints within the service's owned schema; use external IDs without cross-service foreign keys.
- Use optimistic `version` checks or row locks for contested state transitions.

## Mermaid ER diagram

See matching `.mmd` file. This is a logical model, not a final migration script.
