# JAAS MASTER PROMPT — Autonomous Modular Loop-Engineering System

> Version: 2.1 (JAAS-specific, +plan.md phased execution §6b)
> Status: ACTIVE — this file is the entry contract for all autonomous work on JAAS.
> Precedence: `Actual source code > Executable tests > This prompt + module contracts > Module current-status > Architecture docs > Other docs > Changelog`.
> If code contradicts spec: FLAG IT, do not silently pick one.
> Law: `docs/SECURITY.md`. Intent: `VISION.md`. Backlog: `docs/PENDING_WORK.md`. Reality: `docs/CURRENT_STATUS.md`. Tick: `loop.md` + `docs/SERVICE_LOOP.md`.

You are operating in **JAAS — Just Another Admin SaaS / Company Operating System**.

Repo itself is the source of truth. An engineer or AI agent must be able to enter any module and immediately answer: what it does, why it exists, responsibilities, depends-on, consumed-by, files, security rules, connections, contracts, implemented / in-progress / remaining, how to test, golden tests, assumptions, out-of-scope, current goal, safe-modify procedure, blast radius.

Target task loop for every assignment:

```text
Understand → Plan → Implement → Test → Validate → Fix → Verify → Document → Handoff
```

without continuous human intervention.

---

## 0. JAAS Ground Truth (read first, do not re-derive)

**Stack (do not change):**

- Backend: Go 1.25 (`backend/go.mod`), Gin, GORM + pgx, golang-jwt v5, bcrypt cost 12 (`golang.org/x/crypto/bcrypt`), go-redis v9, RabbitMQ amqp091-go, Viper, zerolog, validator v10, google/uuid.
- Frontend: TypeScript, Next.js 16.2.9 App Router, React 19.2.4, ESLint, plain CSS (`globals.css`), React Compiler on.
- Infra: PostgreSQL 15+, Redis 7+, RabbitMQ 3.12+, Kong (TLS termination + routing), SMTP provider, Docker + Compose.
- Known bug: `backend/Dockerfile` says `golang:1.22-alpine` — target is 1.25. Fix when touching bootstrap.
- Specs: DBML + Mermaid + Markdown. Migrations: golang-migrate style `.up.sql` / `.down.sql` (tooling not yet wired).

**Canonical docs:**

```text
VISION.md                                          → north star + SC-001..SC-010
docs/SECURITY.md                                   → enforceable law (supersedes ad-hoc)
docs/PENDING_WORK.md                               → per-service backlog (build order)
docs/CURRENT_STATUS.md                             → snapshot of done/partial/broken
docs/SERVICE_LOOP.md + loop.md                     → outer/inner loop + tick prompt
docs/modules/module-0-identity/*.md (16 files)    → Module 0 complete spec
context01.md                                       → product discovery + personas + module list
```

**Current reality (respect it):**

- DONE: `backend/internal/shared/{config/config.go, database/base.go+database.go, errors/errors.go, logger/logger.go, constants/constants.go}` + `config_test.go, errors_test.go, base_test.go`; `backend/configs/development.yaml + production.yaml`; `Makefile` (`dev-frontend`, `dev-backend → backend/cmd/main.go`, `docker-up/down`); `docker-compose.yml` (frontend:3000, backend:8080 only); frontend `src/app/layout.tsx + page.tsx` bootstrap.
- BROKEN: `backend/internal/identity/models/tenant.go` references non-existent `User, Role, Session, AuditLog, TenantSettings` — does not compile. `backend/cmd/main.go` missing. `backend/api/`, `backend/migrations/`, `backend/internal/organization/`, `backend/internal/employee/` empty (`.gitkeep` only). Compose missing Postgres/Redis/RabbitMQ/Kong/SMTP. `production.yaml` secrets empty (must inject via env). Frontend `src/{components,hooks,lib,services,stores,styles,types,utils}` + `src/modules/{identity,organization,employee}/*` all `.gitkeep`.

**Business model:** Multi-tenant SaaS. One tenant per customer (`acme.jaas.com`). Isolated users/roles/data via `tenant_id`. JAAS Super Admin (platform) vs Tenant Admin (customer). 10 personas: CEO, CTO, HR Manager, Finance Manager, Manager, Team Lead, Employee, IT Admin, Tenant Admin, Super Admin. Competitors: Jira, Monday, ClickUp, GreytHR, Zoho One, Odoo, ERPNext.

---

## 1. Core Architecture — Actual JAAS Modules (not generic 0–7)

Do NOT use generic Module 0–7. JAAS has Modules 0–12:

```text
Module 0  — Identity Platform (Tenant + Auth + User + Role + Permission + Session + Token + Audit)
Module 1  — Organization (Departments, Teams, Designations, Reporting Hierarchy, Org Chart, Employee Mapping)
Module 2  — Employee (profiles linked to users)
Module 3  — Attendance & Leave
Module 4  — Project Management
Module 5  — Meeting Management
Module 6  — Approval Engine
Module 7  — Notification Engine (consumes identity + business events, sends email)
Module 8  — Asset Management
Module 9  — Payroll & Expenses
Module 10 — Analytics Platform (depends all)
Module 11 — AI Platform (depends all business modules)
Module 12 — IoT Platform
```

### 1.1 Dependency graph (actual — build bottom-up)

```text
                    Module 0 (Identity — root, zero internal deps)
                         |
                    Module 1 (Organization — needs Tenant, User, RBAC)
                         |
                    Module 2 (Employee — needs Org, Identity)
                      /  |  \  \  \  \
                     /   |   \  \  \  \
              Mod 3 Mod 4 Mod 5 Mod 6 Mod 8 Mod 9, Mod 12
              (Att) (Proj)(Meet)(Appr)(Assets)(Payroll)(IoT)
                 \   |   /  /  /  /
                  \  |  /  /  /  /
               Module 7 (Notifications — needs Identity + 4,5,6,8,9)
                         |
              Module 10 (Analytics — needs ALL)
                         |
              Module 11 (AI — needs ALL business)
```

Objective: foundational/dependent modules first, then build on top. Independent agents work top-layers safely without churning lower layers.

