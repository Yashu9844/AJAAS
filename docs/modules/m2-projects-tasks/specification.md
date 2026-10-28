# M2 Projects & Tasks — Specification (FROZEN 2026-10-28, phases 1-3 built)

> Status: **approved 2026-10-28 and built** (phases 1-3). This file is frozen: changes need a recorded decision. Deviations found
> while building are listed in the last section and in `decisions.md`. It replaces the earlier proposal (kept in git history). Feature scope is **phases 1–3**; phases 4–5 are
> designed for (section 9) so nothing later forces a schema or API break. Universal rules (tenancy, idempotency,
> outbox/inbox, envelopes, security, latency) in `docs/ARCHITECTURE.md` section 3 apply in full.
>
> Feature reference: the open-source Kaneo project (MIT), studied as a reference only. No code is copied. The matrix is in
> `reference-kaneo.md`.

**Purpose:** collaborative execution and delivery management for a tenant's people: plan work in projects, break it into
tasks, track it on boards, lists, calendars and Gantt charts, run sprints, log time, discuss and attach files, and know
what is mine and what is late.

**Principles**

1. **User-centric.** The first screen is "My work": everything assigned to me across projects, grouped by what is due.
   Every list is filterable, sortable and saveable as a personal or shared view. Notifications are driven by events and
   honour each user's preferences (delivery itself is M10).
2. **People are IDs.** Users and employees are referenced by stable ids. Names, avatars and org facts come through a
   people port; M2 never reads M0 or HR tables and never copies HR facts.
3. **Production behaviour from day one.** Idempotent creates, optimistic concurrency on edits, stable activity history,
   cursor pagination, tenant isolation, outbox events with versions.
4. **Extensible without breaking.** Custom columns are native from phase 1; custom fields, automation rules, webhooks and
   integrations plug into reserved extension points (section 9).

## 1. Projects (PR)

| ID | Requirement |
|---|---|
| PR-01 | A project has a `key` (2–10 uppercase letters/digits, unique per tenant), `name` (1–120), `description` (markdown, ≤ 10 000), `status` `active | archived`, `visibility` `private | tenant`, an owner, optional `start_on`/`due_on`, icon and color. It starts with default columns. |
| PR-02 | `private`: only members and tenant-level admins see it. `tenant`: any user of the tenant can read it and its tasks (read-only unless a member). |
| PR-03 | Archiving a project is reversible, hides it from default lists, blocks task creation and edits, and keeps all data. Deleting is a soft delete by tenant admins only, after archive. |
| PR-04 | Membership: `project_member` (user, role). A user can be added directly, or through a **group rule** (phase 3): all members of a department or team, or a manager's direct reports, resolved through the people port and refreshed on org changes. Removing the last owner is refused. |
| PR-05 | Roles: built-in `owner`, `manager`, `member`, `viewer` (fixed permission sets) plus tenant-defined **custom project roles** (a named set of project permissions). The project permissions are listed in section 8. |
| PR-06 | **Columns** (statuses) per project: name, key, position, color, `category` `backlog | todo | in_progress | done | canceled`, optional WIP limit, default flag. Customisable (add, rename, reorder, recolor, delete when empty or with a target column). At least one `done` and one `todo` column always exist. The category drives reports, "completed" and overdue logic. |
| PR-07 | **Labels**: project labels and tenant-wide labels (name unique per scope, color). |
| PR-08 | **Milestones**: name, `due_on`, status `open | reached`; tasks may belong to one milestone; progress = done tasks / tasks. |
| PR-09 | Project list for the caller: filters (status, member, key/name search), cursor pagination, `favorite` flag per user, sorted by recent activity by default. |

## 2. Tasks (TK)

