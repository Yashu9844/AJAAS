# JAAS Universal Architecture & Module-by-Module Technical Architecture

**Version:** 1.1 --- canonical target architecture (adopted 2026-10-20)\
**Basis:** JAAS Information Architecture v1.0, `JAAS_Universal_and_Module_Architecture.md` v1.0 (owner-supplied,
reproduced in sections 1--11 unchanged except where marked *Adopted*), the DB model & diagram bundle
(`docs/architecture/` and each `docs/modules/<mNN>/model/`), and the generated `docs/SYSTEM_REFERENCE.md`.

**Purpose:** One coherent architecture for the whole JAAS Company
Operating System, followed by service-by-service technical designs.

> **This is the single architecture authority.** Module numbering is M0 Identity & Access plus the 12 product
> modules M1--M12 below. The older "Module 1--7" labels (Organization, Employee, Attendance, Leave, Payroll,
> Recruitment, Performance) are **bounded contexts inside M1 HR Management**, never separate modules.
> Sections 12--17 record what was adopted for this repository: repository layout, HR scope, the test console
> standard, the document map, the migration status, and the access model.

## 1. Decision summary

JAAS is a multi-tenant Company Operating System with 12 user-facing
business modules, a role-personalized Dashboard, and shared platform
capabilities. The product IA defines the 12 modules and user-facing
navigation. The system reference documents the current backend
implementation for Identity & RBAC and HR-domain modules through
Performance & OKRs. These are different views of the same system and
must not be conflated.

### Target deployment model

-   **M0 Identity & Platform Access** is a separately deployable
    foundational service.
-   Each of the **12 product modules** is an independently deployable
    service: HR Management, Projects & Tasks, Meetings, Assets &
    Inventory, Finance, Reports & Analytics, Approvals, Documents, AI
    Assistant, Notifications, Company Knowledge Base, and Settings.
-   Dashboard is a frontend composition/experience, **not** a separate
    business microservice.
-   Total target: **13 domain/platform services**, plus shared
    infrastructure and the frontend/API edge.
-   A service may contain internal subdomains and packages; it does not
    need to be split into many smaller services until measured load,
    team ownership, security, or release independence justifies that
    split.
-   Keep the required technology constraints from the system reference:
    Go, Gin, GORM, PostgreSQL, REST/JSON, Clean Architecture, RabbitMQ
    (not Kafka/NATS), and Redis only for cache/session/rate-limit
    use---not as the source of truth.
-   Independently deployable services must not share write access to
    another service's tables. A shared PostgreSQL cluster is acceptable
    initially, but each service needs a distinct database/schema
    boundary, migration ownership, least-privilege credentials, and no
    cross-service writes. Separate databases/clusters can follow when
    isolation or scale warrants them.
-   This target is a **proposed architecture**, not a claim that all 13
    services currently exist. Current implementation evidence is
    summarized in Section 5.

## 2. Product boundary and source reconciliation

### 2.1 User-facing product modules

The Information Architecture specifies this root navigation:

1.  HR Management
2.  Projects & Tasks
3.  Meetings
4.  Assets & Inventory
5.  Finance
6.  Reports & Analytics
7.  Approvals
8.  Documents
9.  AI Assistant
10. Notifications
11. Company Knowledge Base
12. Settings

Dashboard is a role-personalized landing page across those modules. The
IA also contains personas, role access, dashboard variants, a screen
inventory, and 15 major user journeys; those UI concepts should drive
API and acceptance-test coverage.

### 2.2 Current backend snapshot

`SYSTEM_REFERENCE.md` describes the current backend as follows:

  -----------------------------------------------------------------------
  Current module          Current scope           Snapshot status
  ----------------------- ----------------------- -----------------------
  M0 Identity & RBAC      tenants, users, roles,  Main/hardened
                          permissions, sessions,  
                          JWT, audit              

  M1 Organization         departments, teams,     Main
                          designations, user      
                          mappings, manager, org  
                          chart                   

  M2 Employee             profiles, employment,   Main
                          contacts, masked        
                          statutory data,         
                          documents, timeline     

  M3 Attendance           shifts, assignments,    Backend closed
                          punches, daily records, 
                          regularization, summary 

  M4 Leave                types, holidays,        Backend closed
                          ledger-backed balances, 
                          requests, approvals     

  M5 Payroll              structures, CTC         Backend closed
                          assignments,            
                          maker-checker runs,     
                          statutory data,         
                          payslips, CSV           

  M6 Recruitment          jobs, candidates/stage  Backend closed
                          history, interviews,    
                          offers, hire saga       

  M7 Performance & OKRs   cycles, goals, key      In progress
                          results, check-ins,     
                          reviews, 9-box          
  -----------------------------------------------------------------------

The reference snapshot reports 144 routes plus `/health`, 50 migrations
on `main` (56 on the Module 7 branch), integration suite 63/63, and
green unit packages. Frontend test pages existed only for M0--M2; other
module folders were placeholders, with M3--M7 frontend work blocked on
the identity shell. These figures are snapshot-specific and must be
re-verified before a release.

### 2.3 Reconciliation rule

The old module numbering in older architecture notes conflicts with the
product IA and current masterplan. Use the **product names above as the
canonical navigation/service names**. Treat M0--M7 numbering as
historical/current backend implementation identifiers, not as a mandate
that user-facing module numbers match service names. Meetings,
Approvals, and Notifications scaffolds described as `module-x-*` are
partial scaffolds, not production-ready services.

*Adopted (2026-10-20):* the rows "M1 Organization ... M7 Performance & OKRs" in the table above are the HR
bounded contexts `organization`, `employee`, `attendance`, `leave`, `payroll`, `recruitment`, `performance`
of **M1 HR Management**. All documentation now uses the canonical names (`M0`--`M12`, and `HR/<Context>`
for HR contexts). The old `module-N-*` and `module-x-*` bundles, the IoT bundle (IoT is not a product module),
and the masterplan's Helpdesk module were removed; their reusable content was merged into the new bundles.

## 3. Universal architecture

### 3.1 Logical architecture

``` text
Web / Mobile clients
        |
        v
Next.js / React experience shell
  - role-personalized Dashboard
  - module navigation and screens
  - shared design system, forms, tables, command palette
        |
        v
Kong / API Gateway (TLS, routing, request size limits, coarse rate limits)
        |
        +---- Identity & Access Service (M0)
        |
        +---- HR Management Service
        +---- Projects & Tasks Service
        +---- Meetings Service
        +---- Assets & Inventory Service
        +---- Finance Service
        +---- Reports & Analytics Service
        +---- Approvals Service
        +---- Documents Service
        +---- AI Assistant Service
        +---- Notifications Service
        +---- Company Knowledge Base Service
        +---- Settings Service
        |
        +---- Health/readiness endpoints
                 |
                 +--> PostgreSQL (service-owned data and migrations)
                 +--> Redis (cache, rate limits, ephemeral coordination)
                 +--> RabbitMQ (domain events, retries, dead-letter queues)
                 +--> Object storage (document binaries, exports, attachments)
                 +--> Secrets manager / KMS
                 +--> Observability (logs, metrics, traces, alerts)
```

### 3.2 Universal request path

1.  Gateway terminates TLS, applies size limits and edge throttling, and
    forwards trusted request metadata.
2.  Service resolves tenant from the trusted host/tenant mapping; it
    must not trust a tenant ID supplied in a JSON body.
3.  Authentication verifies the short-lived access token or trusted
    internal service identity.
4.  Authorization checks the requested resource/action and object-level
    access. Hiding UI controls is not authorization.
5.  Request validation normalizes and validates the DTO; never bind a
    request directly to a GORM model.
6.  Application service runs domain rules and the use case. A pure
    domain engine handles calculations/state transitions where useful.
7.  Repository methods require tenant scope and an explicit transaction
    where multi-row consistency is needed.
8.  The transaction writes the business state, audit metadata,
    idempotency record (where applicable), and outbox event atomically.
9.  The HTTP response is returned without waiting for RabbitMQ
    availability when the operation can safely commit locally.
10. An outbox relay publishes committed events; consumers process them
    idempotently and acknowledge only after durable local processing.

### 3.3 Universal code layout