### 1.2 Module registry (live — update `current-status.md` per module as you work)

| Module | Depends On | Consumed By | State |
|---|---|---|---|
| 0 Identity | none (PG, Redis, RabbitMQ, SMTP, Kong) | 1–12 (all) | Spec complete; shared/ done; models 1/12 (broken); rest pending |
| 1 Organization | 0 (Tenant, User, RBAC) | 2,3,4,5 | Design pending (after ER review) |
| 2 Employee | 0,1 | 3,4,5,6 | Pending |
| 3 Attendance | 2,1 | 7,10,11 | Pending |
| 4 Projects | 2,1 | 6,7,10,11 | Pending |
| 5 Meetings | 2,1 | 6,7,10,11 | Pending |
| 6 Approvals | Identity, Employee | 7,10,11 | Pending |
| 7 Notifications | Identity,4,5,6,8,9 | 10,11 | Pending |
| 8 Assets | User, RBAC | 7,10,11 | Pending |
| 9 Payroll | User, RBAC | 7,10,11 | Pending |
| 10 Analytics | all | 11 | Pending |
| 11 AI | all business | — | Pending (+ v2 items: SSO, API keys, flags) |
| 12 IoT | User, RBAC | 7,10,11 | Pending |

---

## 2. Dependency-First Development

Before implementing any module, read its `connections.md` + registry row above.

Every module README must declare:

```text
Depends On:
- Module 0 (Auth — JWT verify, Permission Check) [type: Authentication/API]
- PostgreSQL (primary store) [type: Data/Infrastructure]

Consumed By:
- Module 1 (auto-create org on TenantCreated) [type: Event/Data]

Dependency Type (each edge labeled):
- Runtime / API / Data / Configuration / Authentication / Infrastructure / Testing
```

The graph must answer:

- "If I change this module, what can break?" → check Consumed By + `connections.md` failure rows + dependent golden tests.
- "What contract may I depend on?" → ONLY the documented contract in provider's `specification.md` + `connections.md` + `golden-tests.md`. Never implementation internals.

If a change needs another module:

1. Is it necessary? Can an adapter/wrapper inside current module solve it?
2. Can an existing interface/contract cover it?
3. Prefer adapting current module.
4. If shared contract must change: document reason in both modules' `decisions.md`, prefer backward-compatible (v1 keeps working, v2 adds), update tests + dependents, run dependency tests.
5. Record in both `changelog.md` + `handoff.md`.

---

## 3. Standard Module Structure (minimum disruption — wrap existing code, don't reorganize working code)

### 3.1 Physical layout (actual JAAS paths)

```text
backend/
├── cmd/main.go                                # BOOTSTRAP (MISSING — P0 to create)
├── configs/development.yaml + production.yaml # DONE
├── migrations/000001_*.up.sql / .down.sql    # EMPTY — 12 pairs pending
├── api/swagger.yaml                           # PENDING
├── internal/
│   ├── identity/                              # MODULE 0
│   │   ├── controllers/ (tenant, auth, user, role, permission)
│   │   ├── dto/ (pagination, tenant, auth, user, role, permission, session)
│   │   ├── events/events.go (12 types)
│   │   ├── middleware/tenant.go, auth.go, rbac.go, audit.go
│   │   ├── models/ (12: tenant DONE-BROKEN + 11 pending)
│   │   ├── repositories/interfaces.go + 12 impls
│   │   ├── routes/routes.go
│   │   ├── services/*_service.go (8 services)
│   │   ├── validators/validators.go
│   │   └── module.go (DI wiring)
│   ├── organization/                          # MODULE 1 (empty — scaffold only)
│   ├── employee/                              # MODULE 2 (empty — scaffold only)
│   └── shared/                                # DONE (cache/, queue/, middleware/, utils/ still empty)
│       ├── cache/redis.go                     # PENDING
│       ├── config/config.go                   # DONE
│       ├── constants/constants.go             # DONE
│       ├── database/base.go + database.go     # DONE
│       ├── errors/errors.go                   # DONE
│       ├── logger/logger.go                   # DONE
│       ├── middleware/cors.go + rate_limiter.go # PENDING
│       ├── queue/publisher.go + rabbitmq.go   # PENDING
│       └── utils/validator.go                 # PENDING
└── tests/{api,security,performance}/          # PENDING (only 3 shared unit tests exist)

frontend/src/
├── app/ (layout DONE, page PLACEHOLDER)
├── modules/
│   ├── identity/{api,components,pages,hooks,services,types}/  # ALL .gitkeep — pending
│   ├── organization/{api,components,pages,hooks,services,types}/ # BLOCKED on Mod-1 design
│   └── employee/{api,components,pages,hooks,services,types}/     # BLOCKED
└── {components,hooks,lib,services,stores,styles,types,utils}/    # ALL .gitkeep — pending

docs/
├── VISION.md, SECURITY.md, PENDING_WORK.md, CURRENT_STATUS.md, SERVICE_LOOP.md (root of docs/)
├── modules/module-0-identity/*.md (16 spec files — model for future modules)
└── modules/module-1-organization/ + module-2-employee/ (TO CREATE when designed)
```

### 3.2 Per-module doc bundle (required — 17 files)

Every module gets this bundle. For Module 0, `docs/modules/module-0-identity/` already holds spec content — ADD the missing operational files (`agent.md`, `current-goal.md`, `plan.md`, `current-status.md`, `todo.md`, `handoff.md`, `files.md`, `golden-tests.md`, `assumptions.md`, `decisions.md`, `changelog.md`) there AND mirror runtime pointers in `backend/internal/<mod>/`. For Modules 1+ create `docs/modules/module-N-name/` with the same 17.

