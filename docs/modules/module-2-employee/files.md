# Module 2 — File Map

## Backend Implementation Paths

### Models (`backend/internal/employee/models/`)
- `profile.go` — EmployeeProfile GORM model
- `employment.go` — EmploymentDetail model
- `contact.go` — EmployeeContact model
- `statutory.go` — EmployeeStatutory model
- `document.go` — EmployeeDocument model
- `timeline.go` — EmployeeTimeline model
- `outbox.go` — EmployeeEventsOutbox model
- `models_test.go` — Model definitions test

### Migrations (`backend/migrations/`)
- `000018_create_employee_profiles.up.sql` / `.down.sql`
- `000019_create_employment_details.up.sql` / `.down.sql`
- `000020_create_employee_contacts.up.sql` / `.down.sql`
- `000021_create_employee_statutory.up.sql` / `.down.sql`
- `000022_create_employee_documents.up.sql` / `.down.sql`
- `000023_create_employee_timelines.up.sql` / `.down.sql`
- `000024_create_employee_events_outbox.up.sql` / `.down.sql`

### DTOs & Validation (`backend/internal/employee/dto/`)
- `employee_dto.go` — Create, Update, Filter, and Response DTOs
- `statutory_dto.go` — Statutory request and masked response DTOs
- `document_dto.go` — Document upload and verification DTOs
- `timeline_dto.go` — Timeline response DTO
- `validators.go` & `validators_test.go` — Code, status, and input validators

### Repositories (`backend/internal/employee/repositories/`)
- `interfaces.go` — Repositories contracts
- `profile_repository.go` — EmployeeProfile CRUD + preloads + filters
- `employment_repository.go` — Employment details CRUD
- `contact_repository.go` — Contact info CRUD
- `statutory_repository.go` — Statutory & bank details CRUD
- `document_repository.go` — Document metadata repository
- `timeline_repository.go` — Timeline events repository

### Services (`backend/internal/employee/services/`)
- `employee_service.go` & `employee_service_test.go` — Main profile lifecycle service
- `statutory_service.go` & `statutory_service_test.go` — Sensitive data service
- `document_service.go` & `document_service_test.go` — Document vault service
- `timeline_service.go` & `timeline_service_test.go` — Milestone history service
- `event_consumer.go` & `event_consumer_test.go` — Reactive event sync

### Controllers & Routes (`backend/internal/employee/controllers/`, `routes/`)
- `employee_controller.go` & `employee_controller_test.go`
- `statutory_controller.go` & `statutory_controller_test.go`
- `document_controller.go` & `document_controller_test.go`
- `routes.go` — Endpoint routing and RBAC binding
- `module.go` — Dependency injection container
