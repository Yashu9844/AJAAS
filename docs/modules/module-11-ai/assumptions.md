# Module 11 — Assumptions

- Provider contracts (All business modules) will be stable before P2 implementation starts.
- Module 0 Tenant/User/RBAC semantics apply unchanged (single shared PG, tenant_id isolation, subdomain routing).
- Consumers (None (top of graph)) rely only on contracts frozen at P1, never on implementation internals.
