# M7 Approvals — Specification (PROPOSED)

> Not frozen. This is the target-state contract from `docs/ARCHITECTURE.md` section 5, combined with the logical data
> model from `model/db-model.md`. API paths are proposed shapes. Universal rules (tenancy, idempotency, outbox/inbox,
> envelopes, security, latency) in `docs/ARCHITECTURE.md` section 3 apply in full.

**Purpose:** reusable workflow engine for routing, decisions,
escalation, delegation, and workflow history. The source business
service owns the business record and validates final state transitions.

**Data model:** WorkflowDefinition, WorkflowVersion, WorkflowInstance,
StepInstance, Assignment, Decision, Delegation, EscalationTimer,
WorkflowAudit, IdempotencyRecord.

**Representative APIs:**
- `GET/POST /api/v1/approval-workflows`
- `POST /api/v1/approvals/instances`
- `GET /api/v1/approvals/inbox`
- `POST /api/v1/approvals/{id}/approve`
- `POST /api/v1/approvals/{id}/reject`
- `POST /api/v1/approvals/{id}/delegate`
- `POST /api/v1/approvals/{id}/recall` (only where policy permits)

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

## Proposed entities

| Entity/table | Key fields (illustrative) |
|---|---|
| `WorkflowDefinition` | workflow_id, tenant_id, name, status |
| `WorkflowVersion` | workflow_id, version, schema, published_at |
| `WorkflowInstance` | instance_id, tenant_id, workflow_id, version, source_type, source_id, status, correlation_id |
| `StepInstance` | step_instance_id, instance_id, step_key, status, sequence |
| `ApproverAssignment` | assignment_id, step_instance_id, approver_user_id, assigned_at |
| `ApprovalDecision` | decision_id, step_instance_id, actor_id, decision, comment_ref, decided_at |
| `Delegation` | delegation_id, tenant_id, from_user_id, to_user_id, starts_at, ends_at |
| `EscalationTimer` | timer_id, step_instance_id, due_at, status |
| `WorkflowAudit` | audit_id, instance_id, actor_id, action, occurred_at |

## Relationships and invariants

WorkflowDefinition 1:N WorkflowVersion and WorkflowInstance. Instance pins a version and has 1:N StepInstance. StepInstance has assignments and immutable decisions. Unique decision/idempotency constraints prevent conflicting duplicate actions; timer workers are replay-safe.

## Service dependencies

M0 supplies actor/role context; Notifications receives assignment/outcome events. HR, Finance and Assets create workflow requests and consume decisions, then independently validate their own domain transitions.
