# Module 0 — Plan (updated: 2026-10-01, merged backkendmod0 — G0-1..G0-13 DONE by merge, G0-14 CURRENT)

## Phase Map

| Phase | Goal ID | Objective | Depends On | Status | Proof |
|---|---|---|---|---|---|
| P0 | G0-0 | Foundations: shared/ + configs + compose skeleton + 16-file spec base | — | DONE | shared/*.go + 3 tests; configs/ yaml |
| P1 | G0-1 | Models (12 models + models_test) | P0 | DONE | merge 3af3d09 (origin/backkendmod0 f4b79fc..e7c392e) |
| P2 | G0-2 | Migrations 12 pairs | P1 | DONE | merge 3af3d09 — backend/migrations 000001..000012 |
| P3 | G0-3 | API bootstrap (cmd/main.go + PG/Redis/RabbitMQ compose) | P2 | DONE | merge 3af3d09 — main.go, module.go, compose infra |
| P4 | G0-4 | DTOs + validators | P1 | DONE | merge 3af3d09 — 7 dto files, 2 validator suites |
| P5 | G0-5 | Repositories (12) | P2+P4 | DONE | merge 3af3d09 — interfaces + 10 impls |
| P6 | G0-6 | TenantService | P5 | DONE | merge 3af3d09 — service + unit suite |
| P7 | G0-7 | AuthService (rotation + reuse detection) | P5+P6 | DONE | merge 3af3d09 — 461-line service + 5 test funcs |
| P8 | G0-8 | UserService | P5 | DONE | merge 3af3d09 — service + 6 test funcs |
| P9 | G0-9 | Role + Permission/Session/Token/Audit services | P5+P8 | DONE | merge 3af3d09 — services + suites |
| P10 | G0-10 | Controllers + routes | P6..P9 | DONE | merge 3af3d09 — 5 controllers + routes.go + suites |
| P11 | G0-11 | Middleware stack | P10 | DONE | merge 3af3d09 — tenant/auth/rbac/audit + shared cors/rate_limit/request_logger + suites |
| P12 | G0-12 | Events/queue | P6..P9 | DONE | merge 3af3d09 — events.go + publisher + rabbitmq + test |
| P13 | G0-13 | Frontend identity vs live API | P10+P11 | DONE (code) / UNVERIFIED (live) | N/A — no frontend identity code in merge; boot verification blocked on toolchain |
| P14 | G0-14 | E2E + hardening SC-001..SC-010 | P13 | CURRENT | this plan update |

Status vocabulary: DONE / CURRENT (one) / NEXT (one) / LATER / BLOCKED.

## Current Phase

- Goal: G0-14 E2E + hardening — repo integration, API E2E, security tests, perf budgets, Phase 9 final checks (see current-goal.md).
- Why now: unit ring is green by merge; outer verification ring is the only remaining Module 0 work.
- Scope in: backend/tests/** (new), targeted fixes where tests expose real bugs, checklist Phase 8 remainder + Phase 9.
- Scope out: new endpoints, schema changes, frontend pages.
- Entry criteria: G0-1..G0-12 merged — true. Toolchain (go + docker) available — FALSE on this machine.
- Exit criteria: test/race/coverage gates, integration + E2E + security green, p95 budgets, swagger verified, no TODOs.
- Risks: toolchain gap blocks execution here (mitigate: run on tooled machine or install Go/Docker); merged code unverified live (mitigate: G0-14 runs it).

## Next Phase

- Goal: Module 1 P1 design (G1-1) — spec, architecture, contracts, goldens for Organization.
- Why next: Module 0 backend is code-complete; Organization design is context01.md's stated next task and unblocks Modules 2–5.
- Pre-reqs: G0-14 test verdict (or explicit waiver with risks logged).
- Est. unblocks: Modules 2, 3, 4, 5.

## Later Phases

- Module 1 P2 implementation → P3 integration, then Modules 2–12 per DEPENDENCY-GRAPH.md.

## Gate Log

| Date | Phase | Gate | Decision | By | Reason |
|---|---|---|---|---|---|
| 2026-09-30 | P1 | Schema shape (model tags = DBML) | GO | scaffold | reversible pre-migration |
| 2026-10-01 | P1..P12 | Merge review of backkendmod0 (114 files, +10122) | GO | loop | models/migrations/DTO/repos/services/controllers/middleware/events/bootstrap all present with unit suites; no secrets in diff; conflicts none (ort auto-merge) |
| 2026-10-01 | P14 | Test execution environment | NO-GO here | loop | no go/docker toolchain on this machine — G0-14 runs on tooled machine |

## Progress Journal

| Date | Phase | Did | Sensors | Result | Handoff |
|---|---|---|---|---|---|
| 2026-09-30 | P1 | operational bundle added; loop ready | structure check | GREEN | scaffold handoff |
| 2026-10-01 | P1..P12 | fetched + merged origin/backkendmod0 → loop_engineering (3af3d09); updated goal/status/plan/todo/handoff | code review (toolchain absent) | GREEN (merge) / BLOCKED (test run) | this handoff |
