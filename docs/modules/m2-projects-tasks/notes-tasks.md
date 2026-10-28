# M2 task core — implementation notes

Scope: package `backend/internal/projects/task` (TK-01..TK-14 without the search endpoint, CM-04 reads). Status: built, unit and
integration tests green; not committed (the lead commits).

## What was built

| Area | Where |
|---|---|
| Model, views, inputs | `models.go` (Task row, View with ticket key, column category, blocked/overdue, subtask roll-up, assignees with active flag) |
| Mutation core | `mutate.go`: one transaction per mutation, row `FOR UPDATE`, optimistic version check, one UPDATE, activity + outbox + audit written at the end (`note`) |
| Create, ticket numbers | `create.go` (`UPDATE projects SET task_counter = task_counter + 1 ... RETURNING` in the create transaction) |
| Update | `update.go` (field diff, `Opt[T]` absent/null/value for nullable fields, parent change for tasks without subtasks) |
| Move, reorder, WIP, done rules | `move.go`, `rank.go` |
| Move to another project | `moveproject.go` (+ `task_ticket_alias`) |
| List, filters, keyset | `list.go`, `list_where.go` |
| Archive, restore, delete, duplicate | `lifecycle.go`, `duplicate.go` |
| Bulk | `bulk.go` |
| Assignees, labels | `assign.go` |
| Relations, cycle check | `relations.go` |
| Ticket lookup, subtasks | `ticket.go` |
| Activity reads | `activity.go` |
| `api.TaskResolver` | `load.go` (`Service.Task`) |
| HTTP | `http.go`, `http_links.go` (`Service.Register`) |
| Migrations | `000010_create_task_relations`, `000011_create_task_ticket_alias`, `000012_tasks_list_sort_indexes` (up and down; up/down/up verified in a throwaway database) |
| Swagger | `scripts/pm_routes_tasks.py` (23 operations, tag `Tasks`) |
| Console | `frontend/src/modules/devtest/scenariosPmTasks.ts` exports `S_PM_TASKS` (4 scenarios); the lead wires the page |
| Wiring | `module.go` markers only: `m.task = task.NewService(o.DB, m.project, task.Ports{Audit: ...})` and `m.task.Register(g)` |

## Routes (23)

`POST|GET /projects/:id/tasks`, `POST /projects/:id/tasks/bulk`, `GET /projects/:id/activity`, `GET|PATCH|DELETE /tasks/:id`,
`GET /tickets/:key`, `POST /tasks/:id/{move,reorder,move-project,duplicate,archive,restore}`, `POST /tasks/:id/assignees`,
`DELETE /tasks/:id/assignees/:user_id`, `POST /tasks/:id/labels`, `DELETE /tasks/:id/labels/:label_id`, `GET /tasks/:id/subtasks`,
`GET|POST /tasks/:id/relations`, `DELETE /tasks/:id/relations/:relation_id`, `GET /tasks/:id/activity`.

## Decisions (reason, alternative)

- **Version required on PATCH.** TK-03 says every update carries the version; PATCH answers 400 without it (body `version` or
  `If-Match`). Move, reorder and move-project take the version optionally and always lock the row. Alternative: optional like the
  project PATCH; rejected because PG2 only holds when the version is sent.
- **Same-column reorder does not bump `version`** and writes one row (`rank` only); a column change bumps it. Reason: dragging
  must not conflict with editors. Event `task.moved` is still emitted.
- **Lock order: column advisory lock, then task row.** The target column is locked first (`pg_advisory_xact_lock`) so rank
  allocation, WIP counting and rebalance of one column are serialised and cannot deadlock with row locks. This also makes the
  hard WIP limit exact under concurrency (tested: 9 parallel moves into a limit of 3 admit exactly 3).
- **Rank rebalance.** If neighbours leave no room, share a rank (ties are detected), or a rank exceeds 24 characters, the column
  is rewritten once (bounded to 5000 tasks, else 409 `COLUMN_TOO_LARGE`). I did not use `kernel.Spread`: it grows a character
  every few items on long columns; `spreadRanks` writes fixed-width evenly spaced ranks instead. (Lead: `kernel.Spread` is fine
  for tiny columns only.)
- **Dependency cycles use one advisory lock per tenant, not per project.** A cycle can run through several projects, and a
  per-project lock lets two inserts in disjoint project pairs both pass the walk. Walk: breadth first, 10 000 nodes, then 409
  `DEPENDENCY_LIMIT`. `related` is stored once in canonical id order so A-B and B-A collide.
- **Free text never leaves the tasks table.** Events carry ids, versions and enum values; activity keeps old/new for enums, ids
  and dates and only the field name for title/description; audit metadata is project id and version. Tested (PG12).
- **Force = `task.delete` holder** (manager proxy) for blocked tasks and open subtasks, as the brief said; needed only when the
  rule would otherwise refuse.
