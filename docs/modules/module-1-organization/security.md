# Module 1 — Security (P1: module specifics added; law = docs/SECURITY.md)

Law: docs/SECURITY.md (read before any security-sensitive change).

## Module-specific

- Data class: internal structure (Medium). No salary, no credentials, no tokens in org tables. Lead/manager references are user IDs only — never embed emails, hashes, or tokens in org responses or events.
- Permissions (new, seeded with Module 1 deploy): `organization:read` (departments/teams/designations/mappings/chart reads), `organization:update` (all writes incl. moves, mapping changes, deactivations). Tenant-scoped enforcement via Module 0 `RequirePermission`; `tenant_admin` bypass preserved.
- Trust boundary: same as Module 0 — JWT tid == subdomain tenant; every query filters `tenant_id`; cross-tenant IDs → 404. Manager/lead `user_id` inputs validated against Module 0 (exists + same tenant + active) — never trusted from body alone.
- Rate limits: inherit Module 0 table; org writes fall under default 100/min per user. Chart reads cached, not rate-exempt.
- Audit: every FR-D/T/DG/H/M mutation writes an immutable Module 0 audit row (actor, resource, resource_id, diff old/new, IP, user agent). Reads are not audited except org-chart exports (flagged, audited).
- PII: mapping responses include user IDs + org position only. No bulk export endpoint in v1 (prevents silent exfiltration; export arrives with Module 10 + consent + audit).
- Attack surfaces: hierarchy cycle payloads (guarded FR-H003/FR-M003), depth bombs (FR-H004 cap), IDOR via ID guessing (tenant scoping + 404), event spoofing (consumers verify tenant_id + event_id idempotency, ignore unknown producers).
- Failure: tenant suspended → 403 on all mutating endpoints (FR-E002); DB down → 503 + rollback; cache down → compute direct.

Pre-production checks (§12 of docs/SECURITY.md) apply in full at P3 exit.
