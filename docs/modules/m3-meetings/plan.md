# M3 Meetings - plan

Updated 2026-11-04. It will be built on its own branch `module-3`, cut from `main` after `module-2` is merged (or from
`module-2` if the owner says so). Each step follows the standard loop (`docs/ENGINEERING.md`).

**Status: BACKEND BUILT and tested on branch `module-3` (2026-11-04); test console page, scenarios, browser walks and doc refresh still to do.**

| Step | Scope | Status |
|---|---|---|
| P0 | Research, the 12 owner answers, decisions DMT-01..20, phase-1 specification, design, security, testing plan | DONE |
| P0a | M0 audit fix: resource-role changes audited under the resource type; ResourceAccess unit tests | DONE (`c43dd52`) |
| P1 | M0: resource type `meeting_series` (allowlist, migration for `applies_to`), meeting role templates and permissions (DMT-09/10); M0 and M2 suites re-run | DONE |
| P2 | Foundations: schema `meetings` in the migration runner, `tasksport` (M2 adapter), outbox/inbox wiring, import-boundary test | DONE |
| P3 | `series` and `occurrence`: create, list, read, update, cancel, split, exceptions, materialiser plus window job, RSVP, participants and roles, authorizer; golden recurrence tests | DONE |
| P4 | `agenda`, private points, `minutes` (sections, versions, publish, corrections), decisions, attendance, consent | DONE |
| P5 | `actions` (create or link M2 tasks, snapshots, events), deactivation hook, retention purge, `starting_soon` scheduler | DONE |
| P6 | `feed` (tokens, iCal, `.ics`), search, My meetings | DONE |
| P7 | Swagger, console page and scenarios (J1-J12), product UI screens, performance seed, docs, `SYSTEM_REFERENCE`, full suites | IN PROGRESS: swagger done (56 ops), console specs generated; console page/scenarios, walks, docs, full suite TODO |

Phase 2 (rooms, group invites, templates, carry-over, reports, context pack) starts after a scope go-ahead.

## Open items (not blocking P1-P3)

- Legal review of the consent basis and retention defaults (DMT-14) before production.
- The M2 personal inbox project (DMT-05) needs an M2 decision when phase 2 starts.
- Platform-wide RLS (DMT-08) and per-user API tokens are separate platform decisions.