``` text
service/
  cmd/api/main.go
  cmd/worker/main.go                 # when the service has async work
  internal/
    domain/                          # entities, value objects, invariants, state machines
    application/                     # use cases, transaction orchestration
    ports/                           # interfaces to persistence, events, other services
    adapters/http/                   # Gin handlers, routes, middleware, DTO mapping
    adapters/postgres/               # GORM repositories
    adapters/rabbitmq/               # publishers and consumers
    adapters/redis/                  # cache only; never authoritative business state
    adapters/objectstorage/           # if needed
    events/                          # versioned event envelope and payload contracts
    config/
    observability/
  migrations/
  contracts/openapi/
  contracts/events/
  tests/contract/
  tests/integration/
  Dockerfile
```

This is the target service boundary. The existing
monolithic/module-oriented backend package structure can be migrated
gradually rather than renamed in one risky release.

### 3.4 API conventions

-   Base path: `/api/v1`.
-   JSON REST only; no GraphQL under current constraints.
-   Success envelope: `{ "data": ..., "meta": ... }`.
-   Error envelope:
    `{ "error": { "code": "...", "message": "...", "details": ... } }`.
-   Use stable machine-readable error codes; do not expose SQL, stack
    traces, secrets, or internal topology.
-   UUID primary keys. Use cursor pagination for high-volume collections
    where stable sorting is possible; cap page size.
-   Filtering and sorting must use allowlists, not interpolated SQL.
-   Include `X-Correlation-ID` (or create one), request/actor context,
    and trace context.
-   Mutations that can be retried require `Idempotency-Key`; updates can
    additionally use `If-Match`/version fields to prevent lost updates.
-   Return `404` for foreign-tenant resource IDs where existence
    disclosure is a concern, consistent with the current reference.
-   OpenAPI is the HTTP contract; versioned JSON schemas/AsyncAPI-like
    documentation are the event contracts. Breaking changes require a
    new version or a compatibility window.

### 3.5 Multi-tenancy and authorization

-   Every tenant-owned record includes `tenant_id`; every repository
    query, cache key, search query, export, background job, and event
    handler must preserve tenant scope.
-   Use compound uniqueness such as `(tenant_id, normalized_email)`
    rather than global uniqueness unless the product explicitly requires
    global identity.
-   Tenant ID and actor identity come from trusted middleware/service
    authentication, not client-controlled body fields.
-   Resource authorization must account for role, permission, ownership,
    manager relationship, sensitive-data flag, and workflow state as
    appropriate.
-   Cross-service calls carry tenant, actor/service identity,
    correlation ID, deadline, and authorization context. A downstream
    service still authorizes the operation; gateway authentication alone
    is not sufficient.
-   Super-admin/platform access is a separate explicit trust path,
    logged and tested independently.
-   PII, statutory identifiers, salaries, AI prompts containing
    confidential data, and document URLs require field-level
    protections, redaction, and least-privilege access.

### 3.6 Idempotency contract

For every retryable command (create, approve, pay-run finalize, hire,
assign asset, send external delivery, etc.):

1.  Client supplies `Idempotency-Key` unique for a tenant, actor,
    operation, and API version.
2.  Server canonicalizes the validated request and stores a request
    hash.
3.  An atomic unique constraint claims the key. The same key + same hash
    replays the original outcome; same key + different hash returns
    `409 IDEMPOTENCY_KEY_REUSED`.
4.  Concurrent duplicates cannot execute the side effect twice. One
    request owns the claim; other requests receive the stored result or
    a defined `409/202 in-progress` response.
5.  Store status, response code/body or result reference, created/expiry
    timestamps, and a trace/correlation ID. Retention must exceed the
    client's retry horizon and be longer for financial/high-impact
    actions.
6.  Do not depend on Redis alone for durable idempotency. Use a database
    constraint/transaction as the correctness mechanism.
7.  For asynchronous workflows, return a stable operation/resource ID
    and let retries retrieve that same operation.

### 3.7 Event delivery: outbox, inbox, retries

-   **Outbox:** write the business change and event record in one
    database transaction. A relay publishes after commit and marks
    delivery. The outbox is the source of truth until publication
    succeeds.
-   **Delivery semantics:** assume at-least-once delivery. "Exactly
    once" across a database, broker, and external provider is not
    promised.
-   **Inbox:** each consumer persists `(consumer_name, event_id)` under
    a unique constraint in the same transaction as local changes.
    Duplicate events become no-ops.
-   **Envelope:** `event_id`, `event_type`, `event_version`,
    `occurred_at`, `producer`, `tenant_id`, `correlation_id`,
    `causation_id`, and payload.
-   **Ordering:** partition logically by aggregate/entity ID when order
    matters; include aggregate version and reject or reconcile stale
    events.
-   **Retries:** bounded exponential backoff with jitter; classify
    retryable vs permanent failures; dead-letter after configured
    attempts; expose replay tooling with audit.
-   **Queue outage:** synchronous operations that can commit locally
    must not fail solely because RabbitMQ is down; the durable outbox
    retries later. Never use a no-op publisher as production durability
    unless the outbox remains durable and the fallback behavior is
    explicitly tested.
-   **External side effects:** use provider idempotency keys where
    available and a durable dispatch record for email, SMS, payments,
    webhooks, and AI tool calls.

### 3.8 Cross-service consistency and sagas

There is no cross-service ACID transaction. Use a saga/process manager
for multi-service business processes. Each step has a durable status,
timeout, retry policy, idempotency key, and compensation or
manual-recovery path.

Example: **Recruitment hire** 1. Recruitment validates offer acceptance
and claims a hire operation ID. 2. Recruitment emits
`recruitment.hire.requested`. 3. HR creates the employee in a
pending/provisioning state idempotently. 4. Identity provisions/invites
the user idempotently. 5. Documents creates onboarding document
requirements. 6. Notifications sends the invitation after provisioning
is committed. 7. Recruitment marks the candidate hired when required
confirmations arrive. 8. Failures retry safely; irrecoverable failures
leave a visible failed/pending state with a repair command. Do not
delete valid employee data automatically as a blind compensation.

### 3.9 Latency and scale

The current system reference targets: - single-resource API p95 \< 200
ms; - list API p95 \< 500 ms; - login p95 \< 300 ms; - database query
p95 \< 50 ms; - 1,000+ concurrent users per tenant; - 10,000+ tenants
and 1,000,000+ total users; - M0 target availability 99.9%.

These are source-derived baseline targets, not proof of current measured
performance. Add a universal requirement that normal interactive API
calls stay sub-second at p95, and define stricter endpoint budgets in
OpenAPI/service SLOs. Do not spend the entire budget on serial
service-to-service calls.

Design rules: - Stateless API replicas; horizontal autoscaling based on
CPU, latency, and queue lag. - Use bounded timeouts/deadlines and
cancellation on all outbound calls. - Avoid synchronous fan-out in
dashboards; use role-aware aggregated endpoints and read
models/materialized projections for non-authoritative views. - Cache
only safe, reconstructable data; tenant and permission context must be
part of cache keys. Invalidate by version/event and set explicit TTLs. -
Index tenant-scoped query patterns; inspect `EXPLAIN ANALYZE`; avoid N+1
queries; cap list size. - Separate worker pools from latency-sensitive
HTTP serving. - Use connection pools and backpressure; never allow
unbounded goroutines/queues. - Partition/archive audit/event/ledger
history as measured volume requires; keep immutable financial/audit
records subject to retention policy. - Load-test steady state, bursts,
tenant skew, cold cache, queue outage, database failover, and recovery.

### 3.10 Security, privacy, and operations

-   TLS at edge and authenticated/encrypted service-to-service traffic;
    use mTLS or short-lived workload credentials for internal calls.
-   Secrets live in a secret manager/KMS, not source control, database
    settings rows, logs, or AI prompts.
-   JWT access token 15 minutes, refresh token 7 days, bcrypt cost 12,
    reset token 1 hour, minimum password length 8, auth rate limits, and
    failed-login logging are existing reference requirements; review
    these against current security policy before release.
-   Audit state changes with actor, tenant, action, resource ID,
    outcome, correlation ID, timestamp, and minimal metadata. Never log
    passwords, access tokens, statutory values, salary payloads, or full
    sensitive documents.
