# Module 1 — Assumptions

- Provider contracts (Module 0 (Tenant, User, RBAC)) will be stable before P2 implementation starts.
- Module 0 Tenant/User/RBAC semantics apply unchanged (single shared PG, tenant_id isolation, subdomain routing).
- Consumers (Modules 2, 3, 4, 5) rely only on contracts frozen at P1, never on implementation internals.
