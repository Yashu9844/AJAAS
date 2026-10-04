# Module 1 — Assumptions (P1 update)

- Provider contracts (Module 0 Tenant/User/RBAC + events envelope) are as merged in 3af3d09. G0-14 may surface fixes; if a Module 0 contract changes, this spec's FR-C/E rows are re-gated (decisions.md entry, no silent drift).
- Module 0 Tenant/User/RBAC semantics unchanged (shared PG, tenant_id isolation, subdomain routing, generic auth errors).
- Consumers (Modules 2, 3, 4, 5) rely only on contracts frozen here (spec §4 DTOs, connections C9), never on P2 implementation internals.
- Redis + RabbitMQ available via shared bootstrap (same pattern as Module 0); chart cache degrades to compute-direct.
- `organization:read/update` permissions can be seeded through Module 0 permission seed at P2 (cross-module change record required).
