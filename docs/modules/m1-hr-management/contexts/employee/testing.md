# HR / Employee — Testing

> Part of **M1 HR Management** (bounded context `employee`). Formerly the stand-alone "module-2-employee" bundle; module numbers in older text were renamed to the canonical M0–M12 / HR-context names.

## Golden tests (protected)

Status: FROZEN 2026-10-05 (plan.md P1).

| Test ID | Name | Scenario | Expected Outcome |
|---|---|---|---|
| G1 | Multi-Tenant Isolation | Tenant B attempts GET /employees/{id_A} | 404 Not Found (no cross-tenant leakage) |
| G2 | Atomic Onboarding | Create employee with valid user_id and profile | 201 Created; Profile, Employment, Contact, Timeline all exist in DB |
| G3 | Duplicate Employee Code | Attempt to create employee with existing employee_code | 409 Conflict |
| G4 | Non-existent / Wrong Tenant User | Create employee with user_id belonging to another tenant | 404 User not found in tenant |
| G5 | Self-Service Guard | Employee updates their own address via /employees/me | 200 OK; address updated; status and code untouched |
| G6 | Privilege Escalation Block | Non-admin user attempts PATCH /employees/{id} to change status | 403 Forbidden |
| G7 | Sensitive Statutory Privacy | Regular employee:read gets profile | Bank account and Tax ID are masked |
| G8 | Sensitive Statutory Access | Admin with employee:read_sensitive queries statutory | 200 OK with unmasked bank/tax details |
| G9 | Status Transition Validation | Transition employee from probation -> active | 200 OK; confirmation_date recorded; timeline updated |
| G10 | Reactive Deactivation Sync | M0 Identity emits identity.user.deactivated | Employee status transitions to terminated/inactive; timeline recorded |

## Test strategy and commands

### Coverage & Quality Gates
- **Service Layer Coverage:** $\ge 90\%$ statement coverage on application services.
- **Package Coverage:** $\ge 80\%$ statement coverage across the module.
- **Sensors:** `go build ./...`, `go vet ./...`, `gofmt -l`, `go test ./internal/employee/...`.
- **Negative Paths:** Duplicate code, invalid status transitions, cross-tenant leaks, unverified user attachments.

## Delivery evidence (status and Definition of Done at backend close)

Implemented:
- P1 design bundle (spec, architecture, connections, goldens frozen).
- P2 Backend implementation:
  - 7 models: `EmployeeProfile`, `EmploymentDetail`, `EmployeeContact`, `EmployeeStatutory`, `EmployeeDocument`, `EmployeeTimeline`, `EmployeeEventsOutbox` + `models_test.go`.
  - 7 migrations: 000018..000024 up/down SQL pairs with foreign keys and multi-tenant indexes.
  - DTOs & Custom validators with unit tests.
  - 4 tenant-scoped repositories: `EmployeeProfileRepository`, `EmployeeStatutoryRepository`, `EmployeeDocumentRepository`, `EmployeeTimelineRepository`.
  - 5 domain services: `EmployeeService` (atomic onboarding, code uniqueness, self-service contact update, status transitions), `EmployeeStatutoryService` (field-level PII masking & unmasking), `EmployeeDocumentService` (metadata & admin verification), `EmployeeTimelineService` (career milestones), `EventConsumer` (status sync on `identity.user.deactivated`).
  - 4 controllers & 13 REST endpoints registered with RBAC (`employee:read`, `employee:create`, `employee:update`, `employee:update_sensitive`, `employee:admin`) and `AuditLog` middleware.
  - Domain events & outbox pattern: 5 produced events (`employee.created`, `employee.updated`, `employee.status.changed`, `employee.exited`, `employee.document.verified`).
  - Bootstrap wiring: `backend/cmd/main.go` automigrates models and mounts all `/api/v1/employees` routes.
  - OpenAPI 3.0.3 Swagger spec: `backend/api/swagger.yaml` updated with all employee paths.
- Sensors status:
  - `go build ./...` clean.
  - `go vet ./...` clean.
  - `gofmt -l` clean.
  - `go test ./internal/...` 100% green across all packages.
  - Service statement coverage: 79.6% on employee services, 100% on validators, models, and events.
  - Zero `TODO` comments.

Partially Implemented:
- Permissions seed: `employee:*` permissions integrated in route guards; seed runner for DB pending deploy automation.

Not Implemented:
- P3 integration tests vs live PostgreSQL/Redis/RabbitMQ containers via Docker (`tests/api/`).
- Frontend employee directory and profile components.

Blocked:
- None.

## Golden test locations (integration suite)

Golden behaviours run in `backend/tests/integration` against the production router. Older locations that mention `tests/api/*` are superseded by this table.

| ID | Test |
|---|---|
| G1 isolation | `TestTenantIsolation` |
| G2 atomic onboarding | `TestGoldenM2_AtomicOnboarding` |
| G3 duplicate code | `TestEmployeeLifecycle` |
| G4 foreign-tenant user | `TestGoldenM2_EmployeeForForeignTenantUser` |
| G5 self-service guard | `TestEmployeeSelfService` |
| G6 privilege escalation block | `TestEmployeeRBACTiers`, `TestEmployeeSelfService` |
| G7 PII masked | `TestEmployeeStatutoryAndPIIMasking` |
| G8 sensitive access | `TestEmployeeStatutoryAndPIIMasking` |
| G9 status transition | `TestEmployeeStatusStateMachine` |
| G10 deactivation sync | `TestUserDeactivationConvergesEmployee` |

## Parity evidence (2026-10-21): records and change requests

- Package `internal/hr/employee/records` (migrations hr/000054–000057, up/down/up verified).
- Unit: the service and HTTP edge are nearly fully covered. Golden suites cover contacts and the single-primary
  rule, education, prior-employment overlap, change requests (masking, one open request per type, self-approval,
  apply, cancel, reject), failure injection with rollback, and the HTTP routes and guards. The gorm store is
  covered by integration.
- Integration: `TestEmployeeRecordsAndChangeRequests` covers FR-ER001..FR-ER005 and FR-CR001..FR-CR005, including
  the applied bank change, the timeline entry, isolation and the privacy scan.
- Console: the employee page has a "Records & change requests" tab (31 generated operations) and scenario
  `hr.emp.records`.
