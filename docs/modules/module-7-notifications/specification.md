# Module 7 — Specification

STATUS: DESIGN PENDING (plan.md P1). Nothing below is frozen.

Known scope: Event consumers (identity + business), templates hookup, delivery log, retries.
Purpose: Consume domain events and deliver email (later push/SMS) notifications.

Dependencies: Modules 0, 4, 5, 6, 8, 9 (events). See connections.md.
Consumers: Modules 10, 11.

When P1 design runs, this file must contain (MASTER_PROMPT section 9):
Functional requirements (numbered FR IDs), non-functional (latency budgets per docs/SECURITY.md section 11),
inputs/outputs (DTO shapes), validation rules, error behavior (envelope + status map),
performance budgets, data tables/constraints, API contracts, edge cases, explicit out-of-scope.
