# M2 Projects & Tasks — Design (awaiting approval)

Contract: `specification.md`. Reference: `reference-kaneo.md`. Decisions and open questions: `decisions.md`.

## 1. Service boundary (microservice, same repository)

M2 is its own service in the Go monorepo, built the way M1 was prepared for extraction:

```
backend/internal/projects/            M2 service root (replaces the empty placeholder package)
  module.go                           composition root: wires contexts, ports, routes, relays
  project/   task/   planning/        bounded contexts (below)
  api/                                the only package other modules may import (DTOs, read ports)
  ports/                              interfaces M2 needs: Identity, People, Storage, Clock
backend/internal/shared/storage/      NEW shared S3-compatible object storage port + MinIO/S3 adapter
backend/internal/shared/identityport/ the HR identityport interface promoted to shared (HR keeps an alias package)
backend/migrations/projects/          NEW migration directory; Postgres schema `projects`
backend/cmd/main.go                   all-in-one dev/test binary registers the module (unchanged pattern)
backend/cmd/projects/                 per-service entry point added with the gateway phase (not now)
```

Rules that make it a real boundary: its own Postgres schema and migrations; no imports of `internal/identity/...` or
`internal/hr/...` (the import-boundary test is extended to M2); users and people only through ports; events through
its own outbox and inbox; its own `/api/v1/projects/...` routes and OpenAPI section; its own console page.

**Ports (M2 needs from others)**

| Port | Purpose | Adapter now | Adapter later |
|---|---|---|---|
| `Identity` | authenticate, tenant permission check, audit log, user exists/active | in-process (same as HR) | JWT claims + HTTP |
| `People` | resolve display name/avatar for ids; expand a department/team/reports-of into user ids; is user active | in-process via `hr/api` + `identity` read ports | HTTP to HR/M0 |
| `Storage` | presign upload/download, head, delete objects | S3-compatible (MinIO in dev) | same |
| `Clock` | testable time | system clock | — |

## 2. Bounded contexts

| Context | Owns |
|---|---|
| `project` | projects, members, group rules, project roles, columns, labels, milestones, favorites |
| `task` | tasks, assignees, relations, comments, mentions, watchers, activity, time entries, attachments |
| `planning` | sprints, backlog, saved views, board/list/calendar/Gantt/my-work read models, search, iCalendar tokens |

Each context follows the M1 package pattern (models, store behind an interface, service, http, tests with fakes) so
services are unit-tested without a database; the integration ring tests the real Postgres behaviour.

## 3. Data model (schema `projects`; all ids UUID; every row has `tenant_id`)