- **Assignees.** Must be active project members whose role can read tasks (built-in role or custom role holding `task.read`).
  View: `assignee_ids` lists active assignees, `assignees` lists every assignment with `active`; inactive ones (deactivated login,
  migration 000040) are history only: not counted in the 20-assignee cap, not matched by the assignee filter, not copied by
  duplicate. Assigning an inactive user again reactivates the row. Explicit unassign deletes the row.
- **Soft delete** cascades to the subtree in one recursive statement and emits `task.archived` with `deleted: true` (the event
  list has no deleted event); restoring a task emits `task.updated` with `restored: true`.
- **Move to another project** is refused for tasks with a parent or subtasks (409 `TASK_HAS_HIERARCHY`). Column choice: given
  column, else same key, else first of the same category, else the default column. Milestone and project labels map by
  case-insensitive name or are dropped; tenant labels stay; the sprint is cleared; assignees who cannot read the target are
  removed; the WIP and done rules of the target apply. Activity gets `moved_out` (old project) and `moved_in` (new project).
- **Warnings.** Soft WIP overflow returns `warnings: ["WIP_LIMIT_EXCEEDED"]` in the response view (move, create, duplicate).
- **List ordering.** Sorts are an allow-list; every sort is `(key, id)` with a keyset cursor bound to the sort and direction.
  Priority uses a stored generated column `priority_rank` (migration 000012); `due` sorts undated tasks last through
  `COALESCE(due_at, 'infinity')`. Defaults: rank asc, created/updated/priority/number desc, due asc.
- **Sprint ids** are validated against the `sprints` table of the planning context (same schema) by a plain query, no package
  import.

## Deviations from specification / design

- TK-14 (search) is not in this package; the `search` tsvector and trigger from migration 000007 are used by the `q` list filter
  (`plainto_tsquery` OR title ILIKE) only.
- Advisory lock per tenant instead of per project for dependency edits (above).
- Parent change on update only for tasks that have no subtasks; moving a hierarchy to another project is refused. Both avoid
  subtree re-depth logic; known gap below.
- Some internal helpers take 5 parameters including `ctx` (`mutate`, `bulkOne`, `neighbour`, `pickColumn`); the 7- and 6-parameter
  ones were reduced with the `unit` bundle. No function is over 50 lines; no file is over 300 lines.
- swagger path parameter `{key}` of `/tickets/{key}` is documented with the generator's uuid format (the generator types every
  path parameter as uuid). Cosmetic; the server accepts `KEY-12`.

## Tests

- Unit/service tests with real Postgres (`internal/projects/testdb`): **48 test functions** plus about 35 sub-tests in
  `faults_test.go` (each breaks one table and checks the operation fails cleanly). Package statement coverage **94.6%**.
  Files: create, update, move, list, lifecycle (assignees, labels, archive, delete, duplicate), relations, moveproject, bulk,
  activity (ticket lookup, tenant isolation), inactive, http (2 router-level tests), faults.
- Golden coverage: PG1 (HTTP integration, Idempotency-Key replay), PG2 (`TestConcurrentUpdates`: 12 writers, one winner),
  PG3 (`TestDependencyCycles`, `TestConcurrentCycleInserts`: 30 concurrent edges, acyclic result), PG4 (`TestTenantAndProjectIsolation`
  plus integration), PG5 (`TestTicketNumbersGapless`: 40 concurrent creates; integration with 12), PG7 in miniature
  (`TestKeysetPaginationAllSorts`: all sorts and directions page without gaps or repeats; the 50 000-task seed is the performance
  step P7, not done here), PG12 (`TestEventsAndAuditCarryIDsOnly`).
- Integration (`backend/tests/integration/projects_tasks_test.go`): `TestProjectTasksCore`, `TestTasksListingAndSubtasks`,
  `TestTaskRelationsAssigneesBulkAndMove`; run together with `TestSwaggerMatchesRoutes`, `TestSchemasAreOwnedAndIsolated`,
  `TestMigrationsAreIdempotentAndComplete` with `JAAS_TEST_DB_NAME=jaas_itest_tasks JAAS_TEST_REDIS_DB=11`.
- `go test -race` on the concurrency tests (updates, numbers, cycles, moves, WIP) is clean.

## Known gaps

- No search endpoint (TK-14) and no `My work` / board read models (planning context).
- Subtree moves between projects and re-parenting tasks that have subtasks are refused, not implemented.
- No restore of soft-deleted tasks and no purge job (30-day retention is the platform sweep, not built here).
- Cross-project relation lists load the caller's readable project ids once per request (`ReadableProjectIDs`); fine for the
  current scale, cache if profiling asks.
- The 50 000-task paging budget (PG7) has not been measured; the indexes for every sort are in migration 000012.
- The console page is not wired (lead): add `S_PM_TASKS` to the projects page scenarios.
