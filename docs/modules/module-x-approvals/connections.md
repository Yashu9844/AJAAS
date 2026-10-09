# Approvals (unnumbered; was Module 6, see module-6-recruitment D6-01) — Connections

Depends On: Module 0 (Identity), Module 2 (Employee).
Consumed By: Modules 7, 10, 11.

Dependency types per edge: Runtime / API / Data / Configuration / Authentication / Infrastructure / Testing.
Full edge table (Source / Destination / Protocol / Contract / Auth / Format / Failure / Owner / Tests) is written at P1 design.

Infra (via shared): PostgreSQL (truth), Redis (cache/rate), RabbitMQ (events) — owned by shared bootstrap, consumed through documented contracts only.
Trust boundary: everything outside the module handler is untrusted. Tenant isolation via tenant_id on every query. See docs/SECURITY.md.
