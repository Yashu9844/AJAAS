# JAAS Universal Data Model & Service Dependencies

## Purpose
This file defines cross-service data conventions and the target dependency model. It is a design proposal aligned to the JAAS product IA and the current backend reference; it is not a claim that every target service/schema already exists.

## Universal platform tables / concepts
- `tenants`: tenant lifecycle and platform metadata; owned by M0.
- `user_identities`: login identity and account state; owned by M0.
- `roles`, `permissions`, `role_permissions`, `user_roles`: authorization catalogue and assignments; owned by M0.
- `sessions`, `refresh_tokens`, `password_reset_tokens`, `invitations`: authentication lifecycle; owned by M0.
- `security_audit_events`: security-relevant audit trail; owned by M0.
- `idempotency_records`: service-local per API/service boundary; do not centralize all command results into one shared mutable table.
- `outbox_events`: service-local transactional outbox.
- `inbox_events`: service-local consumer deduplication.
- `service_operations` / saga state: owned by the service orchestrating a long-running workflow.
- `object_storage_refs`: owned by Documents or the relevant domain service; binary bytes live in object storage.

## Dependency rules
- Frontend -> API Gateway -> service API.
- Every service validates identity and authorization; gateway validation alone is insufficient.
- M0 is a foundational dependency for authentication/identity, but business services should not synchronously call M0 for every request if a verified token/short-lived service credential and safe permission cache suffice.
- HR owns employee/org/attendance/leave/payroll/recruitment/performance facts.
- Approvals owns workflow state; the originating domain service owns its business record and validates the final transition.
- Documents owns file metadata/version/ACL/storage references; source domains own the meaning of a document attachment.
- Notifications owns delivery state, not the business outcome.
- Reports consumes events or bounded read APIs to create projections; no unrestricted operational DB access.
- AI uses allowlisted APIs/tools with actor/tenant permissions and audit; no direct SQL/database credentials.
- Settings stores typed configuration and secret references; secret values live in a secret manager.
- Cross-service interactions use versioned APIs/events and durable outbox/inbox. Avoid circular synchronous dependencies.
- Dashboard is a frontend composition/read-model concern, not a separate business microservice.

## Service dependency matrix
| Service | Primary dependencies | Publishes / consumes |
|---|---|---|
| M0 Identity & Access | PostgreSQL, Redis (bounded cache/rate limit), secrets/KMS | Publishes identity lifecycle events; consumes no business-domain events for core authentication |
| M1 HR Management | M0 identity claims, PostgreSQL, Redis, RabbitMQ, Documents refs, Approvals workflow, Notifications events | Publishes HR domain events; consumes approval decisions and relevant identity/document events |
| M2 Projects & Tasks | M0, HR identity references, PostgreSQL, RabbitMQ, Documents refs, Notifications events | Publishes project/task events; consumes identity lifecycle and notification-related configuration |
| M3 Meetings | M0, HR identity references, PostgreSQL, RabbitMQ, Documents refs, Projects task references | Publishes meeting/minutes/action-item events; consumes identity and task references |
| M4 Assets & Inventory | M0, HR employee references, PostgreSQL, RabbitMQ, Approvals, Documents, Notifications | Publishes asset/stock events; consumes approval outcomes and identity lifecycle |
| M5 Finance | M0, PostgreSQL, RabbitMQ, Approvals, Documents, HR payroll export/reference where agreed, Notifications | Publishes expense/reimbursement/budget events; consumes approval outcomes |
| M6 Reports & Analytics | RabbitMQ event streams, PostgreSQL projection store, object storage, M0 authorization context | Publishes report execution/projection events; consumes versioned domain events |
| M7 Approvals | M0 actor/role claims, PostgreSQL, RabbitMQ, Notifications | Publishes workflow decisions; consumes approval requests from HR, Finance, Assets and other domains |
| M8 Documents | M0, PostgreSQL, object storage, malware scanner, KMS, RabbitMQ | Publishes upload/scan/version/access lifecycle events; consumes document requirement/reference events |
| M9 AI Assistant | M0, allowlisted domain APIs, model provider, PostgreSQL, secrets/KMS, audit | Publishes run/tool audit events; consumes only approved retrieval/event feeds |
| M10 Notifications | RabbitMQ, PostgreSQL, Redis (rate limits/cache), email/SMS/push providers, secrets manager | Consumes domain events; publishes delivery outcome events |
| M11 Knowledge Base | M0, PostgreSQL, search index, Documents refs, RabbitMQ | Publishes article/publication/index events; consumes document and identity/ACL lifecycle events |
| M12 Settings | M0, PostgreSQL, secrets manager, RabbitMQ | Publishes versioned configuration/integration events; domain services consume only relevant config changes |

## Dependency cautions
1. Prefer events for non-blocking notifications, analytics, search indexing, and dashboard projections.
2. Request-time APIs are appropriate when authoritative permission/business validation is required.
3. No synchronous chain should traverse more than one downstream service for common interactive requests without an explicit latency budget.
4. Cross-service workflows require durable saga state, retries, timeout policy, idempotency and operator repair.
5. Service names and endpoint shapes in this bundle are target-state proposals; verify against current OpenAPI and database migrations before implementation.