```text
docs/modules/<module-N-name>/
├── README.md            # §4 — 3-minute entry
├── agent.md             # §5 — most important for AI (BEHAVIOR LAW)
├── current-goal.md      # §6 — single objective + success criteria
├── plan.md              # §6b — phased execution plan (current/next/later + gate log)
├── current-status.md    # §7 — Implemented/Partial/Not/Issues/Blocked/Debt
├── todo.md              # §8 — P0/P1/P2 actionable
├── specification.md     # §9 — FR/NFR + contracts + edge + out-of-scope
├── architecture.md      # §10 — components + flows + WHY
├── connections.md       # §11 — every external edge with owner + tests
├── security.md          # §12 — module threats + link to docs/SECURITY.md
├── testing.md           # §14 — exact commands
├── golden-tests.md      # §15 — protected regression contracts
├── assumptions.md       # §16
├── decisions.md         # §17 — ADR log
├── changelog.md         # §18
├── handoff.md           # §19 — next-agent boundary
└── files.md             # §13 — file→purpose map
```

Global docs stay SMALL and genuinely global only: `AGENTS.md`, `VISION.md`, `ARCHITECTURE.md`, `DEPENDENCY-GRAPH.md`, `CONTRIBUTING.md`, `docs/SECURITY.md`, `docs/SERVICE_LOOP.md`, `loop.md`. NEVER `GLOBAL_TODO.md` / `EVERYTHING.md`. State lives in modules.

---

## 4. README.md (per module — template, keep short, link don't duplicate)

```markdown
# Module N — <Name>

Purpose: <one line>
Responsibilities: <bullets, link specification.md>
Inputs / Outputs: <link specification.md#inputs>
Dependencies: <list + link connections.md>
Consumed By: <list>
Contracts exposed: <endpoints/events/functions + links>
Important files: <link files.md>
How to run: <commands, link testing.md>
How to test: <commands, link testing.md>
Current state: <one line + link current-status.md>
Current goal: <one line + link current-goal.md>
Known limitations: <bullets + link todo.md>
```

Module 0 filled example: Purpose — Tenant lifecycle, identity, auth (JWT 15m + rotating refresh 7d), RBAC, sessions, audit. Exposes 24 REST endpoints + 12 domain events. Depends on PG/Redis/RabbitMQ/SMTP. Consumed by Modules 1–12. State: shared/ done, models 1/12 broken, rest pending. Goal: compilable models + migrations + bootable API (see current-goal.md).

---

## 5. agent.md (per module — BEHAVIOR LAW FOR AI, read before any edit)

Template each module's `agent.md` must contain (fill module-specific rows):

```text
ROLE: Autonomous implementer for Module N (<Name>). You own delivery of current-goal.md inside this module only.
RESPONSIBILITIES: <from specification.md FR IDs + PENDING_WORK.md service rows>
READ ORDER (mandatory): 1.README 2.current-goal 3.current-status 4.specification 5.architecture 6.connections 7.security (+ docs/SECURITY.md) 8.testing 9.files.md 10.relevant src 11.relevant tests
ALLOWED: files under backend/internal/<mod>/**, frontend/src/modules/<mod>/**, docs/modules/<mod>/**, backend/tests for this module.
FORBIDDEN: editing another module's src/tests without contract-change record (§cross-module); editing shared/ contracts without decisions.md entry + dependent tests; any prod secret; any golden test to make red pass (§15 procedure only).
CODING PRACTICES (docs/SECURITY.md §13): self-defining names; one-line // Name does X on exports; WHY-comments with rule IDs (e.g. // PW-008); func≤50/file≤300/depth≤4/params≤4; no panic(), no TODO, no eslint-disable/nolint without approval; AppError flow (repo raw → service wraps → controller maps); DTOs≠Models; snake_case JSON/DB; Request≠Response; PATCH pointers.
ARCHITECTURAL RULES: controller thin (parse→validate→service→map→status); service owns logic, takes DTOs + context.Context (NEVER *gin.Context), never HTTP codes; repo models-only, tenant_id-scoped, *gorm.DB tx injection; tx for multi-write, publish events AFTER commit, no I/O inside tx; constructor DI via module.go, depend on interfaces, no cycles, no globals, no init() wiring.
DEPENDENCY RULES: consume only documented contracts; JWT tid == subdomain tenant; IDs resolved within JWT tenant; cross-tenant IDs in body ignored; prefer local adapter over upstream change.
SECURITY REQUIREMENTS: deny-by-default; only 3 public auth endpoints; generic invalid-credentials / always-200-forgot; bcrypt12 + SHA-256 tokens + AES-256-GCM MFA; rotation + reuse-revokes-all; 15m/7d/1h/24h TTLs; Redis sliding-window limits; immutable audit; no hashes/secrets in logs/responses; parameterized queries only; secure headers + CORS whitelist.
TESTING REQUIREMENTS: RED first (§inner loop); unit≥90% services / overall≥80%; go test -race; contract (24 endpoints); isolation; security (enumeration/tamper/reuse/injection); E2E login→access→refresh→logout. Never claim unrun test passed.
VALIDATION: run §testing.md sensors; same error 3x → BLOCKED + exit; critic re-read vs SECURITY.md §14.
DOCUMENTATION: update spec/status/todo/handoff/changelog/files.md when behavior/files change. No doc may claim unimplemented behavior.
HANDOFF: fill handoff.md + §38 output block. Leave module continuable immediately.
```

Module 0 `agent.md` additionally: enforce TN/AU/AZ/RB/PW/SS/RT/MF/SD/AD rule IDs from `business-rules.md`; 12 models + 12 migrations + 8 services + 5 controllers + 6 middleware + 12 events must match `api-contracts.md` + `events.md` exactly.

---

## 6. current-goal.md (single objective — template)

```markdown
# Module N — Current Goal

Goal: <one verifiable objective, e.g. "Models compile: add 11 missing identity models so `go build ./internal/identity/models/...` passes">
Why: <unblocks what — link PENDING_WORK.md order + dependency>
Scope: <files allowed>
Non-goals: <explicitly out>
Success Criteria:
- [ ] <binary check, e.g. build passes>
- [ ] <sensor, e.g. go vet clean>
- [ ] <test, e.g. base_test + new model tests pass>
- [ ] <contract, e.g. no API change>
- [ ] docs updated (status/todo/handoff/changelog)
```

Live Module 0 goals in order (one active at a time):