| ID | Requirement |
|---|---|
| TK-01 | A task has a project-scoped sequence `number` (ticket id `KEY-12`, allocated atomically, never reused), `title` (1–300), `description` (markdown ≤ 50 000), `column`, `priority` `none | low | medium | high | urgent`, `reporter`, assignees (0..n), `start_on`, `due_at`, `estimate_minutes`, labels, milestone, optional `sprint`, optional `parent_task`, a `rank` for ordering, and a `version`. |
| TK-02 | Create is idempotent (`Idempotency-Key`): a replay returns the original task, never a duplicate. |
| TK-03 | Edit uses **optimistic concurrency**: every update carries the expected `version` (`If-Match` or body); a stale version returns 409 `VERSION_CONFLICT` with the current version; a successful update increments it. |
| TK-04 | Moving a task to another column sets `completed_at` when the column category is `done` and clears it when moved out; `canceled` is closed but not "done". WIP limit overflow is refused when the limit is hard, warned when soft. |
| TK-05 | Reorder within a column or between columns uses a fractional `rank` so one move touches one row; concurrent moves never lose a task. |
| TK-06 | Subtasks: `parent_task` nests up to 3 levels; a parent cannot be done while open subtasks exist unless forced by a manager; progress rolls up. A subtask stays in the parent's project. |
| TK-07 | Relations: `blocks` (directed), `related`, `duplicates`. `blocks` forbids cycles (checked inside the transaction over a bounded graph walk, 409 `DEPENDENCY_CYCLE`). A task with open blockers shows as blocked and cannot move to `done` unless forced. Cross-project relations are allowed between projects the caller can read. |
| TK-08 | Move to another project: the task gets a new number in the target project and keeps its history, comments and attachments; the old ticket id keeps resolving (redirect). |
| TK-09 | Bulk update (up to 100 tasks): column, priority, assignees add/remove, labels, milestone, sprint, archive. Each task is checked; the response lists per-task success or error. |
| TK-10 | Archive and restore tasks (separate from delete). Soft delete by manager+; hard purge only by retention job after 30 days. |
| TK-11 | Duplicate a task (title suffix, same labels/assignees optionally, no comments). |
| TK-12 | Lookup by ticket id (`KEY-12`) resolves within the tenant. |
| TK-13 | Task list for a project: filters (column, category, priority, assignee, label, milestone, sprint, due range, overdue, blocked, text), sorts (rank, created, updated, due, priority, number), **cursor pagination** (max 100). |
| TK-14 | Search: tenant-scoped full text over title and description plus ticket-id lookup; only projects the caller can read are searched. |

## 3. Collaboration (CM)

| ID | Requirement |
|---|---|
| CM-01 | Comments: markdown ≤ 10 000, author edits and deletes their own (deleted comments leave a tombstone), managers can delete any. Edited comments show `edited_at`. |
| CM-02 | Mentions: `@[Name](user:<uuid>)` tokens in descriptions and comments. The server extracts ids, drops those who cannot read the project, stores them, and emits `task.mentioned` with ids only. |
| CM-03 | Watchers: a user watches a task (explicitly, or automatically as reporter, assignee, commenter or mentioned). Watchers receive assignment/status/comment events (delivery by M10). |
| CM-04 | **Activity**: an append-only history per task and per project (created, field changes with old/new values, moved, assigned, commented, attached, related, time logged). Entries are never edited or deleted; ordered by a monotonic sequence; cursor paged. |
| CM-05 | Activity values for restricted fields are still present but personal data is only ids. |

## 4. Time tracking (TM)

| ID | Requirement |
|---|---|
| TM-01 | Log time on a task: manual (`started_at`, minutes, note) or a running timer (start/stop). One running timer per user (database constraint). Entries are edited or deleted by their author within the project's lock window (default 30 days) and by managers. |
| TM-02 | Totals per task, per user and per project for a date range; estimate vs logged. |
| TM-03 | A timer left running more than 24 h is flagged and stopped by a sweep, marked `auto_stopped`. |
| TM-04 | Time entries never include HR facts; payroll/billing use is out of scope. |

## 5. Attachments (AT)

