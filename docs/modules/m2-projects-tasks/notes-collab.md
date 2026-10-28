# M2 collaboration context — implementation notes

Scope: specification sections 3 (CM-01..CM-03), 4 (TM-01..TM-04), 5 (AT-01..AT-04) and golden tests PG4, PG6, PG8, PG11, PG12.
Package `backend/internal/projects/collab`. It depends on the task context only through `api.TaskResolver` and on project
access only through `api.Authorizer`; it imports neither `task` nor `project` (tests import `project` for the real Authorizer).
Activity (CM-04) is written with `kernel.LogActivity`; reading it belongs to the task context.

## What was built

- Migrations 000020..000024: `comments`, `mentions`, `task_watchers`, `time_entries` (partial unique index
  `uq_time_entries_running (tenant_id, user_id) WHERE ended_at IS NULL`), `attachments`. Up/down/up verified in a throwaway database.
- Comments: create, cursor-paged list, edit, delete. Tombstone = row kept, body cleared in the database, `deleted_at` set.
- Mentions: exported `ParseMentions` (max 20 distinct ids) and `Service.ReadableMentions`; exported `ReplaceMentions` and
  `AddWatcher` for the task context to reuse for descriptions, reporters and assignees.
- Watchers: watch, unwatch, status, list; automatic for commenters and mentioned users.
- Time: manual log, list, edit, delete, start/stop/running timer, report per task or user, `StopStaleTimers`, `Run` loop.
- Attachments: init, complete, list, download, delete, `SweepAttachments`, malware-scan hook (`Scanner`, default `NoopScanner`).
- Wiring in `module.go` at the `wire:collab` / `routes:collab` markers (service, background `Run` every 5 minutes, routes). The task
  service is already wired and satisfies `api.TaskResolver`, so it is used directly; no interim resolver was kept.

## Routes (21)

`GET|POST /tasks/:id/comments`, `PATCH|DELETE /comments/:id`, `GET|PUT|DELETE /tasks/:id/watch`, `GET /tasks/:id/watchers`,
`GET|POST /tasks/:id/time`, `PATCH|DELETE /time-entries/:id`, `POST /time-entries/timer/start`, `POST /time-entries/timer/stop`,
`GET /time-entries/timer`, `GET /projects/:id/time/report`, `POST|GET /tasks/:id/attachments`, `POST /attachments/:id/complete`,
`GET /attachments/:id/download`, `DELETE /attachments/:id`. Timer paths use POST/GET only, so they never meet the PATCH/DELETE
`:id` routes in one gin tree. Swagger source: `scripts/pm_routes_collab.py`; console spec context `PM_COLLAB`.

## Decisions (decision - reason - alternatives)

- **Mention readability uses an actor without tenant flags.** A mentioned user can read if active in the tenant and
  `Authorizer.Access` grants `task.read` for a bare actor: tenant-visible projects (everyone) or active members of private ones.
  Users who see a private project only through `projects:read_all` are dropped. Reason: fail closed (PG11), flags of other users are
  unknowable here. Alternative: query the identity module for each user's flags - cross-module, rejected.
- **Authors are dropped from their own mentions**; events list only newly added users on edits.
- **Mentions table has `task_id`** (not in the design) so My work can query mentions without joining comments (planning does).
- **No `project_id` on comments or time entries**; they follow the task, so a project move needs no rewrite. Reports join `tasks`.
- **Attachments keep `project_id` but no FK on `task_id`.** The key layout is fixed at init; the retention purge deletes tasks and the
  sweep then retires orphaned rows and deletes their objects. A cascading FK would orphan objects.
- **Comment-level attachments (`comment_id`) not built**; the column can be added later without breaking anything.
- **Lock window** is measured from the entry's `started_at` (30 days, constant). Authors cannot create or move entries into the locked
  past; `time.manage` holders can. A running timer cannot be edited (409 TIMER_RUNNING) but can be discarded by deleting it.
