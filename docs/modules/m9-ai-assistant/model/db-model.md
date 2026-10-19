# M9 — AI Assistant — Database Model & Dependencies

**Domain scope:** Conversations, messages, runs, tool calls/results, model usage, policy decisions and feedback

**Purpose:** Governed AI interface; no direct database access or authority over source facts.

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
| `Conversation` | conversation_id, tenant_id, owner_user_id, status, retention_class |
| `Message` | message_id, conversation_id, role, content_ref, redaction_state, created_at |
| `AssistantRun` | run_id, conversation_id, status, model_ref, started_at, finished_at, idempotency_key |
| `ToolCall` | tool_call_id, run_id, tool_name, schema_version, status, target_service, idempotency_key |
| `ToolResultReference` | tool_call_id, result_ref, sensitivity, created_at |
| `PromptTemplateVersion` | template_id, version, content_ref, status |
| `ModelUsage` | usage_id, run_id, input_tokens, output_tokens, cost_minor |
| `PolicyDecision` | decision_id, run_id, tool_call_id, allowed, reason_code |
| `UserFeedback` | feedback_id, run_id, rating, feedback_ref |

## Relationships and invariants

Conversation 1:N Message and AssistantRun. Run 1:N ToolCall and ModelUsage. Every write-capable ToolCall links to a PolicyDecision and durable idempotency record. Sensitive prompts/results may be stored by reference with redaction and retention policy.

## Service dependencies

Calls allowlisted service APIs with user/tenant permissions; model provider credentials stay in secrets manager. Documents/Knowledge Base retrieval must apply source ACLs. No direct DB access, unrestricted SQL, or implicit write permission.

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
