# Module 3 — Current Goal

Goal: G3-4 repositories — interfaces + GORM implementations for shifts, assignments, records, punches, regularizations and outbox; `tenant_id` on every query; `tx *gorm.DB` parameter on every method; `LockEmployee` (pg_advisory_xact_lock); record `FindOrCreate`; summary aggregate query.
Why: P5 services are written against these interfaces (fakes in unit tests, real impls live).
Scope: backend/internal/attendance/repositories/**.
Non-goals: business rules (services), HTTP.
Success Criteria:
- [ ] build/vet/gofmt clean; `var _ Interface = (*impl)(nil)` assertions compile
- [ ] every query filters tenant_id (review against security.md AS-T3)
- [ ] live behavior covered by P7 goldens (no Postgres in unit ring)
- [ ] plan/status/todo/handoff/changelog updated