-   Readiness checks include dependencies required to serve traffic;
    liveness should not fail merely because RabbitMQ is down if the API
    can still serve safely.
-   Provide structured logs, RED metrics (rate/errors/duration), queue
    lag, outbox age, DB pool saturation, cache hit ratio, saga age, and
    DLQ counts.
-   Use graceful shutdown, bounded drain time, health checks, rolling
    deployment, migration expand/contract, rollback plan, backups,
    point-in-time recovery, restore drills, and incident runbooks.
-   Feature flags are tenant-aware, auditable, and fail closed for
    security-sensitive features.

## 4. Service boundary and ownership matrix

  ------------------------------------------------------------------------
  Service                 Owns                     Must not own
  ----------------------- ------------------------ -----------------------
  M0 Identity & Access    tenants, credentials,    HR employment facts,
                          users/login identities,  payroll values, project
                          sessions, roles,         business records
                          permission catalogue,    
                          auth/audit security      
                          events                   

  HR Management           organization, employee   credentials/session
                          lifecycle/profile,       internals; generic
                          attendance, leave,       approval engine state
                          payroll, recruitment,    
                          performance/OKRs         

  Projects & Tasks        projects, milestones,    employee master record
                          tasks, boards, sprints,  or payroll
                          assignments, comments,   
                          project activity         

  Meetings                meetings, agendas,       authoritative task
                          minutes, attendance,     state; can link to task
                          action items, meeting    IDs
                          resources                

  Assets & Inventory      asset catalogue, stock,  employee master data;
                          assignment/return,       use HR references
                          maintenance, procurement 
                          request references       

  Finance                 expenses,                payroll calculation
                          reimbursements, budgets, ownership if payroll
                          vendors, cost centres,   remains HR; coordinate
                          financial transactions   explicit accounting
                          and reconciliation       boundary

  Reports & Analytics     report definitions,      authoritative writes to
                          schedules, execution     source domains
                          jobs, result metadata,   
                          read models and          
                          aggregates               

  Approvals               workflow                 the originating
                          definitions/instances,   expense/leave/payroll
                          steps, approver          record; owner service
                          assignments, decisions,  enforces final business
                          escalations, delegation  transition
                          history                  

  Documents               metadata, versions,      raw binary in
                          ACL/share grants,        relational DB unless
                          storage references,      small metadata only;
                          retention/legal hold     business facts owned by
                                                   domain services

  AI Assistant            conversations, tool      authoritative company
                          execution ledger,        facts or unrestricted
                          policy/configuration,    direct database access
                          model usage, feedback    

  Notifications           templates, preferences,  source business state
                          dispatch jobs, provider  
                          responses, retry         
                          schedule, delivery       
                          receipts                 

  Company Knowledge Base  articles, taxonomy,      source of truth for
                          versions,                HR/finance
                          approvals/publishing     transactional facts
                          status, search/index     
                          metadata                 

  Settings                tenant preferences,      secrets in plaintext;
                          feature configuration,   each domain still
                          integration              validates settings it
                          configuration,           consumes
                          configurable defaults    
  ------------------------------------------------------------------------

**HR boundary note:** current implementation separates Organization,
Employee, Attendance, Leave, Payroll, Recruitment, and Performance as
modules. The product IA groups them under one HR Management experience.
Target deployment may begin as one HR service with internal bounded
contexts and separate schemas/packages. Split into separate deployables
only if teams, scale, regulatory isolation, or release cadence justify
it. Do not force the current backend modules into seven network services
merely because they have separate packages today.

## 5. Module-by-module architecture

The API paths below are **proposed contract shapes**, not a claim that
every endpoint already exists. Existing M0--M7 endpoint definitions in
the generated reference/OpenAPI remain authoritative for implemented
behavior. Preserve existing contracts during migration unless a
versioned change is approved.

### M0. Identity & Access (platform service)

**Purpose and ownership:** tenant lifecycle, authentication,
users/roles/permissions, sessions/tokens, invitations, security audit.
It is the root dependency and must not depend on another JAAS domain
service.

**Core aggregates/data:** Tenant, UserIdentity, Credential, Role,
Permission, RolePermission, UserRole, Session/RefreshToken,
PasswordReset, Invitation, SecurityAudit, IdempotencyRecord,
OutboxEvent. UUID IDs; tenant-owned tables carry `tenant_id`;
platform-level tenant administration uses an explicit separate path.

**Representative API contract:** - `POST /api/v1/auth/login` -
`POST /api/v1/auth/refresh` - `POST /api/v1/auth/logout` -
`POST /api/v1/auth/forgot-password` -
`POST /api/v1/auth/reset-password` - `GET /api/v1/me` -
`GET/POST /api/v1/tenants` (platform-admin guarded) -
`GET/POST /api/v1/users`, `GET/PATCH /api/v1/users/{id}` -
`GET/POST /api/v1/roles`, `PATCH /api/v1/roles/{id}` -
`GET /api/v1/permissions` - `POST /api/v1/invitations`,
`POST /api/v1/invitations/{id}/accept`

**Rules:** login throttling and failed-attempt logging; access token 15
minutes, refresh token 7 days; reset token 1 hour; bcrypt cost 12 per
current reference; anti-escalation prevents granting permissions the
actor does not hold and prevents non-platform actors granting
`tenant_admin`; permission checks are server-side. Tenant/resource
foreign IDs should not leak existence.

**Events:** `identity.user.invited`, `identity.user.activated`,
`identity.user.deactivated`, `identity.role.changed`,
`identity.session.revoked`, and tenant lifecycle events. Payloads carry
IDs/status only; no credentials or tokens.

**Consistency/idempotency:** invitation creation, refresh-token
rotation, role grants, and tenant provisioning require idempotent
command behavior. Refresh rotation must detect reuse/replay. Tenant
deactivation needs an explicit cross-service lifecycle workflow; do not
assume all service rows can be atomically updated by M0.

**Scale/SLO:** login p95 \<300 ms per source; M0 99.9% availability
target. Authentication path should avoid synchronous calls to every
service. Cache permission/session data only with bounded TTL/version
invalidation and a safe revocation strategy.

**Acceptance tests:** login-refresh-logout; expired/replayed refresh
token; reset token expiry; auth rate limit; tenant isolation; role
escalation denied; platform admin isolation; audit completeness;
idempotent invitation; M0 operates without other JAAS services.

### M1. HR Management (product service; internal bounded contexts)

**Purpose:** one HR experience backed by Organization, Employee,
Attendance, Leave, Payroll, Recruitment, and Performance contexts.
Current source code has these as separate modules; deployment
consolidation/splitting is a target decision and must be handled without
cross-domain shortcuts.

**Subdomains and data:** - Organization: Department, Team, Designation,
reporting/manager mapping, org chart. - Employee: EmployeeProfile,
Employment, Contact, masked StatutoryProfile, EmployeeDocumentReference,
EmploymentTimeline. - Attendance: Shift, ShiftAssignment, Punch,
DailyAttendance, RegularizationRequest, AttendanceSummary. - Leave:
LeaveType, HolidayCalendar, LeaveRequest, LeaveBalanceLedger,
LeaveApprovalReference. - Payroll: PayStructure, CTC/SalaryAssignment,
PayrollRun, PayrollLine, StatutoryCalculation, PayslipReference, export
job. - Recruitment: Job, Candidate, StageHistory, Interview, Feedback,
Offer, HireSaga. - Performance: ReviewCycle, Goal, KeyResult, CheckIn,
Review, Rating/9-box placement.

