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

## Owner answers (2026-10-28)

1. **Scope:** phases 1-3 confirmed; later phases must not be blocked (extension points in specification section 9).
2. **UI:** one test console for every module (`/dev-test`), with an M2 page of its own that depends on the identity pages for sign-in. A product UI is a later phase.
3. **Storage:** approved, behind a provider interface (`shared/storage`). The AWS SDK v2 is added. **MinIO's official images are no longer published** (Docker Hub repository removed, Quay returns 401), so the dev service is `adobe/s3mock` on port 9090; the adapter speaks plain S3, so MinIO, AWS S3 or another compatible server is a configuration change. HR documents and other modules can adopt the same port later; HR is not changed now.
4. **Decided by the implementer (owner delegated 4-8):**
   - WIP limit: **soft by default**, with a per-column `wip_hard` switch (a hard column refuses the move with 409 `WIP_LIMIT`).
   - Subtask depth: **3**.
   - Time-entry lock window: **30 days**, and a project can **disable time tracking**.
   - Labels: **tenant-wide and project labels** both kept.
   - Public project links and recurring tasks: **phase 4+**.
5. **Branching:** `module-2` is pushed to `origin` and pushed again after each phase.
