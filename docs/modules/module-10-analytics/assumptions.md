# Module 10 — Assumptions

- Provider contracts (All modules (read contracts + events)) will be stable before P2 implementation starts.
- Module 0 Tenant/User/RBAC semantics apply unchanged (single shared PG, tenant_id isolation, subdomain routing).
- Consumers (Module 11) rely only on contracts frozen at P1, never on implementation internals.
