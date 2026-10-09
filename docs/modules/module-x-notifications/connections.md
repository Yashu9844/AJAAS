# Module 7 — Connections

Depends On: Modules 0, 4, 5, 6, 8, 9 (events).
Consumed By: Modules 10, 11.

Dependency types per edge: Runtime / API / Data / Configuration / Authentication / Infrastructure / Testing.
Full edge table (Source / Destination / Protocol / Contract / Auth / Format / Failure / Owner / Tests) is written at P1 design.

Infra (via shared): PostgreSQL (truth), Redis (cache/rate), RabbitMQ (events) — owned by shared bootstrap, consumed through documented contracts only.
Trust boundary: everything outside the module handler is untrusted. Tenant isolation via tenant_id on every query. See docs/SECURITY.md.
