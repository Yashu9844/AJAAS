# M8 Documents — Specification (PROPOSED)

> Not frozen. This is the target-state contract from `docs/ARCHITECTURE.md` section 5, combined with the logical data
> model from `model/db-model.md`. API paths are proposed shapes. Universal rules (tenancy, idempotency, outbox/inbox,
> envelopes, security, latency) in `docs/ARCHITECTURE.md` section 3 apply in full.

**Purpose:** document metadata, versions, sharing, retention, and secure
references to binary objects.

**Data model:** Document, DocumentVersion, StorageObjectReference,
DocumentACL, ShareGrant, ScanResult, RetentionPolicy, LegalHold,
AccessAudit.

**Representative APIs:**
- `POST /api/v1/documents/upload-sessions`
- `POST /api/v1/documents`
- `GET /api/v1/documents/{id}`
- `POST /api/v1/documents/{id}/versions`
- `POST /api/v1/documents/{id}/share-grants`
- `POST /api/v1/documents/{id}/revoke-share`
- `POST /api/v1/documents/{id}/retention-hold`

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

## Proposed entities

| Entity/table | Key fields (illustrative) |
|---|---|
| `Document` | document_id, tenant_id, owner_user_id, document_type, classification, status |
| `DocumentVersion` | version_id, document_id, version_no, object_key, checksum, size_bytes, scan_state, created_at |
| `DocumentACL` | document_id, principal_type, principal_id, permission |
| `ShareGrant` | share_id, document_id, token_hash, expires_at, revoked_at |
| `ScanResult` | scan_id, version_id, scanner, result, completed_at |
| `RetentionPolicy` | policy_id, tenant_id, classification, retention_days |
| `LegalHold` | hold_id, document_id, reason, placed_at, released_at |
| `AccessAudit` | audit_id, document_id, actor_id, action, occurred_at |

## Relationships and invariants

Document 1:N DocumentVersion, DocumentACL and AccessAudit. DocumentVersion references immutable object key/checksum and scan state. ShareGrant stores only token hash; legal hold overrides normal deletion. ACL must be checked before issuing short-lived signed URLs.

## Service dependencies

Object storage contains binaries; KMS/secrets manager protects encryption keys. Domain services own the business meaning of attachments. M0 supplies identity/authorization context; publish lifecycle events.
