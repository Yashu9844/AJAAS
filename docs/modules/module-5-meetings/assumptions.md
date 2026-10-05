# Module 5 — Assumptions

- Provider contracts (Modules 2 (Employee), 1 (Organization)) will be stable before P2 implementation starts.
- Module 0 Tenant/User/RBAC semantics apply unchanged (single shared PG, tenant_id isolation, subdomain routing).
- Consumers (Modules 6, 7, 10, 11) rely only on contracts frozen at P1, never on implementation internals.
