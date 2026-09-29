# Module 2 — Specification

STATUS: DESIGN PENDING (plan.md P1). Nothing below is frozen.

Known scope: Employee profiles, employment details, reporting links, status sync on UserDeactivated.
Purpose: Own employee profiles linked to identity users, employment details, status sync.

Dependencies: Modules 0 (Identity), 1 (Organization). See connections.md.
Consumers: Modules 3, 4, 5, 6.

When P1 design runs, this file must contain (MASTER_PROMPT section 9):
Functional requirements (numbered FR IDs), non-functional (latency budgets per docs/SECURITY.md section 11),
inputs/outputs (DTO shapes), validation rules, error behavior (envelope + status map),
performance budgets, data tables/constraints, API contracts, edge cases, explicit out-of-scope.
