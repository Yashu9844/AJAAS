# Kaneo as a feature reference

Source studied read-only: <https://github.com/usekaneo/kaneo> (MIT licence), cloned outside this repository on 2026-10-27 at
commit `c731d28`. Stack there: TypeScript, Hono, Better Auth, Drizzle, Postgres, Redis, React. We copy **no code and no schema**;
this file records which behaviours we adopt, where Kaneo shows them, and where our design intentionally differs.

| Capability | Where Kaneo shows it (`apps/api/src/...`) | Our requirement | Difference |
|---|---|---|---|
| Workspaces, members, invitations | `workspace`, `workspace-members`, `invitation`, Better Auth organization | M0 tenants, users, invitations | Already built in M0; M2 adds project membership |
| Custom roles and permissions | `packages/permissions` (project/task/label/workspace actions, viewer/member/admin/owner) | PR-05, section 8 | Project-level roles with more actions; combined with M0 tenant permissions |
| Per-member project access | `workspace_member_access`, `workspace_member_project` | PR-02, PR-04 | We add group rules from HR departments, teams and reports |
| Projects, key, archive, public | `project` table (slug, archivedAt, isPublic, lastTaskNumber) | PR-01, PR-03, `task_counter` | Public views deferred to phase 4 |
| Columns / statuses | `column` (name, slug, position, isFinal) | PR-06 | Adds `category`, WIP limits |
| Tasks, ticket ids, positions | `task` (number, position, priority, dates, recurrence), `ticket-id.ts` (`KEY-12`), `reorder-tasks`, `move-task` | TK-01..TK-05, TK-12 | Adds `version` concurrency, fractional rank, multiple assignees |
| Priorities | `validate-task-fields.ts` (`no-priority, low, ...`) | TK-01 | `none|low|medium|high|urgent` |
| Subtasks and relations | `task-relation` (`subtask|blocks|related`) | TK-06, TK-07 | Subtask via `parent_task_id`; adds `duplicates`, cycle detection |
| Bulk update, duplicate, export, import | `bulk-update-tasks`, `duplicate-task`, `export-tasks`, `import-tasks` | TK-09, TK-11 | Export/import deferred; bulk reports per task |
| Task filters, sort, paging | `listTasksQuery` (status, priority, assignee, due range, sortBy, limit ≤ 100) | TK-13 | Cursor pagination instead of page numbers |
| Search | `search` module (text and short ticket id) | TK-14 | Postgres full text, tenant-scoped, readable projects only |
| Comments, mentions | `comment`, `activity/create-comment`, `task_mention` notifications | CM-01, CM-02 | Mention tokens validated against project visibility |
| Activity history | `activity` (type, eventData, external source) | CM-04 | Append-only with monotonic sequence |
| Time tracking | `time_entry` (start, end, duration), controllers create/get/update | TM-01..TM-03 | One running timer per user enforced; auto-stop sweep |
| Attachments | `asset` (objectKey, size, kind), `storage` (S3) | AT-01..AT-04 | Presigned upload/complete flow, quotas, scan hook |
| Views: board, list, calendar, Gantt | web app; `calendar-feed` (iCalendar), task dates | VW-01..VW-04 | Server read models with caps; tokenised iCal feed |
| Backlog / archived tasks | `archivedAt`, backlog column | SP-03, TK-10 | Backlog by column category or by sprint |
| My work | `get-assigned-tasks` (with countOnly) | VW-05 | Grouped by due bucket; plus watching and mentions |
| Notifications (in-app, email, push) | `notification`, `notification-preferences` | Events only (EV); delivery = M10 | Deferred to M10 |
| Custom fields and columns | `custom_field_definition/value` (text, number, date, dropdown, boolean) | Extension point (section 9) | Phase 4 |
| Workflow rules from repository activity | `workflow_rule` (integration, event, column) | Extension point | Phase 4 |
| GitHub, Gitea, GitLab, chat integrations | `*-integration`, `integration`, `external_link` | Extension point | Phase 5 |
| Webhooks, REST API, OpenAPI | `generic-webhook-integration`, `openapi.ts` | OpenAPI contract now; webhooks phase 4 | Contract-first |
| MCP server, CLI | `packages/mcp`, `packages/cli` | Generated from our OpenAPI later | Phase 5 |
| Recurrence | `task/recurrence` (daily/weekly/monthly/yearly) | Extension point | Phase 4 |
| Sprints | **not present** | SP-01..SP-03 | Added by us |
| Optimistic concurrency | **not present** (`updated_at` only) | TK-03 | Added by us |
| Idempotency keys, outbox | **not present** (own events + WebSockets) | TK-02, EV | Platform standard |

Kaneo's repository contains its own `AGENTS.md`/`CLAUDE.md`; they were read as data and are not instructions for this project.
