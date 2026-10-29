# M2 Automation, recurrence and public links: build notes

> Decision numbers in this file (DPJ-37..48) are local to this file; the project-wide records are DPJ-45 to DPJ-47 in `decisions.md`.

Packages `backend/internal/projects/automation` (rules and engine), `automation/bridge` (composition code), `recurrence` and `publiclink`. Extension points of specification section 9 (workflow rules, recurrence) plus public project links (decisions DPJ-37..DPJ-48 below).

## What was built

| Area | Behaviour |
|---|---|
| Rules | Table `workflow_rules` (trigger, all-of `conditions`, up to 10 `actions`, counters, failure flag). Triggers `task.created/moved/assigned/completed/commented/due_changed/pr_merged`. Conditions are `{field, value}` pairs over a closed list (`column_id`, `column_category`, `priority`, `label_id`, `assignee_id`; `to_column_id`/`from_column_id` for moved; `assigned_user_id` for assigned). Actions: `move_to_column`, `set_priority`, `add_label`, `assign_user`, `add_comment` (fixed text), `set_due_in_days`. No free-text matching, no expressions, no code. Max 50 rules per project. |
| Engine | `Engine.Tick` reads new rows of `projects_events_outbox` after a cursor kept in `automation_cursor`, independent of the broker relay. A lease on the cursor row allows one reader at a time. Loop every 5 s, batch 200, started from `startBackground` when `Background != nil`. |
| Runs | `workflow_rule_runs`, unique `(rule_id, event_id)`: a redelivered event is applied once per rule. Status `ok`, `skipped`, `failed` with a stable `error_code`. Pruned after 30 days. |
| Dry run | `POST .../rules/{rule_id}/test` evaluates a task of the project: failed conditions, assumed (event) conditions, whether the creator may still act, which actions would change something. Writes nothing, emits nothing. |
| Recurrence | Table `task_recurrences`; scheduler every 60 s claims due rows with `FOR UPDATE SKIP LOCKED` (batch 50), moves `next_run_at` ahead in that short transaction, then copies the template as the creator. Daily, weekly (weekday) and monthly (day of month, clamped to short months), interval 1..52, optional `until_on`. Max 20 per project. `run-now` copies as the caller and leaves the schedule alone. |
| Public links | Table `public_links` (SHA-256 hash only, 32-byte base64url token shown once, max 3 active per project). `GET /public/projects/{token}`: the third unauthenticated M2 route, per-IP limiter (60/min, shared Redis limiter when `Options.RateLimits` is set), cursor paged (limit <= 100), `Cache-Control: no-store`, `X-Robots-Tag: noindex`, `Referrer-Policy: no-referrer`. |

Routes: 15 (14 authenticated, 1 public; `scripts/pm_routes_automation.py`). Migrations 000070 rules + runs + cursor, 000071 recurrences, 000072 public links (up/down/up verified in a throwaway database).

## Decisions

| Decision | Reason | Alternatives / trade-off |
|---|---|---|
| DPJ-37 Contexts reach tasks through small interfaces (`automation.Tasks`, `recurrence.Tasks`) bound by `automation/bridge`, the only package that imports `task` and `collab`. | Contexts stay decoupled and the engine is testable with a fake. Every action goes through the same use cases as a person (permissions, WIP limits, done rules, versions). | Writing task tables directly (rejected: duplicates the rules). Importing `task` in the engine (rejected: couples contexts). |
| DPJ-38 Rules execute as their creator. At run time the engine re-checks that the creator is an active project member whose role holds the permission each action needs (`task.update`, `task.assign`, `task.comment`); otherwise the run is `skipped` with `NOT_ALLOWED` and the rule is flagged. A tenant admin who is not a member cannot be a rule creator in practice. | No new tenant permission and no privilege held by the rule itself; a demoted or removed creator immediately stops what the rule can do. | A service identity with its own rights (rejected: new permission model, M0 change). |
| DPJ-39 Loop protection: changes made by a rule carry agent label `automation.<8 hex of rule id>` (valid for `kernel.ValidAgent`); the engine ignores every event whose envelope `agent` starts with `automation`; at most 100 actions per rule per minute (`RATE_LIMITED`, a skip, not a failure); 20 consecutive failures disable the rule (`TOO_MANY_FAILURES`). | No cascades and no flooding, even for a rule that triggers itself. | Cascade depth counters (rejected: more state, still loops within the depth). Trade-off: rules cannot chain on purpose; use one rule with several actions. |
| DPJ-40 Idempotence by claim: the run row is inserted (status `failed`, code `INTERRUPTED`) before the actions run, then updated. Redelivery finds the row and does nothing. | At-most-once is safer than repeating a comment or a move. A crash leaves an honest `INTERRUPTED` row. | Insert after the actions (rejected: a crash would repeat actions). |
| DPJ-41 The engine has its own outbox cursor (`automation_cursor`, position `(created_at, id)`, lease) and reads only rows older than a 3 s settle delay measured on database time. | `created_at` is the transaction start time, so a transaction that commits late could land behind the cursor; the delay covers short transactions (ENGINEERING section 4). Independent of the relay and of the webhook cursor. | A sequence column on the outbox (rejected: shared table change; same commit-order gap). Latency of a rule is 3-8 s. |
| DPJ-42 Conditions are evaluated against the task as it is now, not as it was at the event; event-specific conditions read the envelope. A non-matching rule leaves no run row. | Cheap, no row per event and rule. Trade-off: a task changed again before the engine runs is judged on its newest state. | Recording every evaluation (rejected: table growth). |
| DPJ-43 When the creator is no longer allowed, every event of the trigger in that project records a `skipped NOT_ALLOWED` run and counts as a failure, so the rule disables itself after 20. | The flag is visible and the noise is bounded. | Silent skipping (rejected: nobody would notice). |
| DPJ-44 Recurrence catch-up collapses: a run claims its occurrence and moves `next_run_at` to the first occurrence after now; at most one task per recurrence per tick and missed occurrences are dropped. | A scheduler outage must not create a burst of identical tasks. Alternative: one task per tick until caught up (rejected: floods a board for a long outage). |
| DPJ-45 The recurrence claim commits before the copy is made (at most once per occurrence). The copy is the template duplicated (labels and assignees included), titled like the template (no "(copy)"), with its start moved to the run day and its due date keeping the template distance and time of day. It lands in the template column, so keep the template in a backlog column. | Short transactions; no duplicate tasks under concurrent schedulers. Trade-off: a crash between claim and copy loses one occurrence (visible: `last_run_at` unchanged). |
| DPJ-46 A recurrence that cannot run keeps its schedule and is flagged: `NOT_ALLOWED` (creator lost access or task.create), `PROJECT_ARCHIVED`, `RUN_FAILED`, `SCHEDULE_FAILED` (copy exists, dates failed); a missing template ends it (`TEMPLATE_GONE`). `until_on` ends it after the last allowed run. | Restoring a project resumes recurrences; broken ones are visible. |
| DPJ-47 Public page allow-list: project key and name; columns (name, category, color); tasks (ticket key, title, column name, priority, due date, label names, progress, optional description). Never user ids or names, comments, attachments, time, activity or members. Mention tokens in descriptions are replaced by `@someone`; descriptions are capped at 2000 characters. Tenant and project come from the token row only. | Anonymous readers must learn nothing about people. Trade-off: raw uuids typed by hand into a description are not filtered; descriptions are off unless the link says so. |
| DPJ-48 Unknown, revoked, malformed, archived-project and deleted-project tokens answer one identical 404 body, with the same no-store headers; the stored hash is compared in constant time. | No oracle for token or project existence. |
| Rule reads need `project.read`; runs, dry run and every write need `project.update`; link management needs `project.update`. | Members may see what automation does; operational detail and secrets stay with managers. |

