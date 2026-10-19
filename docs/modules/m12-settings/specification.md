# M12 Settings — Specification (PROPOSED)

> Not frozen. This is the target-state contract from `docs/ARCHITECTURE.md` section 5, combined with the logical data
> model from `model/db-model.md`. API paths are proposed shapes. Universal rules (tenancy, idempotency, outbox/inbox,
> envelopes, security, latency) in `docs/ARCHITECTURE.md` section 3 apply in full.

**Purpose:** tenant preferences, feature configuration, integrations,
configurable workflow defaults, and environment-level feature switches.

**Data model:** TenantSetting, FeatureFlag, IntegrationConfig,
WebhookSubscription, ConfigVersion, ChangeApproval, SecretReference.

**Representative APIs:**
- `GET/PATCH /api/v1/settings/tenant`
- `GET/POST /api/v1/settings/integrations`
- `POST /api/v1/settings/integrations/{id}/test`
- `GET/POST /api/v1/settings/webhooks`
- `GET/PATCH /api/v1/settings/features`
- `GET /api/v1/settings/audit`

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

## Proposed entities

| Entity/table | Key fields (illustrative) |
|---|---|
| `TenantSetting` | tenant_id, setting_key, typed_value, version, updated_by |
| `FeatureFlag` | flag_id, tenant_id, key, enabled, targeting_rules_ref, version |
| `IntegrationConfig` | integration_id, tenant_id, provider, status, config_ref, secret_ref |
| `WebhookSubscription` | webhook_id, tenant_id, endpoint_ref, secret_ref, event_filters, status |
| `ConfigVersion` | config_version_id, tenant_id, scope, version, checksum, created_at |
| `ChangeApproval` | change_id, config_version_id, approval_instance_id, status |
| `SecretReference` | secret_ref_id, provider_key_ref, rotated_at, status |
| `WebhookDelivery` | delivery_id, webhook_id, event_id, attempt_no, status, next_attempt_at |

## Relationships and invariants

Tenant has many TenantSetting, FeatureFlag, IntegrationConfig and WebhookSubscription. ConfigVersion records immutable revisions. WebhookDelivery is retryable/idempotent and uses signed payloads and bounded retry policies.

## Service dependencies

M0 controls access. Secret values are external in a secrets manager; DB stores references only. Domain services consume relevant versioned configuration events and validate values locally.