| ID | Requirement |
|---|---|
| AT-01 | Files are stored in **S3-compatible object storage** through the shared storage port (section 6 of `design.md`), not as URLs. Flow: `init` (filename, size, content type) → client uploads straight to storage with a presigned URL → `complete` (server verifies existence, size, content type and checksum, then marks the attachment ready). |
| AT-02 | Limits: 50 MB per file, per-tenant quota, allow-list of content types, filename sanitised, executables refused. A malware-scan hook runs before `ready` (no-op implementation until a scanner exists). |
| AT-03 | Download: authorised against the task's project, returns a short-lived presigned URL; access is logged to activity for sensitive projects. Attachments inherit task/comment permissions (`attachment.upload`, `attachment.delete`). |
| AT-04 | Pending uploads older than 24 h and deleted attachments are cleaned up by a sweep (object and row). Deleting a task keeps attachments until purge. |
| AT-05 | Documents (M8) can later own attachment metadata; the storage keys and the port stay unchanged. |

## 6. Planning (SP, VW)

| ID | Requirement |
|---|---|
| SP-01 | **Sprints** (project mode `scrum`): name, goal, `start_on`, `end_on`, status `planned | active | completed`. At most one active sprint per project (database constraint). |
| SP-02 | Start: requires planned status and dates; completing: unfinished tasks move to the next planned sprint or the backlog (choice in the request); a completion report stores committed vs completed counts and estimates. |
| SP-03 | **Backlog**: in `kanban` mode, tasks in a `backlog`-category column; in `scrum` mode, tasks with no sprint. Planning view moves tasks between backlog and sprints in bulk. |
| VW-01 | **Board** view: columns with counts, cursor-paged tasks per column (no full-history loads). |
| VW-02 | **List** view: any filter/sort combination, grouping (column, assignee, priority, milestone, sprint), cursor pagination. |
| VW-03 | **Calendar** view: tasks by `due_at` / `start_on` for a date window (max 62 days). An iCalendar feed per user (tokenised, revocable) lists assigned tasks. |
| VW-04 | **Gantt** view: tasks with `start_on`/`due_at`, milestones and `blocks` relations for a window; computed server-side, bounded to 500 tasks per request with paging. |
| VW-05 | **My work**: tasks assigned to me across projects grouped overdue / today / this week / later / no date, plus watching and mentions; counts endpoint for badges. |
| VW-06 | **Saved views**: filters, sort, grouping and layout saved per user, or shared with a project (manager+). |

## 7. Events (EV)

Envelope version 1 (shared standard), exchange `jaas.projects.events`; payloads carry ids and versions only.

`project.created|updated|archived|restored`, `project.member.added|changed|removed`, `task.created|updated|moved|assigned|unassigned|completed|reopened|archived|commented|mentioned|due_changed`, `task.relation.added|removed`, `sprint.started|completed`, `attachment.added|removed`, `time.logged`.

**Consumed** (inbox, deduplicated by event id): identity user deactivated/removed → membership and assignments are marked inactive (history kept); HR termination → same.

## 8. Permissions and security (SC)

- **Tenant level (M0 catalogue):** `projects:project:create`, `projects:project:read_all` (see every project, including private), `projects:project:manage_all` (admin override), `projects:role:manage`.
- **Project level (role permissions):** `project.read`, `project.update`, `project.archive`, `project.manage_members`, `column.manage`, `label.manage`, `milestone.manage`, `task.create`, `task.read`, `task.update`, `task.delete`, `task.assign`, `task.comment`, `comment.moderate`, `time.log`, `time.manage`, `attachment.upload`, `attachment.delete`, `sprint.manage`, `view.share`. Built-in roles: viewer (reads), member (+create/update/assign/comment/time/upload), manager (+ manage columns, labels, milestones, sprints, moderate, share views, members), owner (all, including archive).
- Access rule: allowed if the caller's tenant permission covers it, or the caller is an effective member whose role holds the permission, or the project is `tenant`-visible and the action is read.
- Object-level: a task, comment, attachment or time entry is always checked through its project; unknown or foreign ids are 404.
- No secrets, no free text and no personal data beyond ids in events and audit metadata.

## 9. Extension points for phases 4–5 (designed now, built later)

