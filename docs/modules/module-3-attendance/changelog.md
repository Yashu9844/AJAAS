# Module 3 — Changelog

2026-09-30 — Created: 17-file doc bundle scaffold. Impact: docs only.
2026-10-08 — P1 design DONE: scope re-defined to Attendance, Shifts & Time Tracking (D3-01); specification/architecture/connections/security/goldens/testing/decisions/assumptions/files frozen. Impact: contracts frozen for P2 (19 endpoints, 6 tables, 5 events). Tests: n/a (docs).
2026-10-08 — P2 DONE: 6 GORM models + constants, models_test (table names, index contracts, append-only, UUID hooks), migrations 000025–000030. Tests: go test ./internal/attendance/models (100% cov). Impact: new tables only, no existing contract changed.
2026-10-09 — P3 DONE: calc engine (goldens G4/G5), DTOs (+G13 pagination clamp), validators. Tests: go test ./internal/attendance/... (calc/dto/validators/models 100%). Impact: none external.
2026-10-09 — P4 DONE: repositories (6 interfaces + GORM impls, advisory lock, FindOrCreate, status aggregate). Tests: build/vet (SQL verified live at P7). Impact: none external.
2026-10-09 — P5 DONE: services (shift, assignment, punch, query, regularization, outbox relay) + events; unit goldens G2/G3/G6/G7/G8/G10/G11/G12 green; services 91.8%. Impact: none external.