**Representative API contract:** - `GET/POST /api/v1/hr/departments`,
`GET/PATCH /api/v1/hr/departments/{id}` - `GET /api/v1/hr/org-chart` -
`GET/POST /api/v1/hr/employees`, `GET/PATCH /api/v1/hr/employees/{id}` -
`GET /api/v1/hr/employees/{id}/timeline` -
`POST /api/v1/hr/attendance/punches`,
`GET /api/v1/hr/attendance/daily` -
`POST /api/v1/hr/attendance/regularizations`,
`POST /api/v1/hr/attendance/regularizations/{id}/decision` -
`GET /api/v1/hr/leave/balances/me`, `POST /api/v1/hr/leave/requests`,
`POST /api/v1/hr/leave/requests/{id}/decision` -
`POST /api/v1/hr/payroll/runs`,
`POST /api/v1/hr/payroll/runs/{id}/calculate`,
`POST /api/v1/hr/payroll/runs/{id}/approve`,
`POST /api/v1/hr/payroll/runs/{id}/finalize` -
`POST /api/v1/hr/recruitment/jobs`,
`POST /api/v1/hr/recruitment/candidates`,
`POST /api/v1/hr/recruitment/offers`,
`POST /api/v1/hr/recruitment/hire` -
`POST /api/v1/hr/performance/cycles`,
`POST /api/v1/hr/performance/goals`,
`POST /api/v1/hr/performance/check-ins`,
`POST /api/v1/hr/performance/reviews`

**Critical invariants from current reference:** - Organization chart
cache has 60-second TTL; ≤5k nodes p95 \<800 ms uncached and \<100 ms
cached. - Employee GET p95 \<200 ms; directory list of 100 items p95
\<500 ms. - Attendance/leave/payroll concurrency uses employee-scoped
locks where required; leave balances are ledger-backed; unpaid leave
informs payroll loss-of-pay; payroll runs use maker-checker; recruitment
has stage history and a hire saga; performance goal weights per
employee/cycle sum to at most 100 and state transitions are
constrained. - Monetary values use exact decimal/paise representations,
not floating point. Leave days and performance scores use explicit fixed
precision. - Sensitive statutory fields require separate permission
flags and must be masked by default. - History/ledger-like records are
append-only; corrections reverse/supersede rather than silently
overwrite.

**Events:** employee created/updated/status changed/exited/document
verified; organization changes; attendance regularized; leave
requested/approved/cancelled; payroll run calculated/approved/finalized;
recruitment stage/offer/hire events; performance cycle/goal/review
events. Standard envelope and outbox apply.

**Saga examples:** hire → employee profile → identity invite → document
checklist → notification; approved leave → attendance marking → payroll
LOP projection. Use durable process state and idempotent handlers.

**Acceptance tests:** each existing module's golden rules; cross-context
transaction boundaries; concurrent punches/leave balance/payroll
finalization; maker-checker separation; duplicate hire request; PII-free
events/audit; manager/owner permissions; exact money and rounding;
tenant isolation; reconciliation for incomplete sagas.

### M2. Projects & Tasks

**Purpose:** collaborative execution and delivery management.

**Data model:** Project, ProjectMember, Milestone, Board, Column/Status,
Task, TaskDependency, TaskAssignee, Comment, AttachmentReference,
Sprint, Label, ActivityEvent, TimeEntry (if approved in product scope).
Employee identity is referenced by stable user/employee IDs; do not
duplicate HR profile facts.

**Representative APIs:** - `GET/POST /api/v1/projects` -
`GET/PATCH /api/v1/projects/{projectId}` -
`POST /api/v1/projects/{projectId}/members` -
`GET/POST /api/v1/projects/{projectId}/tasks` -
`GET/PATCH /api/v1/tasks/{taskId}` -
`POST /api/v1/tasks/{taskId}/comments` -
`POST /api/v1/tasks/{taskId}/dependencies` -
`POST /api/v1/projects/{projectId}/sprints` -
`POST /api/v1/sprints/{id}/start` and `/complete`

**Rules:** project membership and tenant boundary; legal status
transitions; cycle/dependency detection; optimistic concurrency on task
edits; authorization on comments/attachments; soft-delete/archive
policy; stable activity history.

**Events:** `project.created`, `project.member.changed`, `task.created`,
`task.updated`, `task.assigned`, `task.completed`, `sprint.started`,
`sprint.completed`. Events reference IDs and versions; Notifications
consumes assignment/due-date events.

**Latency/scale:** paginated task queries with indexes
`(tenant_id, project_id, status, updated_at)` and
`(tenant_id, assignee_id, due_at)` where workload confirms; avoid
loading full task history in board view; board summaries can be
materialized/cached with version invalidation.

**Acceptance tests:** duplicate task creation replay; concurrent status
updates; dependency cycles rejected; unauthorized project membership
denied; task event replay; large board pagination; attachment ACL;
tenant-isolated search.

### M3. Meetings

**Purpose:** plan meetings and capture decisions/action items.

**Data model:** Meeting, Participant, AgendaItem, MinutesVersion,
AttendanceRecord, ActionItemReference, MeetingResource, RecurrenceRule,
MeetingChangeLog.

**Representative APIs:** - `GET/POST /api/v1/meetings` -
`GET/PATCH /api/v1/meetings/{id}` -
`POST /api/v1/meetings/{id}/participants` -
`POST /api/v1/meetings/{id}/agenda-items` -
`PUT /api/v1/meetings/{id}/minutes` -
`POST /api/v1/meetings/{id}/publish-minutes` -
`POST /api/v1/meetings/{id}/action-items`

**Rules:** timezone-aware start/end times; recurrence expansion bounded;
meeting visibility distinct from participant visibility; minutes version
history; action items should link to task IDs and must not duplicate
task truth in the Meetings service.

**Events:** meeting scheduled/updated/cancelled, participant changed,
minutes published, action item created. Notifications handles reminders
and changes.

**Acceptance tests:** timezone/DST cases; duplicate create retry;
recurrence bounds; private meeting ACL; minutes versioning; event
replay; action item link consistency.

### M4. Assets & Inventory

**Purpose:** track equipment, inventory, assignment, return, and
maintenance.

**Data model:** AssetType, Asset, StockItem, Location, StockMovement,
AssetAssignment, ReturnInspection, MaintenanceTicket,
PurchaseRequestReference, AuditHistory.

**Representative APIs:** - `GET/POST /api/v1/assets` -
`GET/PATCH /api/v1/assets/{id}` - `POST /api/v1/assets/{id}/assign` -
`POST /api/v1/assets/{id}/return` -
`POST /api/v1/assets/{id}/maintenance` - `GET /api/v1/inventory/stock` -
`POST /api/v1/inventory/movements`

**Rules:** serialized assets have one active assignment unless
explicitly modeled otherwise; stock cannot go negative without an
approved exception; every stock change is a ledger/movement;
assignment/return commands are idempotent; employee references resolve
through HR IDs without direct HR table access.

**Events:** asset registered, assigned, returned, maintenance status
changed, stock threshold breached. Notifications can notify assignees
and custodians.

**Acceptance tests:** two concurrent assignments cannot claim one asset;
duplicate movement key is replayed; stock invariants; tenant/location
scope; lost/stolen state workflow; audit trail.

### M5. Finance

**Purpose:** expense claims, reimbursements, budgets, vendors, cost
centres, and financial records. Decide and document whether statutory
payroll accounting remains in HR Payroll or exports into Finance;
Finance must not recalculate payroll.

**Data model:** Vendor, CostCentre, Budget, BudgetPeriod, ExpenseClaim,
ExpenseLine, ReceiptReference, Reimbursement, FinancialTransaction,
ReconciliationBatch, ApprovalReference, ExportJob.

**Representative APIs:** - `GET/POST /api/v1/finance/vendors` -
`GET/POST /api/v1/finance/budgets` - `POST /api/v1/finance/expenses` -
`POST /api/v1/finance/expenses/{id}/submit` -
`POST /api/v1/finance/expenses/{id}/decision` -
`POST /api/v1/finance/reimbursements/{id}/mark-paid` -
`POST /api/v1/finance/exports`

**Rules:** exact decimal currency; explicit currency and rounding;
separation of submitter/approver/payer; duplicate receipt/claim
detection as a risk control; immutable posted transactions with reversal
entries; period close prevents silent backdating; approval service owns
workflow decisions while Finance owns the expense/reimbursement state.

**Events:** expense submitted/approved/rejected, reimbursement
initiated/paid, budget threshold exceeded, reconciliation completed.

**Acceptance tests:** duplicate payment command; same idempotency key
with changed amount rejected; maker-checker; ledger balancing where
double-entry is in scope; export reproducibility; tenant and cost-centre
permissions.

### M6. Reports & Analytics

