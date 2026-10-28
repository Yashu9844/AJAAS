# M2 Projects & Tasks — decisions

Proposed decisions (DPJ) take effect when the design is approved. Open questions (Q) need an answer before the step that depends on them.

## Proposed decisions

- **DPJ-01 Own service, own stack.** M2 is written in Go in the monorepo, with its own Postgres schema `projects`, migrations, routes, outbox/inbox and console page. Kaneo is a feature reference only: no code or schema is copied or run, so there is no second runtime and no licence obligation beyond crediting the reference.
- **DPJ-02 Tenant = workspace.** Kaneo's "workspace" maps to our tenant (M0). Invitations, users and tenant roles stay in M0; M2 adds project membership and project roles on top.
- **DPJ-03 Columns are the statuses.** A task's status is its column; each column has a `category` (`backlog|todo|in_progress|done|canceled`) so reports and rules work across projects with different column names.
- **DPJ-04 Optimistic concurrency from the start.** Tasks carry a `version`; edits send the expected version. (Kaneo has none; the original M2 proposal required it.)
- **DPJ-05 Sprints are first-class** (Kaneo has none). A project is `kanban` or `scrum`; scrum projects plan work into sprints, kanban projects use a backlog column.
- **DPJ-06 Multiple assignees**, as in the original proposal (Kaneo allows one).
- **DPJ-07 Real file storage now.** Attachments use a shared S3-compatible storage port with presigned upload and download. HR documents (which today only store a URL) and M8 can adopt it later.
- **DPJ-08 Markdown, not a rich-text document model.** Descriptions and comments are stored as markdown text; mentions are `@[Name](user:<uuid>)` tokens extracted by the server. A rich editor on the client can emit and render markdown.
- **DPJ-09 Fractional ranks** for ordering so a move touches one row.
- **DPJ-10 Ticket ids** `KEY-N` with a per-project counter; moving a task to another project keeps the old id resolving through `task_ticket_alias`.
- **DPJ-11 HR-aware membership.** Add a department, a team or a manager's direct reports to a project through group rules resolved with the People port. This is the main product difference from a generic tracker.
- **DPJ-12 Extension points over speculative tables.** Phase 4–5 tables are reserved by name and their plug-in points are documented in the specification (section 9); no empty tables are created now.
- **DPJ-13 Shared identity port.** Move `identityport` to `internal/shared/identityport`; `shared/identityport` becomes a thin alias package so no HR file changes.
- **DPJ-14 Notifications.** M2 only publishes events. Until M10 exists nothing is delivered; per-user preferences belong to M10. In-app "unread" for My work uses read markers owned by M2 only if M10 slips (revisit at phase 3).

## Open questions (please answer)

1. **Scope:** phases 1–3 as written in the specification (projects, tasks, comments, time, attachments, sprints, board/list/calendar/Gantt/my-work, search, saved views). Confirm, or move items between phases.
2. **Product UI:** besides the test console, do you want a real Kanban/list UI inside this module (a new `frontend/src/modules/projects` app, using Kaneo as UX reference)? Proposed: console first, product UI as a separate phase after phase 2.
3. **MinIO in the dev compose** and a new Go dependency (AWS SDK v2) for storage: approved?
4. **Hard WIP limits** (refuse) or soft (warn) as the default?
5. **Subtask depth** limit 3: acceptable?
6. **Time-entry lock window** 30 days: acceptable? Should projects be able to disable time tracking?
7. **Tenant-wide labels** in addition to project labels: keep?
8. **Public project links** and recurring tasks stay in phase 4+ as listed: confirm.
