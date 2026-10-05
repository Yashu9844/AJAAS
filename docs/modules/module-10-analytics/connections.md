# Module 10 — Connections

Depends On: All modules (read contracts + events).
Consumed By: Module 11.

Dependency types per edge: Runtime / API / Data / Configuration / Authentication / Infrastructure / Testing.
Full edge table (Source / Destination / Protocol / Contract / Auth / Format / Failure / Owner / Tests) is written at P1 design.

Infra (via shared): PostgreSQL (truth), Redis (cache/rate), RabbitMQ (events) — owned by shared bootstrap, consumed through documented contracts only.
Trust boundary: everything outside the module handler is untrusted. Tenant isolation via tenant_id on every query. See docs/SECURITY.md.