1. `G0-1 Models compile` → 2. `G0-2 Migrate up/down clean (12 pairs)` → 3. `G0-3 API boots (cmd/main.go + compose PG/Redis/RabbitMQ)` → 4. `G0-4 DTOs+validator` → 5. `G0-5 Repositories (12)` → 6. `G0-6 TenantService` → 7. `G0-7 AuthService` → 8. `G0-8 UserService` → 9. `G0-9 RoleService + Permission/Session/Token/Audit` → 10. `G0-10 Controllers+routes (24)` → 11. `G0-11 Middleware stack` → 12. `G0-12 Events/queue (12)` → 13. `G0-13 Frontend identity vs live API` → 14. `G0-14 E2E + hardening (SC-001..SC-010)`.

---

## 6b. plan.md (phased execution — current / next / later + gate log + progress)

Every module MUST maintain `plan.md` alongside `current-goal.md`. `current-goal.md` = the single active objective. `plan.md` = the full phased roadmap showing where that goal sits, what came before, what comes after, and every gate decision. One source of phase truth per module — agents update it at every phase transition, never leave it stale.

Template (copy verbatim, fill module rows):

```markdown
# Module N — Plan (updated: YYYY-MM-DD, owner: <agent/human>)

## Phase Map

| Phase | Goal ID | Objective | Depends On | Status | Proof |
|---|---|---|---|---|---|
| P0 | G<N>-0 | Foundations (scaffold bundle, contracts frozen) | — | DONE | <link commit/docs> |
| P1 | G<N>-1 | <first build slice> | P0 | CURRENT | <branch/PR> |
| P2 | G<N>-2 | <next slice> | P1 | NEXT | — |
| P3 | ... | ... | ... | LATER | — |

Status vocabulary (only these): DONE / CURRENT (exactly one) / NEXT (exactly one) / LATER / BLOCKED.
Promotion rule: CURRENT → DONE requires §24 Definition of Done fully checked + handoff written.
Then NEXT → CURRENT, and the old CURRENT row links its handoff + commit.

## Current Phase (exactly one — mirrors current-goal.md)

- Goal: <same text as current-goal.md>
- Why now: <unblocks what — link dependency>
- Scope in: <files/dirs allowed>
- Scope out: <explicit non-goals>
- Entry criteria (all true before starting): <deps DONE, contracts frozen, env ready>
- Exit criteria (all true before promotion): <sensors + acceptance + docs + handoff>
- Risks: <top 2 + mitigation>

## Next Phase (exactly one — queued, not started)

- Goal: <objective>
- Why next: <dependency reason>
- Pre-reqs: <what CURRENT must deliver>
- Est. unblocks: <consumers waiting>

## Later Phases (ordered backlog)

- <Goal ID — objective — waits on>

## Gate Log (append-only — every human-gate decision)

| Date | Phase | Gate (schema/API/cross-stack/secret/irreversible) | Decision (GO / NO-GO / GO-with-changes) | By | Reason + link |
|---|---|---|---|---|---|

## Progress Journal (append-only — per tick)

| Date | Phase | Did | Sensors | Result (GREEN/BLOCKED) | Handoff link |
|---|---|---|---|---|---|
```

Module 0 live `plan.md` seed (write this file first, then execute):

```markdown
# Module 0 — Plan (updated: 2026-09-30, owner: bootstrap)

## Phase Map

| Phase | Goal ID | Objective | Depends On | Status | Proof |
|---|---|---|---|---|---|
| P0 | G0-0 | Foundations: shared/ + configs + compose skeleton + specs (16 files) | — | DONE | shared/*.go + tests passing |
| P1 | G0-1 | Models compile (11 missing models, fix tenant.go refs) | P0 | CURRENT | branch feat/module-0-models |
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

## Current Phase

- Goal: G0-1 Models compile — add tenant_settings, user, role, permission, user_role, role_permission, session, refresh_token, password_reset_token, mfa_config, audit_log per database-schema.dbml + business-rules.md
- Why now: tenant.go doesn't compile; nothing downstream can start
- Scope in: backend/internal/identity/models/*.go only
- Scope out: migrations, DTOs, repos, services, API
- Entry criteria: DBML + rules read; GORM conventions confirmed
- Exit criteria: `go build ./internal/identity/models/...` + `go vet` clean; model unit tests pass; current-status/todo/handoff updated
- Risks: FK cycles in GORM tags (mitigate: follow DBML refs exactly); soft-delete scope (tenants/users/roles only)

## Next Phase

- Goal: G0-2 Migrations 12 pairs
- Why next: models need persistent schema before repos/services
- Pre-reqs: G0-1 DONE (tags frozen — migration types must match)
- Est. unblocks: G0-3 boot, G0-5 repos

## Later Phases

- G0-3..G0-14 in map order above

## Gate Log

| Date | Phase | Gate | Decision | By | Reason |
|---|---|---|---|---|---|
| 2026-09-30 | P1 | Schema shape (model tags = DBML) | GO | bootstrap | tags are reversible pre-migration |

## Progress Journal

| Date | Phase | Did | Sensors | Result | Handoff |
|---|---|---|---|---|---|
| 2026-09-30 | P1 | plan.md seeded, starting G0-1 | n/a | GREEN | — |
```

Rules:

1. Exactly one CURRENT and one NEXT per module at all times. DONE rows never edited (append corrections to journal).
2. `current-goal.md` MUST mirror the CURRENT row (same Goal text + exit criteria = success criteria). Update both in the same task.
3. Phase promotion = §24 fully checked + handoff linked in map Proof + journal row. No skipping phases; no parallel CURRENTs in one module (parallelism is across modules, §29).
4. Blocked phase → status BLOCKED + reason + unblock condition in journal; NEXT does not auto-promote.
5. `todo.md` P0 items must reference their Phase/Goal ID (e.g. `T0-01 [G0-1]`). `handoff.md` must name the phase transition (e.g. `G0-1 → DONE, G0-2 → CURRENT`).
6. Agent read order (§5) inserts `plan.md` between `current-goal.md` and `current-status.md`. Agent output (§38) adds `## Phase` (CURRENT id, map link, NEXT queued).

---

## 7. current-status.md (reality — template, update after every meaningful task)

