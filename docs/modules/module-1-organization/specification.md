# Module 1 — Specification

STATUS: DESIGN PENDING (plan.md P1). Nothing below is frozen.

Known scope: Departments, Teams, Designations, Reporting Hierarchy, Employee Mapping, Org Chart.
Purpose: Own company structure: departments, teams, designations, reporting hierarchy, org chart.

Dependencies: Module 0 (Tenant, User, RBAC). See connections.md.
Consumers: Modules 2, 3, 4, 5.

When P1 design runs, this file must contain (MASTER_PROMPT section 9):
Functional requirements (numbered FR IDs), non-functional (latency budgets per docs/SECURITY.md section 11),
inputs/outputs (DTO shapes), validation rules, error behavior (envelope + status map),
performance budgets, data tables/constraints, API contracts, edge cases, explicit out-of-scope.