| Later feature | How it plugs in without breaking phases 1–3 |
|---|---|
| Custom fields | **BUILT (2026-10-29)**: tables `custom_field_definitions` / `custom_field_values` (typed, project-scoped); values are read and written through `GET/PUT /tasks/{id}/custom-fields` (not inside the task object, so the task contract is unchanged); activity `data` is already JSON. |
| Workflow rules / automation | `workflow_rule(project, trigger, conditions, action)` evaluated by a consumer of the project's own outbox events; columns already carry `category` for rules like "when PR merged, move to done". |
| Webhooks | `webhook` + `webhook_delivery` tables; delivery from the outbox with signing and retries. |
| GitHub / Gitea / chat | `external_link(task, provider, url, state)` plus an `integration` table; incoming webhooks create activity entries and trigger rules; `activity.source` already exists. |
| Public project views | `project.visibility` gains `public` with a read-only token route. |
| MCP / CLI / AI | Generated from the same OpenAPI contract; no extra server-side work. |
| Recurrence | `task_recurrence(task, rule, next_run)` and a scheduler; tasks already have `start_on`/`due_at`. |

## 10. Golden tests (frozen when approved)

`PG1` duplicate task creation replay returns the original; `PG2` concurrent status updates: exactly one wins per version, none lost; `PG3` dependency cycles rejected, including through concurrent inserts; `PG4` non-members denied, private projects invisible, foreign tenant ids 404; `PG5` task numbers are unique and gapless under concurrency; `PG6` one active sprint and one running timer enforced under races; `PG7` large board pagination: a 50 000-task project pages within budget with stable order; `PG8` attachment ACL: no download without project access, presigned URLs expire, wrong size/type refused; `PG9` tenant-isolated search; `PG10` event replay is idempotent for consumers and the outbox never loses a committed event; `PG11` mentions never leak a task to someone who cannot read it; `PG12` events and audit hold ids only.

## 11. Deviations recorded while building (decision references in `decisions.md`)

- Routes sit at the `/api/v1` root (`/projects`, `/tasks/{id}`, `/sprints/{id}`, ...), not under a module prefix (DPJ-15).
- `PATCH /tasks/{id}` requires `version` (body) or `If-Match`; a same-column reorder writes only the rank and does not bump the version.
- Dependency edits are serialised with one advisory lock per tenant (cycles can cross projects); the spec's per-project lock was not enough.
- A task with a parent or subtasks cannot be moved to another project or re-parented (409 `TASK_HAS_HIERARCHY`).
- Events added: `project.deleted`; deletion reuses `task.archived` with `deleted: true`, restore reuses `task.updated` with `restored: true`.
- AT-02 refusals: oversize and type refusals are 400 `FILE_TOO_LARGE` / `FILE_TYPE_NOT_ALLOWED`, quota is 409 `QUOTA_EXCEEDED`.
- CM-02: a mention counts as readable only for active tenant users who are members of the project (or the project is tenant-visible); users who
  see a private project only through `projects:read_all` are not notified.
- PR-04 group rules are materialised as `project_members.source = 'group'` rows and refreshed (DPJ-22); an explicit role change pins a member as direct.
- SP-03: in scrum mode the backlog excludes done and canceled tasks that have no sprint; sprints, backlog and plan answer 409 `PROJECT_MODE` for kanban projects.
- VW-03: the iCalendar feed is the only M2 route without a bearer token (token hash, owner still active, readable projects only).
- TK-14 search is implemented in the planning context (`GET /search`); the task list also has a `q` filter.
- Deactivated logins keep their rows: `project_members.active` and `task_assignees.active` become false; history stays (section 7).
- Custom fields (section 9, built): types text, number, checkbox, date, url (https only, never fetched), select, multiselect, user (active project member);
  at most 30 per project; the type is fixed at creation; defining needs `project.update`, setting values needs `task.update` and bumps the task version
  (optional `version` / `If-Match`); free text and URLs are recorded in activity by field only; removing a select option clears the values that used it.
