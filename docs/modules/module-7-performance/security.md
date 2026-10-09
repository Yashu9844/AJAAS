# Module 7 — Security

Permissions: `performance:read`, `performance:manage`. Self/manager access is decided per request (PM-008) and never trusts ids from the body for identity.

| ID | Threat | Control |
|---|---|---|
| PS-T1 | Review text/ratings leaking | PM-017: no ratings/scores/potential/text in events or audit |
| PS-T2 | Reading colleagues' goals | PM-008 owner/manager/permission; others 404 |
| PS-T3 | Review forgery | only the assigned reviewer (JWT → employee) submits; others 404 (PM-012) |
| PS-T4 | Peer de-anonymisation | reviewer id omitted for peer reviews shown to the subject (PM-014) |
| PS-T5 | Retro-editing after review | cycle state locks goals (PM-003); reviews immutable once submitted |
| PS-T6 | Weight inflation race | advisory lock per (cycle, owner) (PM-004) |
| PS-T7 | Cross-tenant IDOR | tenant-scoped repositories; 404 |
