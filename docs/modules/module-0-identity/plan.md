# Module 0 — Plan (updated: 2026-09-30, owner: loop_engineering scaffold)

## Phase Map

| Phase | Goal ID | Objective | Depends On | Status | Proof |
|---|---|---|---|---|---|
| P0 | G0-0 | Foundations: shared/ + configs + compose skeleton + 16-file spec base | — | DONE | shared/*.go + 3 tests passing; configs/ yaml |
| P1 | G0-1 | Models compile (11 missing models, fix tenant.go refs) | P0 | CURRENT | branch loop_engineering |
| P2 | G0-2 | Migrations 12 pairs up/down clean | P1 | NEXT | — |
| P3 | G0-3 | API boots (cmd/main.go + PG/Redis/RabbitMQ in compose) | P2 | LATER | — |
| P4 | G0-4 | DTOs + validator | P1 | LATER | — |
| P5 | G0-5 | Repositories (12) | P2+P4 | LATER | — |
| P6 | G0-6 | TenantService | P5 | LATER | — |
| P7 | G0-7 | AuthService | P5+P6 | LATER | — |
| P8 | G0-8 | UserService | P5 | LATER | — |
| P9 | G0-9 | Role + Permission/Session/Token/Audit services | P5+P8 | LATER | — |
| P10 | G0-10 | Controllers + routes (24 endpoints) | P6..P9 | LATER | — |
| P11 | G0-11 | Middleware stack (6) | P10 | LATER | — |
| P12 | G0-12 | Events/queue (12 types) | P6..P9 | LATER | — |
| P13 | G0-13 | Frontend identity vs live API | P10+P11 | LATER | — |
| P14 | G0-14 | E2E + hardening SC-001..SC-010 | P13 | LATER | — |

Status vocabulary: DONE / CURRENT (one) / NEXT (one) / LATER / BLOCKED. Promotion needs MASTER_PROMPT §24 + handoff. No skipping.

## Current Phase

- Goal: G0-1 Models compile — tenant_settings, user, role, permission, user_role, role_permission, session, refresh_token, password_reset_token, mfa_config, audit_log per database-schema.dbml + business-rules.md.
- Why now: tenant.go does not compile; nothing downstream can start.
- Scope in: backend/internal/identity/models/*.go only.
- Scope out: migrations, DTOs, repos, services, API.
- Entry criteria: DBML + TN/RB/PW/SS/RT/MF/SD/AD rules read — true.
- Exit criteria: build + vet clean; model tests pass; status/checklist/handoff updated.
- Risks: GORM tag cycles (follow DBML refs exactly); soft-delete scope (tenants/users/roles only).

## Next Phase

- Goal: G0-2 Migrations 12 pairs (tenants → audit_logs, down in reverse).
- Why next: models need persistent schema before repos/services.
- Pre-reqs: G0-1 DONE (tags frozen — migration types must match).
- Est. unblocks: G0-3 boot, G0-5 repos.

## Later Phases

- G0-3..G0-14 in map order. P4 (DTOs) may run parallel with P2/P3 after P1 — same agent or second agent on dto/ only.

## Gate Log

| Date | Phase | Gate | Decision | By | Reason |
|---|---|---|---|---|---|
| 2026-09-30 | P1 | Schema shape (model tags = DBML) | GO | scaffold | reversible pre-migration |

## Progress Journal

| Date | Phase | Did | Sensors | Result | Handoff |
|---|---|---|---|---|---|
| 2026-09-30 | P1 | agent/goal/plan/status/todo/handoff/files operational bundle added; loop ready | structure check | GREEN | this handoff |