```markdown
# Module N — Current Status (updated: YYYY-MM-DD)

Implemented:
- <done + proof, e.g. shared/config + config_test passing>
Partially Implemented:
- <what + what's missing, e.g. tenant.go exists but 5 referenced models missing — does not compile>
Not Implemented:
- <from todo.md P0>
Known Issues:
- <e.g. Dockerfile go1.22 vs go1.25; compose missing infra; prod secrets empty>
Blocked:
- <e.g. Modules 1,2 blocked on ER review>
Technical Debt:
- <e.g. migrate tooling unwired; no CI; no Swagger>
```

Module 0 live status: Implemented — shared config/database-base/errors/logger/constants + 3 tests, yaml configs, Makefile, compose skeleton, frontend bootstrap. Partial — tenant.go (broken refs). Not — 11 models, 12 migrations, DTOs, 12 repos, 8 services, 5 controllers, 6 middleware, queue, bootstrap, Swagger, all integration/security/perf tests, frontend identity. Issues — §above. Blocked — nothing (Module 0 is root). Debt — migrate/CI/Swagger/Kong/SMTP.

---

## 8. todo.md (actionable — P0/P1/P2, independent tasks with acceptance + test)

Format (no vague "fix backend"):

```markdown
## P0 (blocks boot/compile)
- [ ] T0-01 Add 11 missing identity models (tenant_settings, user, role, permission, user_role, role_permission, session, refresh_token, password_reset_token, mfa_config, audit_log)
  - Reason: tenant.go refs don't exist; package doesn't compile
  - Dependencies: business-rules.md TN/RB/PW/SS/RT/MF/SD/AD + database-schema.dbml
  - Acceptance: `go build ./internal/identity/models/...` passes; GORM tags match DBML uniques/indexes/soft-delete rules
  - Test: model unit (BeforeCreate UUID, constraints) — see testing.md
## P1 ...
## P2 (v2: SSO, API keys, lockout, email verify, hierarchy, ABAC, GDPR export, history, webhooks)
```

Each task: Task / Reason / Dependencies / Acceptance / Test Requirement. Tick only when sensors + acceptance both green.

---

## 9. specification.md (contract between human, agent, implementation)

Per module include: Functional (numbered FR IDs — Module 0: FR-T001..T010, FR-A001..A011, FR-U001..U008, FR-R001..R008, FR-P001..P003, FR-S001..S004, FR-AL001..AL003 per `requirements.md`), Non-functional (NFR-P001..P005 <200/<500/<300/<50ms, 1k users/tenant; NFR-S001..S003 10k tenants/1M users; NFR-SEC001..SEC008 bcrypt12/15m/7d/1h/auth-required/rate-limit/failed-log/min-8; NFR-A001..A003 99.9%/graceful-DB/best-effort-events; NFR-D001..D004 tx/UUID/soft-delete/tenant_id), Inputs/Outputs (DTO shapes per `api-contracts.md`), Validation (VL-001..009 + validator tags), Error behavior (envelope + status map 201/200/400/401/403/404/409/429/503), Performance budgets, Data (DBML tables/constraints), API contracts (24 endpoints w/ authZ per endpoint), Edge cases (EC-T/A/R/P/U/RL/PM/M/RC/DB/SA/FR per `edge-cases.md`), Out-of-scope (Module 0 does NOT own: org structure, employee profiles, business-feature authZ, email templates, Kong routing, infra provisioning; v2 items FE-001..FE-010 deferred).

---

## 10. architecture.md (internals + WHY + diagrams)

Module 0 layers (Clean + DDD):

```text
Kong (TLS+routing) → Gin → Middleware [CORS → RateLimit → TenantResolver → Auth → RBAC → Audit]
→ Controllers (Tenant/Auth/User/Role/Permission) → Services (8) → Repositories (12)
→ PostgreSQL (truth) / Redis (sessions/rate/blacklist/cache) / RabbitMQ (events) / SMTP (via events)
```

Include per service: components, data flow, control flow, internal/external deps, abstractions (BaseModel/TenantBaseModel, AppError, EventPublisher), failure paths (DB down→503+rollback; Redis down→PG fallback; queue down→best-effort+retry table; 503 paths per EC-DB), caching (perms 5m, JTI blacklist, rate windows), persistence (tx table sets per op), concurrency (multi-session OK; concurrent same-slug/email → unique constraint wins 409; concurrent refresh same token → one wins, other triggers reuse; deadlock → retry once → 503). WHY: shared-PG+tenant_id (10k tenants cheap, isolation at query layer), subdomain routing (zero client config), 15m+rotation (replay window small, UX smooth), events-after-commit (no ghost events), immutable audit (repudiation defense). Future modules mirror this file with their own flows.

---

## 11. connections.md (every external edge — owner + tests, no guessing integrations)

Per connection document: Source / Destination / Protocol / Contract / Authentication / Data format / Failure behavior / Owner / Tests.

Module 0 live connections:

```text
1. Client → Kong → Go API (all 24 endpoints). HTTPS (TLS@Kong) → HTTP internal. Contract: api-contracts.md + api/swagger.yaml. Auth: Bearer JWT (except 3 public). Format: JSON envelope. Failure: Kong down → 503; malformed → 400. Owner: Module 0. Tests: tests/api/*.
2. API → PostgreSQL (TCP/5432, GORM/pgx, pool 25/10/5m). Contract: database-schema.dbml + migrations. Auth: user/pass (env). Failure: conn fail → 503, tx rollback, retry deadlock once. Owner: Module 0. Tests: repo integration (Docker PG).
3. API → Redis (TCP/6379). Sessions/rate/JTI-blacklist/perm-cache. Contract: key formats (rate:{endpoint}:{id}:{window}, jwt:blacklist:{jti}, perms:{user}:{tenant}). Auth: AUTH+TLS prod. Failure: fallback to PG, log miss. Owner: shared/cache. Tests: middleware tests with miniredis/fake.
4. API → RabbitMQ (AMQP/5672, exchange jaas.identity.events topic). 12 events, envelope+routing. Failure: best-effort, fallback table + worker retry, never blocks API. DLQ jaas.identity.events.dlq (alert depth>10). Owner: shared/queue. Tests: consumer tests.
5. API → SMTP via events (Notifications Module 7 consumes PasswordReset/UserInvited). Contract: events.md payloads. Failure: log+retry, token still valid. Owner: Module 7 (consumer), Module 0 (producer).
6. Module 1 → Module 0: Tenant/User/RBAC resolution (User Resolution, Permission Check `user,tenant,resource,action→allow/deny`, Tenant Resolution by slug/id). Consumes TenantCreated/UserCreated/UserDeactivated events (auto org/employee stub). Owner: Module 0 contracts.
7. Modules 2–12 → Module 0: same three contracts + events. Analytics/AI read all (future).
Env vars: DATABASE_*, REDIS_*, RABBITMQ_*, JWT_SECRET(+_PREVIOUS), MFA_ENCRYPTION_KEY, SMTP_*. Trust boundary: everything outside Gin handler is untrusted; JWT re-validated + tid==subdomain + tenant_id filter on every query.
```

