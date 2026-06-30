# Module 0 — Domain Events

Every domain event published by Module 0. Includes producer, consumer, payload, retry strategy, idempotency, dead letter queue behaviour, and ordering requirements.

---

## Event Infrastructure

### Exchange

| Property | Value |
|----------|-------|
| Exchange Name | `jaas.identity.events` |
| Exchange Type | Topic |
| Durable | Yes |
| Auto-Delete | No |

### Routing Key Convention

```
identity.{resource}.{action}
```

Examples:
- `identity.tenant.created`
- `identity.user.login_success`
- `identity.role.assigned`

### Event Envelope

Every event is wrapped in a standard envelope:

```json
{
  "event_id": "uuid-v4",
  "event_type": "TenantCreated",
  "routing_key": "identity.tenant.created",
  "version": "1.0",
  "occurred_at": "2026-07-01T00:00:00Z",
  "producer": "identity-service",
  "tenant_id": "uuid",
  "correlation_id": "request-uuid",
  "payload": { ... }
}
```

| Field | Type | Description |
|-------|------|-------------|
| event_id | UUID | Unique identifier for this event instance |
| event_type | string | Event type name |
| routing_key | string | RabbitMQ routing key |
| version | string | Schema version for payload |
| occurred_at | timestamp | When the event occurred |
| producer | string | Service name that produced the event |
| tenant_id | UUID | Tenant scope (if applicable) |
| correlation_id | UUID | Request ID for tracing |
| payload | object | Event-specific data |

---

## Event Definitions

### 1. TenantCreated

| Property | Value |
|----------|-------|
| **Routing Key** | `identity.tenant.created` |
| **Producer** | TenantService |
| **Consumers** | Module 1 (Organization) — auto-create default org structure |
| **Trigger** | POST /api/v1/tenants (after commit) |

**Payload**:

```json
{
  "tenant_id": "550e8400-e29b-41d4-a716-446655440000",
  "name": "Acme Corporation",
  "slug": "acme",
  "plan": "professional",
  "created_at": "2026-07-01T00:00:00Z"
}
```

| Concern | Strategy |
|---------|----------|
| **Retry** | 3 retries with exponential backoff (1s, 5s, 25s) |
| **Idempotency** | Consumer uses `event_id` as deduplication key. Processing the same `event_id` twice is a no-op. |
| **Dead Letter Queue** | `jaas.identity.events.dlq` — after 3 failed retries, message is routed to DLQ for manual inspection |
| **Ordering** | Not required. Tenant creation is a one-time event. |

---

### 2. TenantActivated

| Property | Value |
|----------|-------|
| **Routing Key** | `identity.tenant.activated` |
| **Producer** | TenantService |
| **Consumers** | Module 7 (Notifications) — send reactivation notification to tenant admin |
| **Trigger** | POST /api/v1/tenants/{id}/activate (after commit) |

**Payload**:

```json
{
  "tenant_id": "550e8400-e29b-41d4-a716-446655440000",
  "previous_status": "suspended",
  "new_status": "active",
  "activated_at": "2026-07-01T00:00:00Z"
}
```

| Concern | Strategy |
|---------|----------|
| **Retry** | 3 retries with exponential backoff |
| **Idempotency** | Consumer uses `event_id` as deduplication key |
| **Dead Letter Queue** | `jaas.identity.events.dlq` |
| **Ordering** | Ordered per `tenant_id`. Activate must be processed after any preceding suspend. Use a single consumer per tenant partition. |

---

### 3. TenantSuspended

| Property | Value |
|----------|-------|
| **Routing Key** | `identity.tenant.suspended` |
| **Producer** | TenantService |
| **Consumers** | Module 7 (Notifications) — send suspension notification; all modules — pause tenant-specific background jobs |
| **Trigger** | POST /api/v1/tenants/{id}/suspend (after commit) |

**Payload**:

```json
{
  "tenant_id": "550e8400-e29b-41d4-a716-446655440000",
  "previous_status": "active",
  "new_status": "suspended",
  "suspended_at": "2026-07-01T00:00:00Z"
}
```

| Concern | Strategy |
|---------|----------|
| **Retry** | 3 retries with exponential backoff |
| **Idempotency** | Consumer uses `event_id` as deduplication key |
| **Dead Letter Queue** | `jaas.identity.events.dlq` |
| **Ordering** | Ordered per `tenant_id`. Must be processed before any subsequent activate event. |

---

### 4. UserCreated

| Property | Value |
|----------|-------|
| **Routing Key** | `identity.user.created` |
| **Producer** | UserService |
| **Consumers** | Module 1 (Organization) — auto-create employee stub; Module 7 (Notifications) — send welcome email |
| **Trigger** | POST /api/v1/users (after commit) |

**Payload**:

