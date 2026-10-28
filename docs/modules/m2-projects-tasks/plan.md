# M2 Projects & Tasks - plan

Updated 2026-10-28. Branch `module-2` (pushed to `origin` after each phase). Steps follow the standard loop (`docs/ENGINEERING.md`):
spec, migrations (up/down/up), service and HTTP, tests on a real PostgreSQL, integration ring, swagger, console scenario, docs,
full suites, commit.

**Status: phases 1-3 (P0-P7) BUILT and verified; phases 4-5 (P8) are designed for and not built.**

| Step | Scope | Status |
|---|---|---|
| P0 | Design package approved; open questions answered | DONE 2026-10-28 (`decisions.md`, "Owner answers") |
| P1 | Foundations: `projects` schema in the migration runner, `shared/identityport` promotion, `shared/storage` (S3 adapter, in-memory fake, dev S3 service), `shared/outbox.TableRelay`, `shared/inbox`, import-boundary test | DONE |
| P2 | `project` context: projects, roles, members, columns, labels, milestones, favourites, authorizer; tenant permissions in the M0 catalogue; console page | DONE (35 operations) |
| P3 | `task` core: tasks (numbers, versions, ranks, moves, bulk), assignees, labels, subtasks, relations with cycle checks, ticket lookup, archive/delete, activity | DONE (23 operations, `notes-tasks.md`) |
| P4 | Collaboration: comments and mentions, watchers, time tracking, attachments through object storage, sweeps | DONE (21 operations, `notes-collab.md`) |
| P5 | `planning`: sprints, backlog and plan moves, board/calendar/Gantt/My work, search, saved views, calendar feed tokens | DONE (25 operations, `notes-planning.md`) |
| P6 | Group rules (department/team/reports-of) with an HR people adapter, deactivation of logins out of projects, inbox ledger | DONE (4 operations) |
| P7 | Performance seed (PG7), docs, `SYSTEM_REFERENCE`, console scenarios, headless/browser UI run | DONE (see `testing.md`) |
| P8 (later) | Product UI, custom fields, automation rules, webhooks, GitHub/Gitea and chat integrations, MCP and CLI, recurrence, public links | NOT BUILT - extension points in `specification.md` section 9 |

Definition of done per step: unit coverage >= 85% on services (>= 90% where stated in `testing.md`), integration ring green with
`JAAS_REQUIRE_INTEGRATION=1`, swagger drift test green, migrations up/down/up, console scenario passing live, docs current.

## Known gaps (carried to the next phase)

- Watchers are not removed when a user loses access to a project; delivery filtering belongs to M10 (notifications are not built).
- The iCalendar feed limiter is per process; use the shared Redis limiter when `projects.Options` gets a Redis handle.
- Group-rule membership lags HR by up to ten minutes unless refreshed explicitly (`POST /projects/{id}/group-rules/refresh`).
- No broker consumer exists yet; `HandleIdentityEvent` is ready (inbox-deduplicated) and the in-process deactivation hook covers
  identity deactivation and HR terminations today.
- No real product UI: the test console (`/dev-test/projects*`) is the only frontend.
- Attachment upload bytes are not malware-scanned (a no-op scan hook exists).
- Subtree moves between projects and re-parenting tasks that have subtasks are refused (409), not implemented.
- Generic platform gaps shared with M1 (CI pipeline, metrics, tracing, backups, runbooks, secret manager, load tests).
