# M2 Projects & Tasks — plan

Status: **design awaiting approval.** Steps follow the standard loop (`docs/ENGINEERING.md`): spec → migrations (up/down/up) → models/store/service/http → unit tests with fakes → integration test → swagger → console scenario → docs → full suites → commit.

| Step | Scope | Gate |
|---|---|---|
| P0 | This design package approved; open questions answered | owner GO |
| P1 | Foundations: `projects` schema and migration directory wired into the runner; `shared/identityport` promotion; `shared/storage` port + S3 adapter + fake + MinIO in compose; `shared/outbox` relay generalised; inbox table and consumer harness; M2 added to the import-boundary test | schema GO |
| P2 | `project` context: projects, roles, members, columns, labels, milestones, favorites; M0 permission catalogue entries; console page skeleton | |
| P3 | `task` core: tasks (numbers, versions, ranks, moves, bulk), assignees, labels, subtasks, archive/delete, ticket lookup, activity | |
| P4 | Relations with cycle checks, comments with mentions, watchers, time tracking, attachments (init/complete/download, sweeps) | |
| P5 | `planning`: sprints, backlog and planning moves, board/list/calendar/Gantt/my-work read models, search, saved views, iCalendar tokens | |
| P6 | Group rules (department/team/reports-of) and the inbox consumers (user deactivated, HR termination) | |
| P7 | Performance seed test, regenerate docs and `SYSTEM_REFERENCE`, console scenarios for every golden test, headless and browser UI run | |
| P8 (later) | Product UI, custom fields, automation, webhooks, integrations, MCP/CLI (phases 4–5) | scope GO |

Definition of done per step: unit coverage ≥ 85% on services, integration ring green with `JAAS_REQUIRE_INTEGRATION=1`, swagger drift test green, migrations up/down/up, console scenario passing live, docs current.
