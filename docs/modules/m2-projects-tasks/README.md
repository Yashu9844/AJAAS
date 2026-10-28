# M2 Projects & Tasks

**Status:** phases 1-3 BUILT and verified (2026-10-28). Phases 4-5 (custom fields, automation, webhooks, integrations, MCP/CLI,
product UI) are designed for in `specification.md` section 9 and not built. Branch `module-2`.

**Purpose:** project delivery and work tracking for a tenant: projects, tasks on boards/lists/calendars/Gantt, sprints, comments,
time tracking, attachments in object storage, My work and search. People are referenced by ids only; M2 never reads M0 or HR tables.

**Depends on:** M0 (identity, permissions, audit) through `internal/shared/identityport`; PostgreSQL (schema `projects`); an
S3-compatible object store through `internal/shared/storage`; RabbitMQ for events; HR organisation facts through an adapter
(`internal/hr/people.go`) for group rules. **Publishes:** `jaas.projects.events` (project, task, sprint, attachment, time events,
ids and versions only). **Consumes:** identity deactivation (in-process hook now, inbox-deduplicated handler ready for a broker).

## What exists

| Area | Operations | Package |
|---|---|---|
| Projects, members, roles, columns, labels, milestones | 35 | `backend/internal/projects/project` |
| Tasks, relations, subtasks, bulk, activity | 23 | `.../task` |
| Comments, mentions, watchers, time, attachments | 21 | `.../collab` |
| Sprints, backlog, board, calendar, Gantt, My work, search, saved views, feed tokens | 25 | `.../planning` |
| Group rules | 4 | `.../project` |
| **Total** | **108** (all in `backend/api/swagger.yaml`) | |

Shared inside M2: `kernel` (errors, ranks, cursors, outbox events, activity), `api` (authorizer, task resolver, hooks), `ports`
(People), `testdb`. 22 migrations in `backend/migrations/projects`. Console: `/dev-test/projects` (five pages).

## Files in this folder

| File | Holds |
|---|---|
| `specification.md` | the contract: requirement ids (PR, TK, CM, TM, AT, SP, VW, EV, SC), extension points, golden tests (frozen; deviations are listed at its end) |
| `design.md` | service boundary, packages, data model, mechanisms, storage layer, API groups, performance, and an "as built" section |
| `decisions.md` | owner answers and every decision (DPJ-01..) with reason and alternatives |
| `plan.md` | steps P0-P8 with status and known gaps |
| `testing.md` | test inventory, golden-test evidence, commands |
| `security.md` | threat to evidence table |
| `notes-tasks.md`, `notes-collab.md`, `notes-planning.md` | per-context build notes: decisions, deviations, tests, gaps |
| `reference-kaneo.md` | feature matrix against the open-source Kaneo reference (no code copied) |
| `model/` | owner bundle: proposed tables, ER and architecture diagrams (verbatim) |

## Run it

```bash
docker compose up -d postgres redis rabbitmq s3        # s3 is the S3-compatible dev store (port 9090)
cd backend && APP_ENV=development DATABASE_PASSWORD=postgres DATABASE_DBNAME=jaas_dev go run ./cmd
cd frontend && npm run dev                              # test console at /dev-test/projects
```
