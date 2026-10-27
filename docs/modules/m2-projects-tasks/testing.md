# M2 Projects & Tasks — testing

Golden tests `PG1`–`PG12` are defined in `specification.md` section 10 and become integration tests (real Postgres, MinIO for attachments) plus console scenarios.

Additional required coverage:

- Unit tests with fakes for every service, including failure injection on every store call (rollback proven).
- Race tests: parallel creates (numbers), parallel updates (versions), parallel moves (ranks), parallel relation inserts (cycles), parallel sprint starts, parallel timer starts.
- Authorization matrix: for each role (viewer, member, manager, owner, custom, tenant admin, non-member, other tenant) × each operation group, the expected 200/403/404.
- Storage: init/complete happy path, size or type mismatch, expired URL, cross-tenant key probing, cleanup sweeps.
- Pagination: stable cursors under concurrent inserts; max page size enforced.
- Privacy: events and audit contain ids only; mention filtering; deleted comments leave no body.
- Performance: seeded 10 × 50 000 tasks; board, list, My work, Gantt, search budgets.
- Contract: `TestSwaggerMatchesRoutes` and `TestImportBoundaries` include M2.
