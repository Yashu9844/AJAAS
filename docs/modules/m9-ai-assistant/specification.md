# M9 AI Assistant — Specification (PROPOSED)

> Not frozen. This is the target-state contract from `docs/ARCHITECTURE.md` section 5, combined with the logical data
> model from `model/db-model.md`. API paths are proposed shapes. Universal rules (tenancy, idempotency, outbox/inbox,
> envelopes, security, latency) in `docs/ARCHITECTURE.md` section 3 apply in full.

**Purpose:** conversational interface and governed tool execution across
JAAS. It is not an alternate authorization path or a source of truth.

**Data model:** Conversation, Message, AssistantRun, ToolCall,
ToolResultReference, PromptTemplateVersion, ModelUsage, UserFeedback,
PolicyDecision, RedactionRecord.

**Representative APIs:**
- `POST /api/v1/ai/conversations`
- `GET /api/v1/ai/conversations/{id}`
- `POST /api/v1/ai/conversations/{id}/messages`
- `GET /api/v1/ai/runs/{id}`
- `POST /api/v1/ai/runs/{id}/cancel`
- `GET /api/v1/ai/usage`

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

## Proposed entities

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