```json
{
  "user_id": "550e8400-e29b-41d4-a716-446655440001",
  "tenant_id": "550e8400-e29b-41d4-a716-446655440000",
  "email": "jane@acme.com",
  "first_name": "Jane",
  "last_name": "Smith",
  "status": "active",
  "role_ids": ["550e8400-e29b-41d4-a716-446655440010"],
  "created_by": "550e8400-e29b-41d4-a716-446655440002",
  "created_at": "2026-07-01T00:00:00Z"
}
```

| Concern | Strategy |
|---------|----------|
| **Retry** | 3 retries with exponential backoff |
| **Idempotency** | Consumer uses `event_id` as deduplication key |
| **Dead Letter Queue** | `jaas.identity.events.dlq` |
| **Ordering** | Not strictly required. But should be processed before any UserDeactivated for the same user. |

---

### 5. UserInvited

| Property | Value |
|----------|-------|
| **Routing Key** | `identity.user.invited` |
| **Producer** | UserService |
| **Consumers** | Module 7 (Notifications) — send invitation email with activation link |
| **Trigger** | POST /api/v1/users (when user created with status "invited") |

**Payload**:

```json
{
  "user_id": "550e8400-e29b-41d4-a716-446655440001",
  "tenant_id": "550e8400-e29b-41d4-a716-446655440000",
  "email": "invited@acme.com",
  "first_name": "New",
  "last_name": "Employee",
  "invited_by": "550e8400-e29b-41d4-a716-446655440002",
  "invited_at": "2026-07-01T00:00:00Z"
}
```

| Concern | Strategy |
|---------|----------|
| **Retry** | 3 retries with exponential backoff |
| **Idempotency** | Consumer uses `event_id` as deduplication key |
| **Dead Letter Queue** | `jaas.identity.events.dlq` |
| **Ordering** | Not required |

---

### 6. UserDeactivated

| Property | Value |
|----------|-------|
| **Routing Key** | `identity.user.deactivated` |
| **Producer** | UserService |
| **Consumers** | Module 1 (Organization) — update employee status; Module 7 (Notifications) — send deactivation notice to admins; all modules — revoke active user assignments |
| **Trigger** | POST /api/v1/users/{id}/deactivate (after commit) |

**Payload**:

```json
{
  "user_id": "550e8400-e29b-41d4-a716-446655440001",
  "tenant_id": "550e8400-e29b-41d4-a716-446655440000",
  "deactivated_by": "550e8400-e29b-41d4-a716-446655440002",
  "deactivated_at": "2026-07-01T00:00:00Z"
}
```

| Concern | Strategy |
|---------|----------|
| **Retry** | 3 retries with exponential backoff |
| **Idempotency** | Consumer uses `event_id` as deduplication key |
| **Dead Letter Queue** | `jaas.identity.events.dlq` |
| **Ordering** | Ordered per `user_id`. Must be processed after UserCreated for the same user. |

---

### 7. RoleAssigned

| Property | Value |
|----------|-------|
| **Routing Key** | `identity.role.assigned` |
| **Producer** | RoleService |
| **Consumers** | Module 7 (Notifications) — notify user of new role |
| **Trigger** | POST /api/v1/users/{id}/roles (after commit) |

**Payload**:

```json
{
  "user_id": "550e8400-e29b-41d4-a716-446655440001",
  "tenant_id": "550e8400-e29b-41d4-a716-446655440000",
  "role_ids": ["550e8400-e29b-41d4-a716-446655440010"],
  "assigned_by": "550e8400-e29b-41d4-a716-446655440002",
  "assigned_at": "2026-07-01T00:00:00Z"
}
```

| Concern | Strategy |
|---------|----------|
| **Retry** | 3 retries with exponential backoff |
| **Idempotency** | Consumer uses `event_id` as deduplication key |
| **Dead Letter Queue** | `jaas.identity.events.dlq` |
| **Ordering** | Not strictly required |

---

### 8. RoleRevoked

| Property | Value |
|----------|-------|
| **Routing Key** | `identity.role.revoked` |
| **Producer** | RoleService |
| **Consumers** | Module 7 (Notifications) — notify user of role removal |
| **Trigger** | Role removal operation (after commit) |

**Payload**:

```json
{
  "user_id": "550e8400-e29b-41d4-a716-446655440001",
  "tenant_id": "550e8400-e29b-41d4-a716-446655440000",
  "role_ids": ["550e8400-e29b-41d4-a716-446655440010"],
  "revoked_by": "550e8400-e29b-41d4-a716-446655440002",
  "revoked_at": "2026-07-01T00:00:00Z"
}
```

| Concern | Strategy |
|---------|----------|
| **Retry** | 3 retries with exponential backoff |
| **Idempotency** | Consumer uses `event_id` as deduplication key |
| **Dead Letter Queue** | `jaas.identity.events.dlq` |
| **Ordering** | Not strictly required |

