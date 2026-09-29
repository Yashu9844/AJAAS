# Module 11 — Security

Law: docs/SECURITY.md (read it before any security-sensitive change).
Module note: Prompt and output leakage risk (High). Never mix tenants in training context; redact PII before model calls; audit AI actions.

Threats, PII handling, rate limits, audit requirements and trust boundaries are specified at P1 design.
Hard rules already in force: deny by default, generic auth errors, no secrets in code/logs/responses/git, parameterized queries only, immutable audit on state changes, tenant_id on every scoped query.
