# M10 Notifications — Specification (PROPOSED)

> Not frozen. This is the target-state contract from `docs/ARCHITECTURE.md` section 5, combined with the logical data
> model from `model/db-model.md`. API paths are proposed shapes. Universal rules (tenancy, idempotency, outbox/inbox,
> envelopes, security, latency) in `docs/ARCHITECTURE.md` section 3 apply in full.

**Purpose:** durable dispatch of email, in-app, push, SMS or other
enabled channels, with templates, preferences, retries, and delivery
receipts.

**Data model:** NotificationTemplate, TemplateVersion, UserPreference,
DispatchJob, DispatchAttempt, ProviderReceipt, SuppressionEntry,
DeadLetterRecord.

**Representative APIs:**
- `GET/POST /api/v1/notification-templates`
- `GET/PATCH /api/v1/notification-preferences/me`
- `GET /api/v1/notifications/inbox`
- `POST /api/v1/notifications/{id}/read`
- `POST /api/v1/notifications/{id}/retry` (privileged/admin only)

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

## Proposed entities

| Entity/table | Key fields (illustrative) |
|---|---|
| `NotificationTemplate` | template_id, tenant_id, channel, key, active_version |
| `TemplateVersion` | template_id, version, body_ref, locale, created_at |
| `UserPreference` | tenant_id, user_id, channel, category, enabled |
| `NotificationInboxItem` | notification_id, tenant_id, user_id, event_key, read_at, created_at |
| `DispatchJob` | job_id, tenant_id, event_id, recipient_ref, channel, status, idempotency_key |
| `DispatchAttempt` | attempt_id, job_id, attempt_no, provider_request_ref, outcome, attempted_at |
| `ProviderReceipt` | receipt_id, job_id, provider_message_id, state, received_at |
| `SuppressionEntry` | tenant_id, destination_hash, reason, created_at |
| `DeadLetterRecord` | dead_letter_id, job_id, failure_code, failed_at |

## Relationships and invariants

NotificationTemplate 1:N TemplateVersion. DispatchJob 1:N DispatchAttempt and 0:N ProviderReceipt. Inbox item unique by tenant/user/event key to deduplicate. Delivery outcome is not equivalent to business success.

## Service dependencies

Consumes domain events from RabbitMQ and calls email/SMS/push providers. Provider credentials in secret manager. Delivery events go to Reports and source modules where useful; dispatch failure never rolls back a committed source transaction.
