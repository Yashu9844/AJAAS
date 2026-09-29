# Module 6 — Security

Law: docs/SECURITY.md (read it before any security-sensitive change).
Module note: Workflow integrity (High). No self-approval bypass; every decision audited with actor + reason.

Threats, PII handling, rate limits, audit requirements and trust boundaries are specified at P1 design.
Hard rules already in force: deny by default, generic auth errors, no secrets in code/logs/responses/git, parameterized queries only, immutable audit on state changes, tenant_id on every scoped query.