**Purpose:** provide dashboards, scheduled reports, operational metrics,
and cross-module insights without becoming the source of transactional
truth.

**Data model:** ReportDefinition, ReportVersion, Schedule, Execution,
ResultArtifactReference, DataSourceGrant, ReadModelVersion,
RefreshCheckpoint.

**Representative APIs:** - `GET/POST /api/v1/reports/definitions` -
`POST /api/v1/reports/definitions/{id}/run` -
`POST /api/v1/reports/definitions/{id}/schedule` -
`GET /api/v1/reports/executions/{id}` -
`GET /api/v1/analytics/dashboard` -
`GET /api/v1/analytics/metrics/{metric}`

**Rules:** explicit allowed data sources and field-level permissions;
tenant-scoped read models; report runs are async for expensive queries;
signed, short-lived artifact links; query budgets and export row limits;
distinguish freshness from correctness. Use event-fed projections or
scheduled extracts instead of live synchronous fan-out for every chart.

**Events:** report requested/completed/failed; projection refreshed;
schedule due. Notifications sends completion notices.

**Acceptance tests:** tenant-isolated aggregates; stale projection
indicator; export permission check; report job retry idempotency;
expensive query cancellation; dashboard p95 budget under realistic
fan-out.

### M7. Approvals

**Purpose:** reusable workflow engine for routing, decisions,
escalation, delegation, and workflow history. The source business
service owns the business record and validates final state transitions.

**Data model:** WorkflowDefinition, WorkflowVersion, WorkflowInstance,
StepInstance, Assignment, Decision, Delegation, EscalationTimer,
WorkflowAudit, IdempotencyRecord.

**Representative APIs:** - `GET/POST /api/v1/approval-workflows` -
`POST /api/v1/approvals/instances` - `GET /api/v1/approvals/inbox` -
`POST /api/v1/approvals/{id}/approve` -
`POST /api/v1/approvals/{id}/reject` -
`POST /api/v1/approvals/{id}/delegate` -
`POST /api/v1/approvals/{id}/recall` (only where policy permits)

**Rules:** version workflow definitions; an active instance is pinned to
a definition version; decisions are immutable; maker cannot approve own
high-risk request unless explicit policy allows; decision command
idempotent; stale concurrent decisions return conflict; timers are
durable and safe to replay.

**Events:** approval requested, step assigned, approved, rejected,
escalated, delegated, completed. Origin service consumes completion and
revalidates its own business invariants.

**Acceptance tests:** simultaneous approve/reject; duplicate approve
replay; unauthorized approver; delegation loop prevention; workflow
version change mid-flight; timer replay; originating record changed
before finalization.

### M8. Documents

**Purpose:** document metadata, versions, sharing, retention, and secure
references to binary objects.

**Data model:** Document, DocumentVersion, StorageObjectReference,
DocumentACL, ShareGrant, ScanResult, RetentionPolicy, LegalHold,
AccessAudit.

**Representative APIs:** - `POST /api/v1/documents/upload-sessions` -
`POST /api/v1/documents` - `GET /api/v1/documents/{id}` -
`POST /api/v1/documents/{id}/versions` -
`POST /api/v1/documents/{id}/share-grants` -
`POST /api/v1/documents/{id}/revoke-share` -
`POST /api/v1/documents/{id}/retention-hold`

**Rules:** binaries live in object storage, not ordinary relational
rows; upload/download through short-lived signed URLs; malware scan
before availability; version immutability; object ACL and application
ACL must agree; signed links never appear in logs/events; enforce
retention/legal hold and revocation.

