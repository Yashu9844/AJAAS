# M2 Projects & Tasks

**Status:** phases 1-5 BUILT (2026-10-28/29): projects, tasks, collaboration, planning, group rules, custom fields, webhooks and Git
integrations, automation rules, recurring tasks, public links, CLI and MCP server, AI hooks, and a product UI at `/workspace`.
Not built: per-user API tokens (an M0 permission-model decision), a people directory for names in the UI, Gantt/saved views/bulk actions in
the product UI. Branch `module-2`.

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
| Custom fields and task values | 6 | `.../fields` |
| AI hooks (read-only seams for M9) | 5 | `.../aihooks` |
| Webhooks, chat notifications, Git integrations | 14 | `.../webhooks` |
| Automation rules, recurring tasks, public links | 15 | `.../automation` (+ `automation/bridge`), `.../recurrence`, `.../publiclink` |
| **Total** | **148** (all in `backend/api/swagger.yaml`) | |

Shared inside M2: `kernel` (errors, ranks, cursors, outbox events, activity), `api` (authorizer, task resolver, hooks), `ports`
(People), `testdb`. 28 migrations in `backend/migrations/projects`. Console: `/dev-test/projects` (nine pages). Product UI: `/workspace` (`docs/modules/m2-projects-tasks/notes-ui.md`). Command line: `backend/cmd/jaas-pm`, MCP server: `backend/cmd/jaas-pm-mcp`.

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
| `notes-webhooks.md`, `notes-automation.md`, `notes-ui.md` | per-feature build notes: decisions, deviations, tests, gaps (decision numbers there are local; the project-wide ones are in `decisions.md`) |
| `ai-readiness.md` | the AI seams M2 offers (tool catalogue, context packs, document feed, agent attribution, MCP/CLI) and why; the AI design itself is in `docs/modules/m9-ai-assistant/ai-integration.md` |
| `reference-kaneo.md` | feature matrix against the open-source Kaneo reference (no code copied) |
| `model/` | owner bundle: proposed tables, ER and architecture diagrams (verbatim) |

## Run it

```bash
docker compose up -d postgres redis rabbitmq s3        # s3 is the S3-compatible dev store (port 9090)
cd backend && APP_ENV=development DATABASE_PASSWORD=postgres DATABASE_DBNAME=jaas_dev go run ./cmd
cd frontend && npm run dev                              # test console at /dev-test/projects
```
