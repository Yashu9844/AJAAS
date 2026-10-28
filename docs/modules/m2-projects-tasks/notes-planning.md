# M2 Planning and views context: build notes

Package `backend/internal/projects/planning`. Covers SP-01..SP-03, VW-01..VW-06 and TK-14 (search) of the specification, golden tests PG6, PG7, PG9 and PG12.

## What was built

| Area | Behaviour |
|---|---|
| Sprints (SP-01/02) | Table `sprints`; planned -> active -> completed. Start needs dates and a planned sprint; the partial unique index `uq_sprints_one_active` allows one active sprint per project (409 `SPRINT_ACTIVE`, PG6). Complete takes `move_unfinished_to` (`backlog` or a planned sprint of the same project); unfinished = column category not `done`/`canceled`. The stored report is `{committed_count, completed_count, committed_estimate_minutes, completed_estimate_minutes}` plus `moved_unfinished_count` and `moved_to`. Delete only while planned; its tasks return to the backlog. Edits are versioned (`version` or `If-Match`). Kanban projects answer 409 `PROJECT_MODE`. |
| Backlog and plan (SP-03) | `GET /projects/:id/backlog`: kanban = tasks in a `backlog`-category column, scrum = open tasks without a sprint. `POST /projects/:id/plan` moves up to 100 tasks to a sprint or the backlog, with an optional `versions` map and a per-task result (`moved`, `unchanged`, `conflict`, `not_found`, `archived`). |
| Views (VW-01..05) | Board with counts and a keyset page per column (`cursor_<columnId>`), project calendar and my calendar (window <= 62 days, UTC, 500 task cap with `truncated`), Gantt (dated tasks, milestones, `blocks` edges, keyset paged, <= 500), My work (overdue/today/this_week/later/no_date plus watching and mentions, counts endpoint), search (`websearch_to_tsquery('simple')`, ticket id lookup, ranked by `ts_rank`, <= 50). |
| Saved views (VW-06) | Table `saved_views`; layout, filters, sort and grouping validated against closed allow-lists, decoded with unknown fields refused and stored re-encoded. Shared views need `view.share` in their project; personal views are visible to the owner only. |
| iCalendar (VW-03) | Table `calendar_tokens` (SHA-256 hash only, raw 32-byte base64url token shown once, max 10 active per user). `GET /calendar/feed.ics?token=` is the only M2 route without a bearer token. |

Routes: 25 (see `scripts/pm_routes_planning.py`). Migrations 000030 sprints + FK, 000031 saved_views, 000032 calendar_tokens, 000033 view indexes (up/down/up verified in a throwaway database).

## Decisions

| Decision | Reason | Alternatives / trade-off |
|---|---|---|
| Planning does not import the task context; read models query `tasks`, `board_columns`, `task_assignees`, `task_labels`, `task_relations`, `task_watchers`, `mentions` by SQL; access goes through `api.Authorizer`. | Keeps the context boundary (the task package is built in parallel) and gives one-query pages. | Calling the task service would add N+1 calls. Cost: planning knows those column names. |
| The only write to a task row is `tasks.sprint_id` (+ `version`+1, `updated_at`). | The task context owns every other column and rule (WIP, ranks, completion). One column cannot clobber a concurrent edit of another field; the per-task version check under `FOR UPDATE` still gives conflicts. | Routing through the task service's bulk update (TK-09) was rejected to stay independent. |
| Cross-project queries pass the readable project ids as one `uuid[]` literal (`= ANY(?::uuid[])`). | One bind parameter however many projects; ids are UUID text so the literal cannot carry SQL. | `IN (?,?,...)` hits the 65 535 parameter limit. |
| `taskProjects` drops projects where a custom role holds `project.read` without `task.read`. | `ReadableProjectIDs` only knows project readability. The API refuses such a role, but old data could hold one. | Calling `Access` for every project (too slow). Only custom-role memberships are re-checked. |
| Feed tenant and user come from the token row; the owner is re-checked as active through the identity port; unknown, revoked, malformed and inactive-owner tokens all answer the same 404. | No oracle, no client-supplied tenant. | A JWT-less route cannot know tenant-level flags, so the feed shows only projects the user is a member of or that are tenant-visible (fails closed for admin-only visibility). |
| Feed rate limit is in process (30 per minute per IP) with `Service.SetFeedLimiter` to plug the shared Redis limiter. | `projects.Options` has no Redis handle and the module file may only be edited at markers. | Wire `middleware.RateLimiter` in `module.go` (lead). |
| Backlog in scrum mode excludes done/canceled tasks. | Nothing left to plan. | Literal "no sprint" would list finished work. |
| Overdue = due before today 00:00 UTC; "today" covers the whole UTC day; the week runs Monday to Sunday. | Stable buckets for badges. | Per-user time zones need a profile setting. |
| Completion computes the report at completion time (not a snapshot at start). | Matches SP-02 wording; scope added mid-sprint counts as committed. | Snapshot at start would need another column. |
| `sprint_id` values that existed before the FK are cleared in migration 000030. | No sprint table existed, so any value dangled. | None. |