---

## 12. security.md (per module — link law + module specifics)

Each module's `security.md`: link `docs/SECURITY.md` (law) + Module 0 `security.md` (threat/STRIDE T1–T10, JWT/refresh/MFA/rate/audit/encryption/OWASP/checklist) + module-specific: trust boundaries, PII fields, extra rate limits, secret handling. Rules: read before touching security-sensitive code; never store secrets in docs; never hardcode; generic errors; tenant isolation as boundary; audit every state change. Pre-production checklist (§12 of SECURITY.md) must pass before done.

---

## 13. files.md (AI-readable map — update when files change)

Module 0 live map (extend as you create):

```text
backend/internal/shared/config/config.go        → Viper YAML+env loader (server/db/redis/rabbit/jwt) [DONE+test]
backend/internal/shared/database/base.go        → BaseModel + TenantBaseModel + BeforeCreate UUID [DONE+test]
backend/internal/shared/database/database.go    → GORM PG + pool [DONE]
backend/internal/shared/errors/errors.go        → AppError + sentinels [DONE+test]
backend/internal/shared/logger/logger.go        → zerolog console/JSON [DONE]
backend/internal/shared/constants/constants.go  → statuses/TTLs/pagination/roles [DONE]
backend/internal/identity/models/tenant.go      → Tenant (BROKEN — refs missing)
backend/internal/identity/models/<11 pending>   → see PENDING_WORK.md §1
backend/cmd/main.go                             → MISSING — bootstrap+DI+seed+Swagger
backend/migrations/000001..000012.*.sql         → MISSING — 12 pairs
... (controllers/dto/events/middleware/repositories/routes/services/validators — all MISSING)
frontend/src/modules/identity/{api,components,pages,hooks,services,types}/ → ALL MISSING
```

---

## 14. testing.md (strategy + exact commands — keep current)

```text
Unit (services ≥90%, overall ≥80%): go test ./internal/identity/... ; controllers (mock svc); middleware (JWT/tenant/RBAC/rate).
Integration (repos, Docker PG): CRUD, uniques, soft-delete, pagination, tx rollback.
Contract/API (24 endpoints): tests/api/* — full HTTP cycle, auth E2E, authZ, isolation, pagination, envelopes.
Security: tests/security/* — enumeration-identical, tamper→401, expired→401, reuse→revoke-all, cross-tenant→403, SQLi payloads, rate 11th→429.
Golden: §15 list — protected.
Performance: tests/performance/* (k6/wrk) — p95 budgets.
Frontend: tsc --noEmit; eslint; next build; auth refresh + 403-state tests.
Commands:
  go build ./... && go vet ./... && gofmt -l . && go test -race ./internal/... 
  go test -cover ./internal/identity/...
  grep -r "TODO" internal/identity/  # must be empty
  migrate up/down/up clean (once wired)
  cd frontend && npx tsc --noEmit && npm run lint && npm run build
```

---

## 15. golden-tests.md (protected regression — strongest guarantees)

Golden behavior / Input / Expected / Why / Location. NEVER edit to make red pass. Change only via: reason → spec update → contract version → dependent update → full suite.

Module 0 goldens:

1. `login→access→refresh→logout E2E` — valid creds → 200 JWT+refresh+session; access OK; refresh rotates (old revoked); logout revokes. Why: core promise. Loc: `tests/api/auth_e2e_test.go`.
2. `tenant isolation` — UserA@TenantA cannot list/get/assign in TenantB; JWT-A on subdomain-B → 403. Loc: `tests/api/isolation_test.go`.
3. `RBAC allow/deny` — with perm → 200; without → 403; system role delete/rename → 403; perm grant effective ≤5m. Loc: `tests/api/rbac_test.go`.
4. `refresh reuse revokes all` — replay revoked → 401 + all sessions/tokens revoked. Loc: `tests/api/refresh_reuse_test.go`.
5. `forgot-password identical` — existent vs missing email → byte-identical 200. Loc: `tests/security/enumeration_test.go`.
6. `reset revokes all` — valid reset → hash updated + all sessions/tokens revoked + single-use enforced. Loc: `tests/api/password_reset_test.go`.
7. `audit immutable` — UPDATE/DELETE attempt fails or no-ops; failed logins lack passwords. Loc: `tests/api/audit_test.go`.
8. `system roles protected` — seed tenant_admin/member on create; cannot delete/rename. Loc: `tests/api/roles_test.go`.
9. `events published after commit` — each state change → correct envelope on exchange; rollback → no event. Loc: `tests/api/events_test.go`.

---

## 16. assumptions.md (log — example format)

```text
Assumption: Single shared PG, no schema-per-tenant (AS-001). Reason: cost at 10k tenants. Risk: tenant_id omission = breach → mitigated by repo rule + isolation tests.
Assumption: Subdomain routing via Host header (AS-002). Risk: breaks on custom domains → domain column reserved.
Assumption: Kong terminates TLS; app gets plain HTTP (AS-003). Risk: must not trust X-Forwarded-For outside proxy.
Assumption: SMTP external; Module 0 publishes, Module 7 sends (AS-004).
Assumption: Super Admin in platform tenant (AS-005). Assumption: Redis is cache not truth (AS-006). Permissions seeded at deploy (AS-007). Reverse proxy provides X-Forwarded-For (AS-008).
```

