# Module 12 — Security

Law: docs/SECURITY.md (read it before any security-sensitive change).
Module note: Device identity and auth (High). Validate all ingest payloads; per-tenant device isolation.

Threats, PII handling, rate limits, audit requirements and trust boundaries are specified at P1 design.
Hard rules already in force: deny by default, generic auth errors, no secrets in code/logs/responses/git, parameterized queries only, immutable audit on state changes, tenant_id on every scoped query.