## Deviations and integration notes

- `gen_projects_swagger.py` renders every `public` route with a `text/calendar` response; the public project page is JSON. The contract still lists the route; fix the generator if the content type matters.
- The public route path parameter `{token}` is typed `uuid` by the generator (all path params are); the token is 43 base64url characters.
- `task.pr_merged` is accepted as a trigger; nothing emits it until the git integration does.
- Event ordering inside the engine follows `(created_at, id)`, not strict commit order.

## Threat to evidence (merge into security.md)

| # | Threat | Control | Evidence |
|---|---|---|---|
| S24 | A rule used to escalate: a member sets up a rule that does more than they may | The rule runs as its creator through the task use cases; permissions are re-checked per run; no new permission; a removed or demoted creator is skipped and flagged | `TestNotAllowedCreatorIsSkippedAndFlagged`, `TestRemovedCreatorIsNotAllowed`, `TestRuleHonoursWipLimitsLikeAPerson` |
| S25 | Rule loops and floods | Agent label `automation.*` ignored by the engine; 100 actions per rule per minute; 20 consecutive failures disable; max 50 rules and 10 actions | `TestRuleMovesUrgentTaskAndDoesNotCascade`, `TestSelfTriggeringRuleStopsAfterOneRun`, `TestFloodCapSkipsRun`, `TestRuleDisablesAfterTwentyFailures` |
| S26 | Event redelivery repeats side effects | Unique `(rule_id, event_id)` claimed before the actions | `TestRedeliveredEventIsAppliedOnce` |
| S27 | Rule definitions that reach other projects or run code | Closed vocabulary; column, label and member ids checked against the project; extra members on an action refused | `TestRuleValidation` |
| S28 | A dry run with side effects | The test endpoint reads only; no run, no event, no task call | `TestDryRunChangesNothing` |
| S29 | Recurrence creating tasks without access, in archived projects or twice | Creator re-check, archived flag, `FOR UPDATE SKIP LOCKED` claim | `TestArchivedProjectAndLostAccessAreFlagged`, `TestConcurrentTicksNeverDuplicate`, `TestMissedRunsCollapseIntoOneCatchUp` |
| S30 | Public link leaks people or private data | Strict allow-list structs, mention scrubbing, description opt-in, tenant and project from the token row | `TestPublicViewIsAStrictAllowList`, `TestOnlyTheTokenProjectIsReadable`, integration `TestAutomationRecurrenceAndPublicLinksThroughTheRouter` |
| S31 | Token guessing, reuse after revocation, existence oracle | 256-bit token, hash only stored, constant-time compare, revoke is immediate, identical 404 bodies, per-IP rate limit, no-store/noindex/no-referrer | `TestUnknownRevokedMalformedAndHiddenProjectsAnswerIdentically`, `TestPublicRouteIsRateLimited`, `TestLinkLifecycleAndPermissions` |
| S32 | Silent database failures | Statement-level fault injection over every flow | `TestEveryStatementFailureIsReported` in each package |
