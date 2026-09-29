# Module 3 — Specification

STATUS: DESIGN PENDING (plan.md P1). Nothing below is frozen.

Known scope: Check-in/out, shifts, leave types, leave requests, approvals hookup, balances.
Purpose: Track attendance, shifts, leave requests and balances.

Dependencies: Modules 2 (Employee), 1 (Organization). See connections.md.
Consumers: Modules 7, 10, 11.

When P1 design runs, this file must contain (MASTER_PROMPT section 9):
Functional requirements (numbered FR IDs), non-functional (latency budgets per docs/SECURITY.md section 11),
inputs/outputs (DTO shapes), validation rules, error behavior (envelope + status map),
performance budgets, data tables/constraints, API contracts, edge cases, explicit out-of-scope.
