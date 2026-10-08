# Module 3 — Handoff (2026-10-09, G3-4 DONE → G3-5 CURRENT)

Completed: P2 models+migrations; P4 repositories (6, tenant-scoped, advisory lock); P3 calc engine (goldens G4/G5), DTOs (G13 clamp), validators — all 100% covered.
Not completed: P5–P8.
Known issues: migrations unexecuted until P7 (Docker); Module 4 re-scope pending (D3-01).
Files changed: backend/internal/attendance/models/*, backend/migrations/00002[5-9]*, 000030*, module-3 docs.
Tests executed: `go test -cover ./internal/attendance/...` — calc, dto, validators, models PASS (100% each).
Remaining risks: GORM vs SQL drift (P7 check).
Required follow-up (G3-5): (1) ports + events + in-memory fakes, (2) shift/assignment services with G10, (3) punch/query/regularization/outbox services with G2,G3,G6,G7,G8,G11,G12.
## Phase: P4 → DONE, P5 → CURRENT, P6 → NEXT.
