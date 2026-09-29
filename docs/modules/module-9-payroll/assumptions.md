# Module 9 — Assumptions

- Provider contracts (Module 0 (User, RBAC)) will be stable before P2 implementation starts.
- Module 0 Tenant/User/RBAC semantics apply unchanged (single shared PG, tenant_id isolation, subdomain routing).
- Consumers (Modules 7, 10, 11) rely only on contracts frozen at P1, never on implementation internals.