- **Stop timer needs no project permission**: it only touches the caller's own row, and a user who lost access must be able to end it.
  Timer minutes are rounded, at least 1, at most 1440. The sweep caps an auto-stopped entry at 24 h after its start.
- **Tracking disabled** blocks logging and starting (409 TIME_TRACKING_DISABLED); edit, delete, stop and reports still work.
- **Reports** are open to everyone who can read tasks (per-user rows included); the estimate is the sum over all live tasks of the
  project, not limited to the date range. Rows capped at 500, totals computed separately so they stay exact.
- **Content types**: allow-list of images (no SVG), PDF, text, CSV, Markdown, JSON, ZIP, Office/OpenDocument, mp4, mp3; parameters are
  stripped. Executable extensions are refused anywhere after the first dot (defeats `invoice.pdf.bat`). Filename: last path segment,
  control and format characters (bidi overrides) removed, 255 characters max.
- **Error codes stay within 400/403/404/409 in the swagger** (the generator knows no other); size, type and filename failures are 400
  with codes `FILE_TOO_LARGE` / `FILE_TYPE_NOT_ALLOWED`; quota and per-task limits are 409 `QUOTA_EXCEEDED` / `TOO_MANY_ATTACHMENTS`.
  503 `STORAGE_UNAVAILABLE` is documented in the route summaries and the module docstring only.
- **Pending uploads are invisible** to everyone except the uploader (404); complete is uploader-only (403 for others once ready).
- **Download of private projects is logged** as activity `attachment_downloaded`; tenant-visible projects are not (AT-03 "sensitive").
- **Sweep ordering**: stale pending rows are first marked `deleted` in one UPDATE (so a late complete fails), then each deleted row's
  object is deleted, then the row. A failed object delete keeps the row for the next run.
- Delete of an attachment removes the row immediately when the object delete succeeds; otherwise the sweep finishes it.

## Deviations from the specification and design

- Table names are plural (`mentions`, `task_watchers`, `time_entries`, `attachments`) to match the existing schema.
- `comment_id` on attachments omitted (above). AT-05 (documents owning metadata) unaffected.
- No checksum is computed server-side: the sha256 is verified only when the client sent one at init (the storage adapter binds it).

## Tests

- `internal/projects/collab`: 37 test functions (real PostgreSQL via `testdb`, `storage.Memory`, plus one real-S3 round trip against
  `http://localhost:9090`, required with `JAAS_REQUIRE_INTEGRATION=1`). Statement coverage 92.1%. Covers validation tables, every
  403/404/409 path, PG11 mention filtering, PG6 race (12 goroutines, exactly one timer), lock window, quota/type/size/executable
  refusals, upload mismatch (missing, size, type, checksum), scanner outage and infection, download ACL (PG8), sweeps with a failing
  provider, tombstones, events/audit/activity holding ids only (PG12), the HTTP routes and the 503 path.
- `tests/integration/projects_collab_test.go`: 3 tests through the real router (`TestCollabComments`, `TestCollabTime`,
  `TestCollabAttachmentsWithoutStorage`); the integration app has no storage, so attachment endpoints answer 503 there.
- Console scenarios `frontend/src/modules/devtest/scenariosPmCollab.ts` (`S_PM_COLLAB`: comments, time, attachments). The browser
  upload to the presigned URL is not scripted; the scenario stops after init/mismatch checks (or skips on 503).

## Known gaps

- Watchers are not removed when a user loses project access; delivery filtering belongs to M10.
- `TestSwaggerMatchesRoutes` currently fails only because the task context's `scripts/pm_routes_tasks.py` does not exist yet
  (`PM_TASKS` has 0 operations); no collaboration route is undocumented.
- The Run loop and sweeps are tested directly, not through `module.New`.
- Descriptions of tasks do not yet store mentions: the task context must call `ParseMentions`, `ReadableMentions` and `ReplaceMentions`
  (source kind `task`) and `AddWatcher`.
- Rate limiting of attachment init/download is not added in this context.