| Table | Columns (key ones) | Notes |
|---|---|---|
| `project` | id, tenant_id, key, name, description, status, visibility, owner_user_id, icon, color, start_on, due_on, mode (`kanban|scrum`), task_counter, version, archived_at, deleted_at, created_at, updated_at | unique (tenant_id, key) among non-deleted |
| `project_role` | id, tenant_id, name, permissions text[], builtin bool | tenant-defined custom roles; built-ins seeded and immutable |
| `project_member` | tenant_id, project_id, user_id, role_id, source (`direct|group`), source_ref, active bool, added_by, added_at | PK (project_id, user_id); `active=false` when the user is deactivated |
| `project_group_rule` | id, tenant_id, project_id, kind (`department|team|reports_of`), ref_id, role_id | phase 3; resolved through the People port, refreshed on org events |
| `project_favorite` | tenant_id, user_id, project_id | |
| `board_column` | id, tenant_id, project_id, name, key, position, color, category, wip_limit, wip_hard, is_default | unique (project_id, key) |
| `label` | id, tenant_id, project_id NULL, name, color | project NULL = tenant-wide; unique per scope, case-insensitive |
| `milestone` | id, tenant_id, project_id, name, due_on, status | |
| `task` | id, tenant_id, project_id, number, parent_task_id, title, description, column_id, priority, reporter_user_id, start_on, due_at, estimate_minutes, milestone_id, sprint_id, rank, completed_at, archived_at, deleted_at, version, search tsvector, created_at, updated_at | unique (project_id, number); indexes below |
| `task_assignee` | tenant_id, task_id, user_id, assigned_by, assigned_at | PK (task_id, user_id) |
| `task_label` | tenant_id, task_id, label_id | |
| `task_relation` | id, tenant_id, source_task_id, target_task_id, type | unique (source, target, type); `blocks` cycle-checked |
| `task_watcher` | tenant_id, task_id, user_id, reason | |
| `task_ticket_alias` | tenant_id, old_key, old_number, task_id | keeps old ticket ids resolving after a project move |
| `comment` | id, tenant_id, task_id, author_user_id, body, edited_at, deleted_at, created_at | |
| `mention` | tenant_id, source_kind (`task|comment`), source_id, user_id | |
| `activity` | seq bigserial PK, id uuid, tenant_id, project_id, task_id NULL, actor_user_id, type, data jsonb, source, occurred_at | append-only (no update/delete grants in the app role) |
| `time_entry` | id, tenant_id, task_id, user_id, started_at, ended_at, minutes, note, source, auto_stopped | partial unique: one row per user with `ended_at IS NULL` |
| `attachment` | id, tenant_id, project_id, task_id, comment_id NULL, storage_key, filename, content_type, size, sha256, status (`pending|ready|deleted`), uploaded_by, created_at, deleted_at | |
| `sprint` | id, tenant_id, project_id, name, goal, start_on, end_on, status, started_at, completed_at, report jsonb, version | partial unique: one `active` per project |
| `saved_view` | id, tenant_id, project_id NULL, owner_user_id, name, layout, filters jsonb, sort jsonb, grouping, shared bool | |
| `calendar_token` | tenant_id, user_id, token_hash, created_at, revoked_at | iCalendar feed |
| `outbox_events` | as the shared outbox pattern | published by the shared relay |
| `inbox_events` | event_id PK, tenant_id, type, received_at, processed_at | consumer deduplication |
| `idempotency_records` | the shared table, in this schema | |

**Indexes** (the hot paths): `task (tenant_id, project_id, column_id, rank)` for boards; `task (tenant_id, project_id, status-category via column, updated_at)`;
`task_assignee (tenant_id, user_id)` joined to `task (due_at)` for My work; `task (tenant_id, project_id, due_at)` for calendar/Gantt;
GIN on `task.search`; `activity (tenant_id, task_id, seq)` and `(tenant_id, project_id, seq)`; `time_entry (tenant_id, task_id)` and `(tenant_id, user_id, started_at)`.

**Extension tables reserved by name, created in phase 4/5**: `custom_field_definition`, `custom_field_value`, `workflow_rule`,
`webhook`, `webhook_delivery`, `integration`, `external_link`, `task_recurrence`.

## 4. Key mechanisms

- **Ticket numbers:** `UPDATE project SET task_counter = task_counter + 1 ... RETURNING` inside the create transaction, so numbers are unique, ordered and never reused, even if the create later fails (a gap is not allowed because the counter bump rolls back with the transaction).
- **Optimistic concurrency:** `UPDATE task SET ..., version = version + 1 WHERE id = $1 AND version = $2`; zero rows means 409 `VERSION_CONFLICT` with the current version returned. Moves and bulk operations use the same predicate per task.
- **Ranking:** fractional string ranks (base-62 between two neighbours). Rank collisions or exhaustion trigger a bounded rebalance of one column in a transaction (advisory lock per column). A move touches one row.
- **Dependency cycles:** inside the relation insert transaction, lock the project's relation graph by advisory lock, walk `blocks` edges from the target with a depth/size cap (10 000 nodes) and refuse if the source is reachable.
- **Single active sprint / single running timer:** partial unique indexes, with 409 mapped from the constraint violation.
- **Soft delete and purge:** `deleted_at`; a daily sweep hard-purges after 30 days and deletes the objects.
- **Mentions:** parsed from markdown tokens on write, filtered by the author's view of the project's readers, stored in `mention`; the event lists the user ids that remain.
- **Search:** Postgres `tsvector` on title+description maintained by trigger; queries always include `tenant_id` and the set of project ids the caller can read (resolved once per request and cached for the request).
- **Authorization:** one `Authorizer` resolves (tenant permissions ∪ effective membership role) per project with a short per-request cache; effective membership merges direct rows and group-rule expansion. Every task/comment/attachment/time operation first resolves its project.

## 5. Storage layer (the "library like S3")

`internal/shared/storage` exposes:

