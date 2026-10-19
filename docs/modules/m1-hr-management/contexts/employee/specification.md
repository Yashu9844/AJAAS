# HR / Employee — Specification

> Part of **M1 HR Management** (bounded context `employee`). Formerly the stand-alone "module-2-employee" bundle; module numbers in older text were renamed to the canonical M0–M12 / HR-context names.

Status: FROZEN 2026-10-05 (plan.md P1). P2 must implement exactly this.

### 1. Purpose

Own employee profiles, employment lifecycle records, personal/contact details, sensitive statutory/bank info (with field-level privacy), employee documents, and milestone timeline history. HR/Employee is operational workforce data linking identity principals (M0 Identity) and org hierarchy positions (HR/Organization) to real-world employment terms.

### 2. Functional Requirements

#### Employee Profiles (FR-EP)

| ID | Requirement |
|----|-------------|
| FR-EP001 | Create an employee profile in the authenticated tenant: `user_id` (1:1 with active M0 Identity User), `employee_code` (required, unique per tenant, e.g. `EMP-001`), `first_name`, `last_name`, `display_name`, `gender`, `date_of_birth`, `marital_status`, `blood_group`, `avatar_url`. |
| FR-EP002 | `employee_code` must be unique per tenant (case-insensitive) across active and soft-deleted rows. Format: `^[A-Z0-9-]{2,32}$`. |
| FR-EP003 | Get employee profile by ID (tenant-scoped, 404 otherwise). Standard response returns sanitized personal/contact data. |
| FR-EP004 | List employees paginated (`page`/`per_page`, defaults 1/20, max 100) with filters: `status`, `department_id`, `search` (name or code). |
| FR-EP005 | Update profile details (first/last/display name, gender, marital status, blood group, avatar). Employee code is immutable once set. |
| FR-EP006 | Soft delete employee (`deleted_at`); employee codes remain reserved. |
| FR-EP007 | Self-service profile endpoint `GET /employees/me` returns current user's profile and mapping snapshot. |

#### Employment Details & Lifecycle (FR-ED)

| ID | Requirement |
|----|-------------|
| FR-ED001 | Onboard employment terms: `employment_type` (`full_time`, `part_time`, `contract`, `intern`), `joining_date` (required), `probation_end_date`, `confirmation_date`, `notice_period_days` (default 30). |
| FR-ED002 | Status transitions: `active`, `probation`, `notice`, `terminated`, `resigned`, `on_leave`. Validated via state transition matrix. |
| FR-ED003 | Confirm employee (`probation` -> `active`): sets `confirmation_date`, appends timeline event `confirmed`. |
| FR-ED004 | Initiate exit (`resigned` or `notice`): sets `resignation_date`, calculates tentative `exit_date` from notice period. |
| FR-ED005 | Finalize termination/exit (`terminated` or `resigned`): sets `exit_date`, `exit_reason`, marks status `terminated`/`resigned`, automatically triggers M0 Identity user deactivation or status sync. |

#### Contact Information (FR-EC)

| ID | Requirement |
|----|-------------|
| FR-EC001 | Maintain contact info: `personal_email`, `work_phone`, `personal_phone`, `current_address`, `permanent_address`, `emergency_contacts` (JSONB array of name, relation, phone). |
| FR-EC002 | Self-service contact update (`PATCH /employees/me`): employee can update own personal phone, current address, and emergency contacts. |

#### Statutory & Bank Details (FR-ES)

| ID | Requirement |
|----|-------------|
| FR-ES001 | Store sensitive statutory information: `tax_id` (PAN / SSN / National Tax Identifier), `national_id`, `bank_name`, `bank_account_number`, `bank_routing_swift`. |
| FR-ES002 | Field-level privacy: statutory endpoints require `employee:read_sensitive` / `employee:update_sensitive`. |
| FR-ES003 | Standard employee GET masks bank account (`****1234`) and tax ID (`*****5678`) unless sensitive access is authorized. |