## Deviations from specification and design

* Table names are plural (`sprints`, `saved_views`, `calendar_tokens`) like the rest of the schema; `calendar_tokens` has `id`, `label`, `last_used_at`; `saved_views` has `version`.
* Sprint names are unique per project (extra rule, 409 `SPRINT_NAME_TAKEN`), saved view names unique per owner and scope (409 `VIEW_NAME_TAKEN`).
* The list view (VW-02) is the task context's list endpoint, not repeated here.
* Calendar and Gantt are not cursor paged beyond the stated bounds: calendar is capped (500, `truncated`), Gantt is keyset paged.
* Events: `sprint.started`, `sprint.completed` and, for planning moves, one `task.updated` per moved task (ids, version, sprint id).

## Test inventory

Unit and service tests with real Postgres (`internal/projects/testdb`), package `planning`: 45 test functions in 12 files (sprint CRUD, state machine, PG6 race with 8 goroutines, completion report numbers, plan moves and concurrent same-version conflict, backlog by mode, board paging and stability, calendar window and truncation, Gantt with dependencies and hidden unreadable blockers, My work grouping/paging/watching/mentions, search ranking and PG9 isolation, custom-role task.read, saved view validation and visibility, tokens, ICS escaping and folding, feed content, limiter, HTTP routes). Statement coverage 92.7 percent (`go test -cover`, target 88).

Integration (`tests/integration/projects_planning_test.go`): `TestPlanningSprintsAndPlan`, `TestPlanningViewsSearchAndFeed` through the real router, plus `TestSwaggerMatchesRoutes`, `TestSchemasAreOwnedAndIsolated`, `TestMigrationsAreIdempotentAndComplete` (all pass with `JAAS_TEST_DB_NAME=jaas_itest_plan JAAS_TEST_REDIS_DB=13`).

Console: `frontend/src/modules/devtest/scenariosPmPlanning.ts` exports `S_PM_PLANNING` (two scenarios). The lead wires it into the pages.

## Performance (PG7), `JAAS_PERF=1`, 50 000 tasks in one project

Seed by `generate_series` in 13.6 s. Full walk of the 30 000-task todo column in 300 pages of 100: strictly increasing (rank, id), no gaps or repeats. p95 over 30 runs (budget 500 ms):

| Operation | p95 |
|---|---|
| Board walk page (300 pages) | 111 ms (max 172 ms) |
| Board first page, 50 per column | 113 ms |
| Board deep page (offset 15 000) | 122 ms |
| Backlog page (100) | 19 ms |
| Search `needle` (500 hits) | 78 ms |
| Search ticket id | 28 ms |
| Gantt page (500) | 68 ms |
| My work (50 per group) | 226 ms |
| My-work counts | 43 ms |

## Known gaps

* The feed limiter is per process; production should use the shared Redis limiter.
* No per-user time zone for My work and the feed (UTC).
* Group-based "watching" notifications are M10; only counts and lists are provided.
* Gantt dependency edges are capped at 2 000 per page.
* Completing a sprint logs one project-level activity entry, not one per moved task.