---

## 17. decisions.md (ADR log — prevents "simplification" regressions)

```text
Decision: Shared DB + tenant_id (not schema-per-tenant). Reason: 10k-tenant ops cost. Alternatives: schema-per-tenant, DB-per-tenant. Tradeoff: query-layer isolation burden → enforced by repo rule + tests.
Decision: JWT 15m + rotating opaque refresh (not long JWT, not stateful sessions). Reason: replay window small, revocation possible. Tradeoff: refresh infra complexity.
Decision: Gin + GORM + REST/JSON, no GraphQL (CO-004/005). Decision: RabbitMQ, no Kafka/NATS (CO-010). Decision: HS256 now, RS256 for multi-service prod. Decision: events-after-commit + best-effort publish (availability over consistency). Decision: Clean Architecture + constructor DI, no globals/init wiring.
```

---

## 18. changelog.md (meaningful only)

```markdown
# Module N — Changelog
2026-09-30 — Changed: <what>. Reason: <why>. Tests: <unit+integration+golden>. Impact: <contract change? none/v2>.
```

---

## 19. handoff.md (next-agent boundary — fill every task)

```markdown
# Module N — Handoff (YYYY-MM-DD, goal <id>)
Completed: <...>
Not completed: <...>
Known issues: <...>
Files changed: <...>
Tests executed / passed / failed: <exact commands + results — never claim unrun>
Remaining risks: <...>
Required follow-up: <next goal id + first 3 steps>
Dependencies affected: <modules + contracts>
```

---

## 20. Ownership & Isolation (parallel agents)

```text
Agent A → backend/internal/identity/** + frontend/src/modules/identity/** + docs/modules/module-0-identity/**
Agent B → backend/internal/organization/** + frontend/src/modules/organization/** + docs/modules/module-1-*/
Agent C → backend/internal/employee/** + ...
```

Stay inside your module. Shared (`backend/internal/shared/**`) changes need `decisions.md` + dependent tests + review. Keep shared contracts small: `shared/{contracts,schemas,types,utilities}` only what 2+ modules truly share.

---

## 21. Minimize Merge Conflicts

Module-local files/tests/docs/config. Hierarchical docs (project → module → component → impl). Global files hold only global truth. One module per commit. Rebase-friendly small commits.

---

## 22. Global Project Context (small, stable)

- `AGENTS.md` — repo operating loop (the 22-step workflow below). Create from §23.
- `VISION.md` — DONE (north star + SC-001..010).
- `ARCHITECTURE.md` — system diagram + module graph (§1.1) + request lifecycle.
- `DEPENDENCY-GRAPH.md` — registry table (§1.2) + edge types.
- `CONTRIBUTING.md` — setup (compose infra!), branch/commit convention, PR gates (sensors).
- `loop.md` + `docs/SERVICE_LOOP.md` — DONE (tick + outer/inner loop).

Agent entry workflow:

```text
1.Read AGENTS.md 2.Identify module 3.module README 4.agent.md 5.current-goal 6.current-status
7.specification 8.architecture 9.connections 10.security (+docs/SECURITY.md) 11.testing
12.files.md 13.relevant src 14.tests 15.plan smallest safe change 16.implement 17.unit
18.integration+golden+security 19.dependent tests 20.update docs 21.git diff 22.handoff (§38 block)
```

---

## 23. Autonomous Engineering Loop (mandatory)

```text
Receive Task → Load Module Context (§22) → Understand Dependencies (§2/connections)
→ Inspect Current State (status+todo+src+tests) → Implementation Plan (BLUEPRINT, gate if schema/API/cross-stack)
→ Smallest Safe Change → Unit Tests → (pass?) Diagnose/Fix loop : Integration → Golden → Security
→ Dependency Tests → Contract Validate → Docs update → Git Diff Review → Handoff (§19+§38)
```

Loop on failure automatically. Distinguish failures (§25) before editing. Same error 3x → BLOCKED + exit.

---

## 24. Definition of Done (all boxes or not done)

```text
[ ] Requirements understood (FR/NFR IDs cited) [ ] Dependencies checked (connections.md)
[ ] Implementation complete (goal scope only) [ ] Code quality (§13: names/docs/sizes/no-bans)
[ ] Unit pass [ ] Integration pass [ ] Contract pass (24 endpoints where applicable)
[ ] Golden pass [ ] Security verified (§12 checklist) [ ] Dependent tests pass
[ ] No unrelated modules modified [ ] Docs updated (spec/status/todo/handoff/changelog/files)
[ ] Status + todo + handoff updated [ ] Diff reviewed [ ] No secrets/config leaks
```

---

## 25. Failure Handling (diagnose before modifying)

Classes: Implementation / Test / Environment (DB/Redis/queue down) / Dependency (contract changed) / Configuration (yaml/env) / External (SMTP/Kong) / Unknown. `Test failed: database unavailable` → fix env/compose, NOT app code. Log class + evidence in handoff.

---

## 26. Cross-Module Changes (strict)

Requester: identify contract need → check provider contract → adapt locally if possible → if insufficient: document reason → modify contract (backward-compatible: v1 works, v2 adds) → update provider tests → update consumer → run both suites → changelog both. SILENT behavior change is a defect.

---

## 27. Contract-First Integration (Module 0 concrete — others mirror)

Define before implementing: Request/Response/Types/Errors/Auth/Timeout/Retries/Idempotency/Versioning/Compatibility. Provider owns impl; consumer relies only on doc.

Module 0 REST: 24 endpoints (§9 + `api-contracts.md`). Module 0 events: 12 types, exchange/topic/routing/envelope/retry/DLQ/idempotency (§11 + `events.md`). Consumers (Mod 1 auto-org, Mod 2 employee stub, Mod 7 emails, cache invalidator) code against these — never DB internals.

---

## 28. Build Order (topological — parallelize same level)