#### Employee Documents (FR-DOC)

| ID | Requirement |
|----|-------------|
| FR-DOC001 | Attach employee documents: `document_type` (`id_proof`, `contract`, `resume`, `education`, `certification`, `nda`), `file_name`, `file_url`, `file_size`, `mime_type`. |
| FR-DOC002 | List documents for employee (tenant-scoped). |
| FR-DOC003 | Verify document: `POST /employees/{id}/documents/{doc_id}/verify` records `verified_at` and `verified_by` user ID (requires `employee:admin`). |

#### Milestone Timeline (FR-TL)

| ID | Requirement |
|----|-------------|
| FR-TL001 | System-generated immutable timeline events: `hired`, `probation_completed`, `promoted`, `transferred`, `resigned`, `terminated`. |
| FR-TL002 | Read timeline: `GET /employees/{id}/timeline` returns chronological events. |

#### Events Consumed & Produced (FR-EV)

| ID | Requirement |
|----|-------------|
| FR-EV001 | Consume `identity.user.deactivated` -> marks employee status as `inactive`/`terminated`, appends exit timeline event. |
| FR-EV002 | Produce `employee.created`, `employee.updated`, `employee.status.changed`, `employee.exited`, `employee.document.verified` on exchange `jaas.employee.events` (topic, durable). |
| FR-EV003 | Reliable publishing via Outbox pattern (`employee_events_outbox`). |

### 3. Non-Functional Requirements

| ID | Requirement | Target |
|----|-------------|--------|
| NFR-P001 | Single-employee GET | p95 < 200ms |
| NFR-P002 | Paginated employee directory (100 items) | p95 < 500ms |
| NFR-P003 | Database single query latency | p95 < 50ms |
| NFR-SEC001 | Multi-tenant isolation on every query (`WHERE tenant_id = ?`) | Mandatory |
| NFR-SEC002 | Sensitive field masking for bank/tax PII | Mandatory |
| NFR-SEC003 | RBAC permission enforcement (`employee:read`, `employee:create`, `employee:update`, `employee:read_sensitive`, `employee:admin`) | Mandatory |

### 4. REST API Shapes & DTOs

* `POST /api/v1/employees` (201) -> `CreateEmployeeRequest`
* `GET /api/v1/employees` (200) -> `EmployeeListResponse` (Paginated)
* `GET /api/v1/employees/{id}` (200) -> `EmployeeDetailResponse`
* `PATCH /api/v1/employees/{id}` (200) -> `UpdateEmployeeRequest`
* `GET /api/v1/employees/me` (200) -> `EmployeeSelfResponse`
* `PATCH /api/v1/employees/me` (200) -> `UpdateSelfContactRequest`
* `POST /api/v1/employees/{id}/status` (200) -> `TransitionStatusRequest`
* `GET /api/v1/employees/{id}/statutory` (200) -> `StatutoryResponse` (Sensitive)
* `PUT /api/v1/employees/{id}/statutory` (200) -> `UpdateStatutoryRequest`
* `POST /api/v1/employees/{id}/documents` (201) -> `UploadDocumentRequest`
* `GET /api/v1/employees/{id}/documents` (200) -> `DocumentListResponse`
* `POST /api/v1/employees/{id}/documents/{doc_id}/verify` (200) -> `VerifyDocumentResponse`
* `GET /api/v1/employees/{id}/timeline` (200) -> `TimelineListResponse`

## Assumptions

1. **User Exists in Identity:** An employee profile must be associated with an existing, active `User` in the same tenant (M0 Identity).
2. **Document Binary Storage:** In v1, employee documents store URL references (`file_url`), sizes, and MIME types. Cloud object storage (S3/GCS/MinIO) generates pre-signed URLs or upload targets.
3. **Org Mapping Independence:** HR/Organization manages which department/team/manager an employee belongs to; HR/Employee manages the person's employment terms, contact information, and documents.