```go
type Store interface {
    PresignPut(ctx, key string, contentType string, size int64, sha256 string, ttl time.Duration) (PutTarget, error) // URL + required headers
    PresignGet(ctx, key string, filename string, ttl time.Duration) (string, error)
    Head(ctx, key string) (ObjectInfo, error)   // size, content type, checksum, exists
    Delete(ctx, key string) error
}
```

Adapter: S3-compatible (AWS SDK v2) configured by endpoint, region, bucket and credentials from the environment; **MinIO** container in the dev compose
so developers and tests run it locally. Key layout `t/<tenant>/p/<project>/a/<attachment-id>`; buckets are private; only presigned URLs leave the system
(5-minute upload, 2-minute download). Unit tests use an in-memory fake; an integration test runs against MinIO. HR documents and M8 can adopt the same port later.

Upload flow: `POST /tasks/{id}/attachments` (init, authorised, writes a `pending` row, returns id + presigned PUT) → client PUT → `POST /attachments/{id}/complete`
(HEAD the object, compare size/type/checksum, scan hook, then `ready` and an activity entry). A sweep removes `pending` rows and objects older than 24 h.

## 6. API (OpenAPI section `Projects`; about 100 operations in 3 phases)

| Group | Operations (indicative) |
|---|---|
| Projects | create, list, get, update, archive, restore, delete, favorite/unfavorite; members list/add/change/remove; group rules list/add/remove |
| Roles and permissions | permission catalogue, list/create/update/delete project roles |
| Columns, labels, milestones | list/create/update/reorder/delete each |
| Tasks | create, list, get, get by ticket id, update (versioned), move column, reorder, move project, duplicate, archive/restore/delete, bulk update, assignees add/remove, labels add/remove, watch/unwatch, subtasks, relations list/add/remove, activity |
| Comments | list, create, update, delete |
| Time | list, create manual, update, delete, start/stop/running timer, totals report |
| Attachments | init, complete, list, download link, delete |
| Planning | sprints create/list/get/update/start/complete/delete/report; backlog list; plan (bulk move) |
| Views | board, list (via task list), calendar, Gantt, my work, my-work counts, search, saved views CRUD, iCalendar token create/revoke |

All routes sit under `/api/v1/projects/...`; ids are UUID; envelope `{data, meta}` / `{error: {code, message}}` as everywhere. Mutations accept `Idempotency-Key`.
Tasks accept `If-Match: <version>` (or `version` in the body).

## 7. Events and inbox

Outbox rows written in the same transaction as the change, published by the shared relay (`kernel.OutboxRelay` pattern generalised to `shared/outbox`) to `jaas.projects.events`, envelope version 1, ids and versions only.
The inbox consumer (first one in the system) subscribes to identity and HR lifecycle events, records the event id in `inbox_events` in the same transaction as its effect, and ignores replays.

## 8. Performance and scale

Budgets (asserted in a performance test as in M1): single GET < 200 ms p95, lists/boards < 500 ms p95, DB statements < 50 ms p95 on the hot indexes.
Seed test: 10 projects × 50 000 tasks. Board loads columns with counts via one grouped query plus per-column cursor pages (no full-history loads); Gantt and calendar are windowed and capped; My work uses the assignee index.
Counts for badges are computed on read with covering indexes; caching (per tenant+user key, version-invalidated) only if the measurements require it.

## 9. Test console and docs

A `/dev-test/projects` page with generated operation forms from the OpenAPI and scenarios for each golden test, plus `contexts`-style docs (`testing.md`, `decisions.md`, `plan.md`) updated as steps complete; `docs/TEST_CONSOLE.md` and `SYSTEM_REFERENCE` regenerated at the end.

## 10. Risks and mitigations

| Risk | Mitigation |
|---|---|
| Rank rebalancing under concurrent moves | per-column advisory lock, bounded rebalance, golden test with parallel moves |
| Group-rule membership drifting from HR | refresh on org events plus a nightly reconcile; effective membership computed at read time with a short cache |
| Object storage dependency in dev/CI | MinIO container and an in-memory fake for unit tests |
| Scope growth from the Kaneo feature list | phases 1–3 frozen here; phase 4–5 only through the reserved extension points |
| Second service increases operational surface | per-service entry point and gateway arrive with the gateway phase; until then the all-in-one binary keeps dev simple |
