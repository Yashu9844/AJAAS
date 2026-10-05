# Module 7 — Assumptions

- Provider contracts (Modules 0, 4, 5, 6, 8, 9 (events)) will be stable before P2 implementation starts.
- Module 0 Tenant/User/RBAC semantics apply unchanged (single shared PG, tenant_id isolation, subdomain routing).
- Consumers (Modules 10, 11) rely only on contracts frozen at P1, never on implementation internals.