---

### 9. PermissionAssigned

| Property | Value |
|----------|-------|
| **Routing Key** | `identity.permission.assigned` |
| **Producer** | RoleService |
| **Consumers** | Cache invalidation service — clear permission cache for affected users |
| **Trigger** | POST /api/v1/roles/{id}/permissions (after commit) |

**Payload**:

```json
{
  "role_id": "550e8400-e29b-41d4-a716-446655440010",
  "tenant_id": "550e8400-e29b-41d4-a716-446655440000",
  "permission_ids": ["550e8400-e29b-41d4-a716-446655440020"],
  "assigned_at": "2026-07-01T00:00:00Z"
}
```

| Concern | Strategy |
|---------|----------|
| **Retry** | 3 retries with exponential backoff |
| **Idempotency** | Consumer uses `event_id` as deduplication key |
| **Dead Letter Queue** | `jaas.identity.events.dlq` |
| **Ordering** | Not required |

---

### 10. PermissionRevoked

| Property | Value |
|----------|-------|
| **Routing Key** | `identity.permission.revoked` |
| **Producer** | RoleService |
| **Consumers** | Cache invalidation service — clear permission cache for affected users |
| **Trigger** | Permission removal operation (after commit) |

**Payload**:

```json
{
  "role_id": "550e8400-e29b-41d4-a716-446655440010",
  "tenant_id": "550e8400-e29b-41d4-a716-446655440000",
  "permission_ids": ["550e8400-e29b-41d4-a716-446655440020"],
  "revoked_at": "2026-07-01T00:00:00Z"
}
```

| Concern | Strategy |
|---------|----------|
| **Retry** | 3 retries with exponential backoff |
| **Idempotency** | Consumer uses `event_id` as deduplication key |
| **Dead Letter Queue** | `jaas.identity.events.dlq` |
| **Ordering** | Not required |

---

### 11. PasswordReset

| Property | Value |
|----------|-------|
| **Routing Key** | `identity.password.reset` |
| **Producer** | AuthService |
| **Consumers** | Module 7 (Notifications) — send confirmation email; Security monitoring — alert on password changes |
| **Trigger** | POST /api/v1/auth/reset-password (after commit) |

**Payload**:

```json
{
  "user_id": "550e8400-e29b-41d4-a716-446655440001",
  "tenant_id": "550e8400-e29b-41d4-a716-446655440000",
  "reset_at": "2026-07-01T00:00:00Z"
}
```

| Concern | Strategy |
|---------|----------|
| **Retry** | 3 retries with exponential backoff |
| **Idempotency** | Consumer uses `event_id` as deduplication key |
| **Dead Letter Queue** | `jaas.identity.events.dlq` |
| **Ordering** | Not required |

---

### 12. SessionRevoked

| Property | Value |
|----------|-------|
| **Routing Key** | `identity.session.revoked` |
| **Producer** | AuthService / SessionService |
| **Consumers** | Cache invalidation service — remove session from Redis; Security monitoring |
| **Trigger** | POST /api/v1/auth/logout, user deactivation, tenant suspension (after commit) |

**Payload**:

```json
{
  "session_id": "550e8400-e29b-41d4-a716-446655440099",
  "user_id": "550e8400-e29b-41d4-a716-446655440001",
  "tenant_id": "550e8400-e29b-41d4-a716-446655440000",
  "reason": "logout",
  "revoked_at": "2026-07-01T00:00:00Z"
}
```

| Concern | Strategy |
|---------|----------|
| **Retry** | 3 retries with exponential backoff |
| **Idempotency** | Consumer uses `event_id` as deduplication key |
| **Dead Letter Queue** | `jaas.identity.events.dlq` |
| **Ordering** | Not required |

---

## Dead Letter Queue (DLQ) Strategy

| Property | Value |
|----------|-------|
| DLQ Exchange | `jaas.identity.events.dlx` |
| DLQ Queue | `jaas.identity.events.dlq` |
| DLQ Routing Key | Original routing key preserved |
| Max Retries Before DLQ | 3 |
| DLQ TTL | None (messages persist until manually processed) |
| Alerting | Alert operations team when DLQ depth > 10 messages |
| Resolution | Manual inspection via admin tooling. Replay or discard. |

---

## Consumer Best Practices

1. **Always check `event_id` for deduplication** before processing.
2. **Use manual acknowledgement** — ACK only after successful processing.
3. **Reject and requeue** on transient failures (database timeout, network error).
4. **Reject without requeue** (→ DLQ) on permanent failures (schema mismatch, business rule violation).
5. **Log every event** received, processed, and failed for auditing.
6. **Set prefetch count** to limit concurrent processing per consumer.