**Events:** document
uploaded/scanned/verified/versioned/shared/revoked/deleted according to
retention policy. Domain services own business meaning (e.g., "employee
contract"), Documents owns file lifecycle and ACL.

**Acceptance tests:** cross-tenant IDOR; expired/revoked signed link;
scan failure quarantine; duplicate upload session; retention/hold
conflict; access audit; version integrity.

### M9. AI Assistant

**Purpose:** conversational interface and governed tool execution across
JAAS. It is not an alternate authorization path or a source of truth.

**Data model:** Conversation, Message, AssistantRun, ToolCall,
ToolResultReference, PromptTemplateVersion, ModelUsage, UserFeedback,
PolicyDecision, RedactionRecord.

**Representative APIs:** - `POST /api/v1/ai/conversations` -
`GET /api/v1/ai/conversations/{id}` -
`POST /api/v1/ai/conversations/{id}/messages` -
`GET /api/v1/ai/runs/{id}` - `POST /api/v1/ai/runs/{id}/cancel` -
`GET /api/v1/ai/usage`

**Rules:** every tool call is an allowlisted operation with a schema,
deadline, permission check, tenant/actor context, audit record, and
idempotency behavior. The model never receives database credentials or
unrestricted SQL. Treat retrieved documents and tool output as untrusted
data, defend against prompt injection, minimize PII, apply retention
policy, and require confirmation for high-impact actions (payroll
finalize, user/role changes, external sends). Separate read-only answers
from write-capable actions.

**Events:** assistant run started/completed/failed, tool action
requested/completed, usage threshold reached. Do not publish raw prompts
or sensitive tool payloads by default.

**Acceptance tests:** unauthorized tool invocation blocked; prompt
injection cannot bypass tool ACL; duplicate tool call safe;
cancellation/deadline enforced; sensitive fields redacted; tenant data
never crosses conversations; cost/usage quotas enforced.

### M10. Notifications

**Purpose:** durable dispatch of email, in-app, push, SMS or other
enabled channels, with templates, preferences, retries, and delivery
receipts.

**Data model:** NotificationTemplate, TemplateVersion, UserPreference,
DispatchJob, DispatchAttempt, ProviderReceipt, SuppressionEntry,
DeadLetterRecord.

**Representative APIs:** - `GET/POST /api/v1/notification-templates` -
`GET/PATCH /api/v1/notification-preferences/me` -
`GET /api/v1/notifications/inbox` -
`POST /api/v1/notifications/{id}/read` -
`POST /api/v1/notifications/{id}/retry` (privileged/admin only)

**Rules:** accept event/command with a stable notification key;
de-duplicate dispatch; respect preferences and legal/critical-message
overrides; provider retries are bounded; template rendering escapes
untrusted content; delivery state is distinct from business success;
provider credentials in secret manager.

**Events:** notification queued/sent/delivered/bounced/failed. The
current reference already describes a notification service consuming
identity email events; keep that contract during migration.

**Acceptance tests:** duplicate event does not send twice when provider
idempotency allows; retry/backoff; invalid address handling;
unsubscribe/preferences; template injection; DLQ replay; provider outage
does not block source API.

### M11. Company Knowledge Base

**Purpose:** versioned company articles, policies, taxonomy, publication
workflows, and searchable knowledge.

**Data model:** Article, ArticleVersion, Category, Tag, AudienceRule,
ReviewAssignment, PublicationState, SearchIndexRecord, Feedback.

**Representative APIs:** - `GET /api/v1/knowledge/articles` -
`POST /api/v1/knowledge/articles` -
`GET/PATCH /api/v1/knowledge/articles/{id}` -
`POST /api/v1/knowledge/articles/{id}/submit-review` -
`POST /api/v1/knowledge/articles/{id}/publish` -
`POST /api/v1/knowledge/search`

**Rules:** immutable published versions; role/department/tenant ACL
enforced at retrieval time, not only at indexing; draft vs published
content separated; reindex events idempotent; stale index should not
leak restricted text. Link documents through Documents service
references.

**Events:** article created/updated/submitted/published/archived; index
refresh requested/completed.

**Acceptance tests:** unpublished article absent from ordinary search;
ACL-filtered search; duplicate indexing event; version history; stale
index access blocked; tenant-specific policy visibility.

### M12. Settings

**Purpose:** tenant preferences, feature configuration, integrations,
configurable workflow defaults, and environment-level feature switches.

**Data model:** TenantSetting, FeatureFlag, IntegrationConfig,
WebhookSubscription, ConfigVersion, ChangeApproval, SecretReference.

**Representative APIs:** - `GET/PATCH /api/v1/settings/tenant` -
`GET/POST /api/v1/settings/integrations` -
`POST /api/v1/settings/integrations/{id}/test` -
`GET/POST /api/v1/settings/webhooks` -
`GET/PATCH /api/v1/settings/features` - `GET /api/v1/settings/audit`

**Rules:** typed/validated settings schema; versioned changes; secrets
are references into a secret manager, never plaintext values; high-risk
settings require approval/re-authentication; consumers fail safely when
settings are missing/invalid; tenant flags cannot grant permissions or
bypass service authorization.

**Events:** setting changed, integration connected/disconnected, feature
flag changed, webhook delivery failed.

**Acceptance tests:** tenant config isolation; invalid config rejected;
secret never returned in GET/log; optimistic concurrency; high-risk
setting audit/approval; webhook secret rotation.

## 6. Dashboard and frontend architecture

Dashboard is a frontend composition layer, not a thirteenth product
service. It should call a purpose-built, role-aware aggregation endpoint
or consume a materialized read model for common cards. Avoid serial
calls to every service on page load.

-   Frontend: Next.js 16 / React 19 per current reference; establish
    identity shell and design system first.
-   Shared shell: tenant selection where allowed, navigation, user menu,
    global search/command palette, notification center, breadcrumbs,
    error boundary, accessible forms/tables, feature flags.
-   Route guards improve UX but backend permissions remain
    authoritative.
-   Dashboard response should return independently loadable widgets,
    each with `status`, `updated_at`, and data; one failed widget must
    not fail the whole dashboard.
-   Use role/persona-based card definitions; filter data server-side.
-   Use cursor pagination and optimistic updates only where conflict
    recovery is clear.
-   Screen inventory and 15 user journeys from the IA become frontend
    E2E journeys and service contract scenarios.

## 7. Data ownership and integration rules

1.  No service writes another service's tables, even if the services
    initially share one PostgreSQL cluster.
2.  Cross-service reads use APIs for request-time authoritative checks
    or event-fed read models for dashboards/reporting.
3.  Never build a synchronous dependency chain across multiple services
    for a basic write.
4.  Use stable IDs and versioned contracts; avoid copying mutable HR
    facts into other services.
5.  Each service owns migrations, indexes, retention, backup/recovery
    requirements, and schema compatibility.
6.  Use explicit data classification and retention for PII, payroll,
    documents, AI conversations, and audit records.
7.  Build data export/deletion workflows that respect legal retention
    and audit requirements.

## 8. Migration plan from current backend to target microservices

Do not rewrite the platform in one cutover. Use a strangler migration
with compatibility adapters and a stable public API.

### Phase 0 --- Inventory and contract freeze

-   Verify current branch/commit, route list, migrations, module status,
    OpenAPI, event names, and integration test state.
-   Treat generated `SYSTEM_REFERENCE.md` as a snapshot; regenerate it
    from the target commit.
-   Create a canonical product-module-to-backend-context mapping and
    resolve legacy module numbering.
-   Publish OpenAPI and event schemas; classify every existing route as
    keep, adapt, deprecate, or replace.
-   Baseline latency, throughput, error rate, DB pool use, queue lag,
    and tenant skew.

**Exit gate:** every current endpoint and table has an owner; no
undocumented cross-module write remains.

### Phase 1 --- Universal safety primitives

-   Standardize request/actor/tenant/correlation context, API envelopes,
    error codes, health/readiness, metrics/tracing, and audit
    conventions.
-   Add durable idempotency records to high-impact mutation paths.
-   Verify transactional outbox/inbox and DLQ/replay behavior.
-   Enforce tenant isolation in repositories, caches, exports,
    background jobs, and event consumers.
-   Add service contract tests and migration up/down/up tests.

**Exit gate:** failure-injection tests prove no duplicate
money/hire/approval/asset side effects and no lost committed events.

### Phase 2 --- Identity shell and edge

-   Stabilize M0 independently; define gateway routes, service identity,
    internal auth, permission catalogue/versioning, and revocation
    behavior.
-   Finish frontend identity shell and common navigation before building
    all module UIs.
-   Introduce per-service config, secrets, health checks, deployment
    pipeline, dashboards and alerts.

**Exit gate:** each service can authenticate/authorize requests without
importing another service's internal packages.

### Phase 3 --- Establish HR service boundary

-   First preserve existing M1--M7 packages and contracts behind an HR
    service facade.
-   Replace direct cross-module package calls with ports; then convert
    in-process calls to API/events only where independent deployment is
    required.
-   Split database ownership by schema/credentials/migrations before
    separate deployments.
-   Preserve exact money, ledger, lock, maker-checker, append-only
    history, and hire-saga rules.
-   Keep existing endpoint compatibility through gateway routing and
    adapters.

**Exit gate:** HR journeys pass end-to-end and all module-specific
golden tests still pass.

### Phase 4 --- Extract shared workflow and document capabilities

-   Build Approvals and Documents as real services; migrate partial
    scaffolds only after contract and data ownership are explicit.
-   Move approval state to Approvals, while source modules continue to
    own business records.
-   Move document metadata/version/ACL to Documents; migrate binary
    storage separately with checksums and ACL verification.

**Exit gate:** leave/expense/payroll approval journeys and employee
document access work with no cross-service DB writes.

### Phase 5 --- Extract collaboration and operations modules

-   Build Projects & Tasks, Meetings, Assets & Inventory, Finance,
    Notifications, and Settings using the same template.
-   Migrate one workflow at a time with dual-read or shadow comparison
    where appropriate; avoid dual writes unless there is a documented
    reconciliation mechanism.
-   Route writes to one owner; compare old/new read results before
    switching.

**Exit gate:** module contract tests, tenant isolation, latency SLOs,
and rollback procedure pass for each extraction.

### Phase 6 --- Reports, Knowledge Base, AI

-   Build event-fed analytics/read models; do not query every
    operational database directly.
-   Establish Knowledge Base versioning and ACL-filtered search.
-   Enable AI tools only after per-tool authorization, audit, quotas,
    prompt/data handling, confirmation gates, and safe retry behavior
    exist.

**Exit gate:** report freshness is visible, search respects ACLs, and AI
cannot bypass underlying service permissions.

### Phase 7 --- Hardening and cutover

-   Run load, soak, failover, restore, queue outage, partial network
    outage, and tenant hotspot tests.
-   Canary by tenant or percentage; observe error budget and latency
    before increasing traffic.
-   Verify migration rollback/forward repair and data reconciliation.
-   Remove legacy routes only after usage telemetry proves safe
    deprecation.

**Exit gate:** production SLOs hold during peak load and failure drills;
runbooks and owners are on call.

## 9. Universal acceptance criteria ("system design checklist")

The user previously referenced a specific "100 system design checklist,"
but the checklist's actual 100 items were not present in the source
material retrieved for this document. The following is a **proposed
100-point acceptance checklist**, not a transcription of that missing
list. Replace or reconcile it with the user's canonical checklist before
treating it as the final governing standard.

### Architecture and ownership

1.  Every service has a written purpose and owner.
2.  Each service owns its business data.
3.  No cross-service table writes exist.
4.  Service dependencies are documented.
5.  No circular synchronous dependencies exist.
6.  Product IA maps to service ownership.
7.  Dashboard is not treated as a business domain.
8.  API and event contracts are versioned.
9.  Service extraction has a rollback plan.
10. Architecture decisions are recorded.

### API and contracts

11. REST/JSON conventions are consistent.
12. OpenAPI exists for every service.
13. Request DTOs are separate from persistence models.
14. Error codes are stable and documented.
15. Pagination has a maximum page size.
16. Sort/filter inputs are allowlisted.
17. Correlation IDs propagate end-to-end.
18. Timeouts and request size limits are enforced.
19. Breaking changes use versioning/compatibility windows.
20. Contract tests run in CI.

### Idempotency and consistency

21. Retryable mutations require idempotency keys.
22. Key scope includes tenant, actor, operation and API version.
23. Request hash prevents key reuse with different payload.
24. Atomic uniqueness prevents concurrent duplicate execution.
25. Replay returns the original stable outcome.
26. Durable records---not Redis alone---enforce correctness.
27. Consumer inbox deduplicates event IDs.
28. Business update and outbox event commit atomically.
29. Queue outage does not lose committed events.
30. External side effects use provider idempotency or durable dispatch.
31. Saga steps have durable state.
32. Saga retries are bounded and observable.
33. Compensation/manual recovery is defined.
34. Optimistic concurrency prevents lost updates.
35. High-risk workflows have race-condition tests.

### Tenancy and security

36. Every tenant-owned row has tenant scope.
37. Every repository query scopes by tenant.
38. Cache keys include tenant and permission-sensitive context.
39. Search and exports enforce tenant boundaries.
40. Event consumers verify tenant context.
41. Tenant IDs are never trusted from request bodies.
42. Object-level authorization is enforced.
43. Role escalation is prevented.
44. Sensitive fields are masked by default.
45. Secrets are stored in a secret manager.
46. Tokens/passwords never enter logs.
47. Internal services authenticate each other.
48. Rate limits cover auth and expensive endpoints.
49. Failed auth attempts are recorded.
50. Security audit is tamper-resistant/appropriately protected.

### Data and storage

51. UUID primary keys are used where required.
52. Multi-table writes use transactions.
53. Monetary math avoids floating point.
54. Ledger/history records are append-only where required.
55. State machines reject illegal transitions.
56. Uniqueness constraints enforce domain invariants.
57. Foreign keys/indexes match access patterns.
58. Migrations are deterministic and owned by service.
59. Migration up/down/up is tested where rollback is supported.
60. Expand/contract migration supports rolling deployment.
61. Retention/deletion policy exists for each data class.
62. Backups are monitored.
63. Restore drills are run.
64. Large exports are asynchronous and bounded.
65. Object storage ACLs align with application ACLs.

### Performance and scale

66. Single-resource p95 is under 200 ms target.
67. List p95 is under 500 ms target.
68. Login p95 is under 300 ms target.
69. DB query p95 is under 50 ms target.
70. Normal interactive APIs remain sub-second at p95.
71. Load test covers 1,000 concurrent users per tenant target.
72. Scale test covers 10,000 tenants / 1 million users design target.
73. N+1 queries are tested/observed.
74. Query plans are reviewed for hot paths.
75. Pagination and result limits prevent unbounded reads.
76. Cache TTL/invalidation behavior is documented.
77. Queue lag and outbox age are monitored.
78. Worker concurrency has backpressure.
79. Connection pools are bounded.
80. Dashboards avoid serial cross-service fan-out.

### Resilience and operations

81. Liveness and readiness are distinct.
82. Dependency timeouts are enforced.
83. Retries use bounded exponential backoff and jitter.
84. Circuit breakers/bulkheads protect critical paths where needed.
85. DLQs have alerting and replay controls.
86. Graceful shutdown drains work safely.
87. Rolling deployment is supported.
88. Canary/feature-flag rollout is supported.
89. Runbooks exist for DB, broker, cache and provider outage.
90. Incident metrics and error budgets are defined.

### Observability, testing, and delivery

91. Structured logs include correlation IDs.
92. RED metrics exist per endpoint.
93. Distributed traces cover service boundaries.
94. Sensitive data is redacted in telemetry.
95. Unit tests cover domain rules.
96. Integration tests cover database and broker behavior.
97. Failure injection covers repository/provider failures.
98. Coverage target exceeds 80% for service layer per current reference.
99. CI runs security, contract, migration, and load-smoke checks.
100. Every release has a verified rollback/reconciliation plan.

## 10. Required deliverables before implementation sign-off

1.  Canonical product-module/service map.
2.  Context map and data ownership matrix.
3.  OpenAPI contract for every service.
4.  Event catalogue with schema versions and consumer matrix.
5.  ERD and migration ownership per service.
6.  Idempotency table schema and API contract.
7.  Outbox/inbox schema and relay/consumer runbooks.
8.  Saga state diagrams for hire, approvals, reimbursements/payroll, and
    asset assignment where multi-service.
9.  Per-service SLO and load-test report.
10. Threat model and data classification.
11. Deployment topology, secrets, backup/restore and disaster-recovery
    plan.
12. Screen-to-API map for the IA's screen inventory and 15 user
    journeys.
13. Reconciled, authoritative version of the user's 100-point system
    design checklist.

## 11. Explicit unresolved decisions

-   Is HR Management deployed initially as one service with bounded
    contexts, or are attendance/leave/payroll/recruitment/performance
    independently deployed from day one? Recommendation: start with a
    single HR deployment boundary and internal bounded contexts,
    preserve current module seams, then split based on measured needs.
-   What is the authoritative "100 system design checklist"? The exact
    list was not found in the source snapshot. The checklist in Section
    9 is a proposed replacement only.
-   What are the exact 9 personas and all 15 journey names? The
    Information Architecture declares them, but this architecture draft
    has not reproduced every row from the source HTML. Before screen/API
    sign-off, map every original persona, role permission, screen, and
    journey without omission.
-   Which object-storage, search, AI model/provider, payment/finance
    provider, and notification channels are approved? These are
    integration decisions, not assumed facts.
-   What availability SLOs apply to services other than M0? Set them by
    business criticality before production.

*Adopted answers (2026-10-20):*

| Question | Answer |
|---|---|
| HR deployment shape | **One HR Management service** with internal bounded contexts (`docs/modules/m1-hr-management/`). Contexts keep their seams (ports, own tables, own events) so any one can be split later on measured need. |
| 100-point checklist | Section 9 is the **interim governing checklist** until the owner supplies the original list. Every module's Definition of Done cites the items it satisfies. |
| Personas / journeys | Still open. The repository's earlier product discovery lists 10 personas (CEO, CTO, HR Manager, Finance Manager, Manager, Team Lead, Employee, IT Admin, Tenant Admin, JAAS Super Admin); the IA says 9. Reconcile before product-UI sign-off. |
| Providers (object storage, search, AI, payments, notification channels) | Still open; each is decided at the owning module's design phase and recorded in its `decisions.md`. Until then adapters are interfaces with a local/dev implementation. |
| Non-M0 SLOs | Still open. Interim: the section 3.9 latency budgets apply to every service. |

## 12. Repository layout (adopted)

The repository is a Go monorepo plus a Next.js frontend. The strangler migration in section 8 is applied to it as
follows. "Now" is the state at the start of the restructure; "Target" is what the restructure's Phases B–C deliver.

| Concern | Now | Target |
|---|---|---|
| M0 code | `backend/internal/identity/` | same path; gains outbox-backed events, idempotency, invitations, readiness (M0 `plan.md`) |
| M1 code | seven top-level packages `backend/internal/{organization,employee,attendance,leave,payroll,recruitment,performance}` | `backend/internal/hr/<context>/` plus `backend/internal/hr/module.go` — one HR composition root, one `RegisterRoutes` |
| Cross-context calls | direct imports of other contexts' `services` / `models` / `dto` packages | each context exposes a small `api` package (read ports + DTOs); consumers depend on that only; an import-boundary test enforces it |
| HR → M0 | HR imports identity repositories and middleware | HR depends on an `identityport` interface (authenticate, authorize, user lookup, audit); in-process adapter now, JWT-claims + HTTP adapter when HR is deployed separately |
| Routes | `/api/v1/{departments,teams,designations,mappings,org-chart,employees,attendance,shifts,leave,payroll,recruitment,performance}` | M0 unchanged (`/api/v1/auth`, `/users`, `/roles`, `/permissions`, `/audit-logs`, `/tenants`); all HR under **`/api/v1/hr/...`** |
| Database | one Postgres schema (`public`), one migration sequence `000001`–`000056` | Postgres schemas **`identity`** and **`hr`**, migration directories `backend/migrations/identity/` and `backend/migrations/hr/`, one owner per schema, no cross-schema foreign keys or writes |
| Reliability primitives | outbox per HR context; M0 publishes directly (best effort) | `backend/internal/shared/` gains `idempotency` (DB-backed `Idempotency-Key`) and `outbox` + `inbox` (versioned envelope: `event_version`, `correlation_id`, `causation_id`), used by every service |
| Placeholder packages | `backend/internal/{ai,analytics,approvals,assets,iot,meetings,notifications,projects}` (one stub file each) | removed; each module's package is created under its canonical name when that module enters design |
| Deployables | one binary `backend/cmd/main.go` | `cmd/main.go` stays the all-in-one dev/test binary; per-service entry points (`cmd/identity`, `cmd/hr`, ...) arrive with the gateway (section 8 phase 2). Boundaries are enforced in code first so the split is a build change, not a rewrite |
| Frontend | `/dev-test` console for M0 and two HR contexts; empty module folders | `/dev-test` **test console** covering every shipped feature (section 14); product UI later under `frontend/src/modules/<service>/` |
| Docs | 19 module bundles of 17–26 files each with legacy numbering | section 15 document map |

## 13. M1 HR Management scope (adopted)

HR Management is built to the depth of an enterprise HCM suite (reference: Workday HCM). Every capability is complete
end to end — data model, rules, permissions, audit, events, API, tests and a test-console page — before it is called
done. A smaller feature set at full quality beats a larger set of partial features.

| Context | Status at restructure | Capability (summary; full scope in `docs/modules/m1-hr-management/specification.md`) |
|---|---|---|
| `organization` | built | departments, teams, designations, reporting lines, org chart |
| `employee` | built | profiles, employment, contacts, masked statutory data, document metadata, timeline |
| `attendance` | built | shifts, assignments, punches, daily records, regularization |
| `leave` | built | leave types, holidays, ledger-backed balances, requests, approvals |
| `payroll` | built | salary structures, CTC assignments, maker-checker runs, statutory deductions, payslips, payout CSV |
| `recruitment` | built | jobs, candidates and stage history, interviews, offers, hire saga |
| `performance` | in progress | review cycles, goals and key results, check-ins, reviews, 9-box |
| `core` (new) | planned | job architecture (job families, job profiles, grades), positions and headcount, effective-dated job history, lifecycle business processes (hire, promote, transfer, change job, terminate, rehire) |
| `onboarding` (new) | planned | onboarding and offboarding plans, task checklists, document requirements, exit clearance, final-settlement hand-off to payroll |
| `compensation` (new) | planned | pay grades and ranges, salary change history, merit / increment cycles feeding payroll CTC |
| `learning` (new) | planned | course catalogue whose content is **external links or documents with thumbnails** (no video hosting), assignments with due dates, completions, custom learning plans |
| `benefits` (new) | planned | tenant-defined benefit plans, eligibility, enrolment windows, enrolments and dependants, payroll deduction hand-off |

## 14. Test console standard (adopted)

Every feature ships with a page in the frontend **test console** (route `/dev-test`) in the same delivery phase as its
backend. The console is how the owner verifies each module by hand without waiting for the product UI.

- One entry per module and, for M1, per HR context. Each lists every endpoint with a form and shows the raw request,
  the raw response, and the envelope or error verbatim.
- Session panel: tenant (subdomain) selection, login, refresh, logout, current permissions; a second session for
  cross-tenant and RBAC checks.
- Scenario runner: the context's golden tests as one-click scenarios with a pass/fail result per step.
- Inspectors: audit-log and outbox/event viewers for the active tenant.
- It is a verification tool, not product UI: no design-system work and no persona dashboards, and it is excluded from
  production builds.
- A feature is not done (Definition of Done) until its console page exists and its scenarios pass against the live
  backend. How to run and extend it: `docs/TEST_CONSOLE.md`.

## 15. Document map (adopted)

| Path | Role |
|---|---|
| `docs/ARCHITECTURE.md` | this file — the architecture authority |
| `docs/architecture/` | universal diagram and universal DB model and dependency matrix (owner bundle, verbatim) |
| `docs/SECURITY.md` | security law and code style; applies to every module |
| `docs/ENGINEERING.md` | how work is done: loop, gates, Definition of Done, sensors, git, documentation standard |
| `docs/TEST_CONSOLE.md` | how to run and extend the test console |
| `docs/SYSTEM_REFERENCE.md` | generated inventory of the code (routes, tables, files, functions, tests); Part 2 is regenerated, never hand-edited |
| `docs/modules/README.md` | module index and status |
| `docs/modules/m0-identity-access/` | M0 bundle |
| `docs/modules/m1-hr-management/` | M1 bundle; `contexts/<context>/` holds each bounded context's specification, design, testing and decisions |
| `docs/modules/m2-…` to `m12-…` | product module bundles; until a module enters design they hold `README.md`, `specification.md` (proposed contract from this file) and `model/` |
| `AGENTS.md`, `loop.md` | agent entry workflow and loop tick |

## 16. Migration status

| Restructure phase | Section 8 phase | Scope | Status |
|---|---|---|---|
| A — architecture and docs | 0 | this file, module bundles, document cleanup, M0 gap analysis | done; approved by owner 2026-10-20 |
| B — M0 alignment | 1–2 | access model (§17) and gaps listed in `docs/modules/m0-identity-access/plan.md` | done 2026-10-20 |
| C — HR service boundary | 3 | section 12 "Target" column for M1, shared idempotency/outbox | done 2026-10-20 (inbox arrives with the first consumer) |
| D — test console | 2 (shell) | console covering M0 and every built HR context | done 2026-10-20 |
| E — complete HR | 3 | finish `performance`, then `core`, `onboarding`, `compensation`, `learning`, `benefits` | current |
| Afterwards | 4–7 | M7 Approvals and M8 Documents, then M2–M6 and M9–M12 | later |

## 17. Access model (adopted 2026-10-20)

Hierarchical, scoped, role-based access with per-person exceptions — the model used by enterprise HCM suites.

### 17.1 Building blocks

| Concept | Definition |
|---|---|
| Permission | `resource:action` from a catalogue each service registers with M0 (grouped by module, e.g. HR › Leave › approve). Sensitive permissions are flagged. |
| Scope | how far a granted permission reaches: `own` (the caller's records), `direct_reports`, `all_reports` (whole reporting chain below the caller), `department` (caller's department and its sub-departments), `tenant` (everyone). Scopes are ordered: own < direct_reports < all_reports < department < tenant for escalation checks. |
| Grant | a (permission, scope) pair. |
| Role | a named, tenant-owned set of grants with an optional parent role; a role's effective grants = its own grants ∪ its parent's effective grants (acyclic, depth ≤ 10). |
| Predefined roles | seeded per tenant, editable or cloneable, never silently overwritten: Tenant Admin (all), CEO / Executive, HR Admin, HR Manager, Manager, Employee; side roles Payroll Admin, Payroll Approver, Finance, Recruiter, Interviewer, IT Admin, Auditor. Inheritance: Employee ⊂ Manager ⊂ HR Manager ⊂ HR Admin ⊂ CEO / Executive; Tenant Admin bypasses checks. |
| Automatic roles | **Employee** is held by every user with an employee record; **Manager** by every user who currently has direct reports. They are derived from HR facts (no manual assignment) and change as the org changes. |
| User grant | a grant given to one user directly, optional `expires_at` (temporary cover, delegation). |
| User block | a permission removed from one user regardless of roles; **a block always wins**. |

### 17.2 Rules

- Effective access of a user = union of grants from assigned roles, automatic roles and unexpired user grants, minus
  blocks. When several grants give the same permission, the widest scope applies.
- Anti-escalation: nobody can create or assign a grant (permission + scope) wider than one they hold themselves;
  Tenant Admin can be granted only by a Tenant Admin or the platform.
- Sensitive data needs its sensitive permission in addition to scope (`docs/modules/m1-hr-management/specification.md` §2.2).
- Separation of duties (maker ≠ checker) is enforced by the owning service regardless of grants.
- Every change to roles, inheritance, grants and blocks is audited; an "explain access" API shows each effective
  permission with its source (role, inherited role, automatic role, user grant) and any block.

### 17.3 Responsibilities

- **M0** owns the catalogue, roles, inheritance, user grants, blocks and predefined-role templates, and answers
  "which scopes does this user hold for `resource:action`" (cached per user, invalidated by an access version that
  changes on any relevant write).
- **The service that owns the data evaluates the scope** against its own facts. HR resolves `direct_reports`,
  `all_reports` and `department` from its reporting lines; M0 never reads HR tables. HR tells M0 who currently
  qualifies for the automatic Manager and Employee roles through an event/port, so M0 keeps no copy of the org chart.
- Every endpoint declares its permission and the scopes that apply to it; a route contract test checks the
  declaration.