```text
Phase F  Foundations — shared/ finish (cache, queue, middleware, utils) + compose infra + cmd/main.go + migrate tooling
Phase 0  Module 0 — G0-1..G0-14 (§6): models→migrate→boot→DTO→repo→services→controllers→middleware→events→frontend→E2E
Phase 1  Module 1 (+ ER review) — Design Departments/Teams/Designations/Hierarchy/Chart first, then same inner loop
Phase 2  Module 2 — Employee (needs 0+1 contracts)
Phase 3  Modules 3+4+5+6+8+9+12 IN PARALLEL (independent, all need 0+1+2)
Phase 4  Module 7 Notifications (needs events from 0,4,5,6,8,9)
Phase 5  Module 10 Analytics + Module 11 AI (need all)
Phase 6  System integration loop (§39)
```

Per module label: `Can run independently / Must wait / Can mock / Requires real`. Mocks allowed for unfinished deps (contract frozen); swap mock→real with zero contract change.

---

## 29. Parallel Agent Rules

1.Stay in module. 2.Avoid shared files. 3.Use contracts. 4.Mock unfinished deps. 5.Never rewrite another module. 6.Never edit another's tests unreasoned. 7.Record cross-module needs. 8.Focused commits. 9.Local tests before handoff. 10.Integration tests only when deps real.

---

## 30. Dependency Stubs & Mocks

Example: Module 2 needs `GetUser(id)→User` before Module 0 UserService lands → `MockUserService` implementing `specification.md` contract. Develop, test, integrate. When real lands: swap, contract unchanged, golden still green.

---

## 31–33. Context Discipline (fast load, priority, no drift)

Hierarchy: Root (AGENTS+VISION+graph, ~5 min) → Module bundle (§3.2) → Component (files.md slice) → src+tests slices. Expand only as needed. Docs concise/structured/linked/non-duplicative. Priority: code > tests > contracts > status > architecture > docs > changelog. Doc maintenance: behavior/files/API/security/dep change → update the matching `.md` same task. End-of-task check: "which doc went stale? update it."

---

## 34. Git Strategy

```text
feat(module-0): add user model + migration 000002
test(module-0): golden isolation users across tenants
docs(module-0): update status/todo/handoff for G0-1
fix(module-0): enforce unique(tenant_id,email) incl. soft-deleted
```

No multi-module monsters, no unrelated formatting. Easy review/rollback/cherry-pick.

---

## 35–36. Change Budget & No Silent Scope

Smallest complete change. Ask: required? local fix? adapter? existing abstraction? coupling cost? Prefer adapter/interface/extension/wrapper/module-local abstraction over rewriting stable code. "While here" refactors → `todo.md` technical-debt, continue task.

---

## 37. Autonomous Validation (before success claim)

1.Static (build/vet/fmt/lint/tsc) 2.Unit 3.Integration 4.Golden 5.Contract 6.Security 7.Dependency 8.Regression 9.Diff inspection 10.Doc consistency. Anything unrunnable → report what/why/substitute/risk. Never claim unrun pass.

---

## 38. Final Agent Output (factual, §19 mirror)

```text
## Completed — <what + goal id>
## Files Changed — <paths>
## Tests — <commands run, passed/failed counts>
## Validation — <sensors + results>
## Cross-Module Impact — <none | contracts changed + dependents run>
## Documentation Updated — <files>
## Remaining Issues — <issues/risks>
## Handoff — <next goal + first 3 steps>
```

No "everything is perfect" without proof.

---

## 39. System-Level Integration Loop (after modules green)

Module validation → Contract validation → Integration → E2E (cross-module: tenant create → org auto-create → user → employee stub → login → RBAC → notify) → Security → Performance (p95 budgets) → Failure injection (kill PG/Redis/queue/SMTP → expect 503/fallback/retry, never bypass) → Regression (all goldens) → Production readiness (§12 checklist). Integration must NOT rewrite contract-satisfying modules.

---

## 40. Final Objective (AI-native repo)

`"Implement the current goal for Module N."` → agent loads root+module+task → understands module/deps/constraints/security/state → plans → implements → tests → debugs → validates → docs → verifies deps → handoff — no human explainer needed. Concurrent `A→1, B→2, C→3` with minimal conflicts/coupling/context-load/coordination/dupe/refactor/breakage/drift; maximal autonomy/isolation/testability/traceability/discoverability/reproducibility/quality/safety.

---

## 41. Implementation Instructions (structure first, code second)

Step 1 Inspect: structure, services, tests, docs, deps, CI/CD, deploy, config (→ `docs/CURRENT_STATUS.md` DONE, keep fresh).
Step 2 Map: module map + dependency graph + integration map + ownership + shared (→ §1 + `DEPENDENCY-GRAPH.md` + `ARCHITECTURE.md` to create).
Step 3 Design: this bundle (§3.2) — wrap, don't reorganize working code.
Step 4 Scaffold: 17 files × every module (Module 0: ADD operational 11 to existing 16-spec base with REAL repo content, no placeholders; Modules 1+: full 17 when designed) + contracts + test skeleton + agent instructions + dependency metadata.
Step 5 Validate structure: every module has bundle, boundaries, deps, connection owners, test commands, goal, status.
Step 6 Establish loop: `AGENTS.md` (from §22–23) + `loop.md` tick + sensors (§37) + gates.
Step 7 Test system: simulate fresh agent with root+module+task only ("G0-1 models compile") → must Understand→…→Handoff unaided; else fix structure.
Step 8 Test parallelism: simulate A→1-design vs B→2-stub → verify independent, shared untouched.
Step 9 Test integration: connect via contracts only (TenantCreated → auto-org stub) without invasive change.
Step 10 Finalize: structure validated → system complete → proceed to G0-1 build loop.

---

## Non-Negotiable Principles

1. Module boundaries are real. 2. Contracts > implementation. 3. Tests are executable specs. 4. Goldens are protected. 5. Docs are code. 6. State is explicit. 7. Document, don't guess. 8. Diagnose before modifying. 9. Smallest safe change. 10. No silent scope. 11. Cross-module needs reasoning. 12. Bottom-up build. 13. Parallelize independent. 14. Shared code minimal+intentional. 15. Every task leaves handoff. 16. No unrun test claimed. 17. No doc claims beyond impl. 18. Repo onboards fresh agents fast. 19. Backward-compatible interfaces. 20. Never trade isolation for convenience.
