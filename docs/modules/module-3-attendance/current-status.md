# Module 3 — Current Status (updated: 2026-10-08, P1 DONE, P2 CURRENT)

Implemented:
- P1 design bundle: specification (FR-SH/SA/PU/AR/RG/EV, AT-001..AT-022, 19 endpoints, 6 tables, 5 events), architecture, connections C1–C10, security, goldens G1–G13, testing, decisions D3-01..D3-12, assumptions.

Partially Implemented:
- None.

Not Implemented:
- All code (P2–P8).

Known Issues:
- Module 4 bundle still describes Projects; must be re-scoped to Leave (D3-01) before G4-1.
- Inherited from Module 0 (not fixed here): RequirePermission grants permissions of soft-deleted roles; `/tenants` unauthenticated. Module 3 relies on RequirePermission as-is.

Blocked:
- P8 frontend needs an identity login shell (no frontend code exists yet).

Technical Debt:
- None yet.
