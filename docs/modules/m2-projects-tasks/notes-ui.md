# M2 Projects & Tasks - product UI notes

Status (2026-10-29): first product UI built at `/workspace` (not the test console). Code in `frontend/src/modules/projects/`, routes in
`frontend/src/app/workspace/`. The test console at `/dev-test` is untouched. The UI uses the public API contract only
(`backend/api/swagger.yaml`); call shapes mirror `backend/pkg/pmclient`.

## Routes

| Route | Page |
|---|---|
| `/workspace/sign-in` | Sign-in (organisation slug, e-mail, password; "Server" section holds API host and port) |
| `/workspace` | My work (landing page after sign-in): overdue / today / this week / later / no date with counts |
| `/workspace/projects` | Projects: search, status filter, favourites, role badge, create dialog, "load more" |
| `/workspace/projects/{projectId}/board` | Kanban with drag and drop, quick add, WIP markers, task drawer |
| `.../list` | Table with filters (text, column, priority, assignee, overdue, blocked), sort, direction, cursor paging; saved views bar; row checkboxes, select page and a bulk bar (move to column, set priority, archive after a confirmation) that lists each task outcome |
| `.../calendar` | Month grid from `GET /projects/{id}/calendar` (42-day window) |
| `.../gantt` | Gantt: 90-day window (earlier / later / today), week ticks, today line, milestone markers, bars (start to due) or one-day markers, `blocks` arrows, horizontal scroll, "Load more tasks" (cursor), "Show as a list" accessible fallback |
| `/public/projects/{token}?t=&h=&p=` | Anonymous read-only public project page (columns and tasks, "Load more tasks"); outside `/workspace`, no session |
| `.../backlog` | Backlog and sprints (tab shown only when `mode = scrum`): create, start, complete, move backlog task to sprint |
| `.../settings` | Four sections. General: columns (reorder, WIP limit, add, delete empty), labels, milestones, custom field definitions (create, rename, options, delete), archive/restore. Access: members and roles (built-in and custom roles in the pickers), custom roles (list, create, edit, delete), group rules (list, add, remove, refresh). Automation: rules (list, enable/disable, edit, runs, dry-run test, delete, create with one condition and one action) and recurring tasks (list, pause/resume, run now, edit schedule, delete, create). Webhooks & links: webhooks (create, enable/disable, test ping, rotate secret, delete, deliveries with redeliver), git integrations (GitHub / Gitea: create, rotate secret, delete, payload URL) and public links (create, revoke; full page URL shown once) |
| `/workspace/tasks/KEY-12` | Full-page task (deep link, target of search results). The same task opens as a drawer from board, list, backlog and calendar. Task actions (duplicate, archive/restore, move to project) and linked pull requests are part of it |

Header on every signed-in page: My work, Projects, global search (debounced 300 ms, `GET /search`, results link to the task page), sign out.
Projects are addressed by id in the URL (`GET /projects/{id}` takes an id, not the key).

## Structure (`frontend/src/modules/projects/`)

| Folder | Holds |
|---|---|
| `types/` | API shapes (common, project, task, collab, planning) |
| `api/` | `http.ts` (shared fetch wrapper), `errors.ts` (typed `ApiError`, messages), `session-store.ts` (tokens), `config.ts` (endpoint), `client.ts` (shared instance), `*-api.ts` (typed operations per area) |
| `lib/` | Pure logic: `rank-neighbours.ts`, `optimistic.ts`, `dates.ts`, `mentions.ts`, `paths.ts` (links, safe redirect), `format.ts`, `jwt.ts` |
| `hooks/` | `use-resource`, `use-paged-list`, `use-board` (optimistic move), `use-task-detail`, `use-project` (context), `use-session`, people/toast contexts |
| `components/` | `ui`, `shell`, `auth`, `projects`, `board`, `list`, `calendar`, `sprints`, `settings`, `task` (one concern per file, all under 300 lines) |
| `pages/` | One client component per route; the files in `src/app/workspace/**` are thin server wrappers (`params` is a Promise in this Next version and is awaited there) |
| `styles/` | Plain CSS (`base`, `board`, `task`, `views`), every class prefixed `pm-`, light and dark via `prefers-color-scheme`; no new dependency (the repo has no Tailwind) |
| `__checks__/` | `run.mjs`, node checks of the pure logic |

State is hooks and context only: `useResource` (load by key, stale-while-reload, optimistic `setData`), `usePagedList` (cursor "load more"),
`ProjectProvider` (project, columns, labels, members shared by the tabs), a small external store for the session and one for user names.
No npm dependency was added.

## API client behaviour (`api/http.ts`)

- Unwraps `{data, meta}`; errors become `ApiError {status, code, message, details}` (`details.currentVersion` from `VERSION_CONFLICT`).
- 401 on an authenticated call: one shared refresh (refresh tokens rotate, so concurrent calls wait for the same refresh), then the call is retried once; if the refresh fails or the retry is 401 again the session is ended and the guard redirects to sign-in. Login and refresh never trigger a refresh.
- 429: waits for `Retry-After` (default 2 s, capped at 30 s) and retries once, then surfaces `RATE_LIMITED`.
- Tenant addressing: browsers cannot set `Host`, so calls go to `http(s)://<slug>.<host>:<port>/api/v1/...` (the same scheme as the console; `*.localhost` resolves in Chrome and Firefox). Host and port default from `NEXT_PUBLIC_API_HOST` / `NEXT_PUBLIC_API_PORT` (default `localhost:8080`), can be changed in the sign-in form ("Server") and are remembered in localStorage (not secrets). Slug and host are validated before use.
- Concurrency: `PATCH /tasks/{id}` and `PUT /tasks/{id}/custom-fields` send the task `version` in the body; `POST /tasks/{id}/move` sends it too. A 409 `VERSION_CONFLICT` shows a message and reloads the latest data. Creates send an `Idempotency-Key`.
- Moves: `POST /tasks/{id}/move` with `column_id`, `before_id` (task goes before the card that follows it) or `after_id` (at the bottom), and `version`. The neighbour is computed from the visible destination cards (`lib/rank-neighbours.ts`). The board is updated at once (`lib/optimistic.ts`), rolled back on any error, reloaded on a version conflict; the soft-WIP warning (`warnings: ["WIP_LIMIT_EXCEEDED"]`) shows as a toast and the column header shows `count/limit` with a marker. Keyboard users move a card with the select on the card (shown on focus).

## Auth choice and risks

Choice: **access token in memory only; refresh token in `sessionStorage`** (tab-scoped, gone when the tab closes), restored on reload by one
refresh call, silent refresh scheduled one minute before expiry, sign-out calls `POST /auth/logout` and clears both.

Why: the API returns both tokens in the JSON body and has no cookie mode, so an httpOnly cookie is not available. Keeping the refresh token only
in memory would sign people out on every reload (refresh tokens rotate and the access token lives about 15 minutes). `localStorage` was rejected
(persists across tabs and restarts, longer exposure). `sessionStorage` is the smallest persistent store that keeps reload usable.

Limits and risks:
- Any XSS in this origin can read the refresh token from `sessionStorage` (and the access token from memory through the running page). Mitigations in the code: every user text is rendered as a React text node (no `dangerouslySetInnerHTML`), links are built from validated ids and keys (`lib/paths.ts`), the post-sign-in redirect accepts only `/workspace...` paths, download and upload URLs must be http(s). A strict Content-Security-Policy on the frontend host is still required and is not set by this UI (needs a shared config change).
- A duplicated tab copies `sessionStorage`; with rotating refresh tokens one of the two copies is invalidated and that tab is sent to sign-in.
- Tenant slug and server endpoint are in `localStorage` by design (not secrets). No secret is in the bundle; the only `NEXT_PUBLIC_` variables are the API host and port.
- The permanent fix is an httpOnly, SameSite refresh cookie issued by the API (or a same-origin BFF route handler); the session store is the only file that would change.

## Authorisation in the UI

The UI hides controls from the project role of the caller (`my_role`, tenant admin from the token roles) and also remembers any 403 from a settings action and hides those controls (`use-guarded-action.ts`). This is convenience only: every operation is authorised by the server and a refused call shows "You do not have permission".

## Not covered / known gaps

- **Names:** shown from `display_name` of `GET /projects/{id}/members` (first and last name, never the e-mail), remembered per session by `hooks/people-store.ts` whenever a members list is loaded (project workspace, task page). A person who is no longer a member, or an API without the field, shows as `User 5a0a266c`. My work and search results show no people. The "add member" picker in Settings is the one place that uses the administrator-only user directory (first 100 users, name and e-mail) and is empty for other roles.
- Not built: saved-view editing and layouts other than the list (views are created, applied and deleted; the "blocked" filter is not part of the server view format and is not saved; a view with several values in one filter applies only the first), rules with several conditions or actions (shown and runnable, but not editable here: the edit form would drop parts, so it says so), changing the template task of a recurrence, tenant-wide labels, watchers, time report, sprint report, iCalendar feed tokens, the per-task "force" options of move and bulk (blocked / open-subtask overrides).
- Group rules need a reference id (department, team or the manager's user id) typed or pasted; there is no department/team directory in the API yet, so the form only validates the id format.
- Webhook and git integration secrets and the public-link URL are shown once in a copy box that lives in component memory only (never stored; the walk checked that browser storage holds no secret). Closing the box or leaving the page loses it; rotate or recreate to get a new one. The public page URL is `/public/projects/{token}?t={tenant}&h={api host}&p={api port}`: the API needs the tenant host, so the tenant slug and API address travel in the query (none of them secret; the token is the only secret). The public page uses no session, sends no authentication header, shows a neutral "Not found" for any 404 (unknown, revoked, archived) and for a missing or malformed tenant address, is `noindex, nofollow, nocache` with `referrer: no-referrer`, and has no links or actions. A bad token is therefore indistinguishable from a bad address.
- Gantt: dependencies are drawn as straight-ish arrows only when both tasks are on screen; tasks without any date are not returned by the API and so are not shown; no drag to reschedule.
- Task actions: duplicate (with optional labels and assignees), archive (confirmed) / restore and move to another project (project pick only; the API route also accepts a target column and a version, the UI sends the task version and lets the server choose the column). A task with a parent or subtasks is refused by the server (409 `TASK_HAS_HIERARCHY`), shown as a message. After a move the old ticket key keeps resolving; the UI navigates to the new key.
- Linked code (git integrations): the drawer lists links of kind pr / branch / commit with state; only `https://` URLs without credentials become links (`target=_blank`, `rel="noopener noreferrer"`), anything else is plain text. Creating links needs a signed provider call; the walk produced one with a signed test payload.
- Custom project roles are tenant-wide and need a tenant permission: the section shows "Not available for your role" on 403. The role pickers (add member, change role) list the built-in roles and the custom roles; custom role permissions come from `GET /project-permissions`.
- Rule dry run needs a task key and shows which actions would run, assumed conditions (for example the target column of a move) and failed ones; rule runs show status and action count only (the API holds no run detail).
- Blocked marker on board cards: the board payload has no `blocked` flag, so it shows on list/task views only. Overdue on cards is computed on the client from `due_at` and the column category.
- Comments render markdown source as plain text (line breaks kept, mentions highlighted); there is no markdown renderer and no rich editor. Mentions are inserted with a "Mention" picker as `@[Name](user:uuid)` and otherwise stay plain text.
- Attachment upload PUTs to the presigned URL from the browser: the object store must allow the UI origin (CORS on the store) or the upload shows "The file could not be sent to storage". Not verified against the dev store in the browser walk.
- `Retry-After` is now exposed by the API CORS configuration, so the client reads it (capped at 30 s); the 2 s default applies only when the header is absent.
- The task drawer is local state; the URL does not change while it is open (deep link: `/workspace/tasks/KEY-12`).
- Board paging loads 30 cards per column and "Load more" per column; drag and drop onto a column whose later cards are not loaded places the card relative to the loaded neighbour only.
- Native HTML5 drag and drop does not work on touch screens; the per-card select is the touch and keyboard path.
- Accessibility was designed (labels, native dialog with focus trap, `aria-current`, live regions, visible focus) but not audited with a screen reader.
- No automated component or end-to-end tests exist in the repo (no framework, none added); the browser walk below is a manual script kept outside the repo.

## Checks and results (2026-10-30)

Run from `frontend/`:

```bash
node_modules/.bin/tsc --noEmit                                         # clean
node_modules/.bin/eslint src/modules/projects src/app/workspace        # 0 errors, 0 warnings, no eslint-disable
node src/modules/projects/__checks__/run.mjs                           # 31 checks passed (run.mjs + run-planning.mjs, shared harness.mjs)
npm run build                                                          # run in the first round (compiled); NOT re-run for the follow-up because the dev server was running - the lead runs it
```

`run.mjs` transpiles the pure TypeScript (api, lib, types) with the repo's `typescript` into a temp folder and checks with `node:assert`:
error mapping, `{data, meta}` unwrapping, bearer and tenant URL, 401 -> refresh -> one retry, lost session, no refresh for public calls, 429 with
`Retry-After`, network errors, version in the PATCH body, path escaping, rank neighbours for drop positions, optimistic move without mutating the
rollback snapshot, server-value reconcile, WIP and paging helpers, calendar grid, due date round trip, overdue rule, mention parsing, redirect
safety, minute parsing, JWT claims. Follow-up checks: bulk selection and per-task result summary, Gantt scale / bar / marker / today / window maths, arrows and page merging, saved view <-> filter mapping, rule / recurrence / webhook form building and validation. Last round: role picker encode/decode, safe https links, integration payload URL, public page URL and query validation, rule / recurrence to form mapping (and which rules are not editable).

Headless Chrome smoke walk (DevTools protocol, script in the session scratchpad, not in the repo) against the lead's servers (frontend :3000, API :8091,
tenant `uitest`): **28/28 steps passed, no console errors**: guard redirect to sign-in, sign-in through the form, My work, projects list and search,
create project and land on its board, quick add of three tasks, drag to another column, drop above a card, reorder inside a column, order kept after
reload (session restored from the refresh token), task drawer with all sections, comment, time log, priority edit with version, list / calendar /
settings tabs, Backlog tab only for the scrum project, task deep link with custom fields, global search, no horizontal scroll at 360 px on the board
and My work, sign out and session gone. Screenshots (scratchpad only):
`C:\Users\Admin\AppData\Local\Temp\claude\D---pycache-personal-JASS\1757393a-4e63-44d1-81b5-d0d81f93332e\scratchpad\shots\` (`02-mywork`, `05-board`, `06-drawer`, `09-taskpage`, `11-mobile-board`, ...).
The walk created data in tenant `uitest` (projects `W...` and `SHAPE`).

Follow-up walk (2026-10-30, same servers, API rebuilt with `display_name`): **31/31 steps passed, no console errors, no horizontal scroll at 360 px on Gantt, List and Settings**:
names from `display_name` on the task page, row selection and bulk priority with the per-task result list, archive confirmation (cancelled), save / apply / delete a saved view,
Gantt bars, today line, milestone, blocks arrow and list fallback, settings: milestone and custom field create/delete, group rule id validation, rule create/disable/delete,
webhook creation with the secret shown once and absent from `localStorage`/`sessionStorage`, public link shown once, deliveries list. The first run of the walk found a real defect:
names stayed as short ids because React Compiler reused memoised output when the names store changed; `usePeople` now returns a new function per names version (fixed, walk re-run).
The walk created data in tenant `uitest` (project `SHAPE`: milestone, rule, recurrence, webhook `hook`, a public link, saved view; walk-created items were deleted).

Last-round walk (2026-10-30): **27/28 steps passed on the first full run, the one failure was my check, not the app** (it counted `sessionStorage` keys on the public page and found Next.js own dev key `__next_debug_channel`; checked separately: no `jaas.*` key, no `localStorage` key, no anchors on the public page). The walk covered: linked pull request link with `rel="noopener noreferrer"`, duplicate, archive with confirmation, restore, move to another project (lands on the new key), custom roles create / edit / delete and custom roles in the role picker, rule dry run, runs panel, rule edit, recurrence edit, git integration payload URL and one-time secret, public link URL shown once, public page without a session (`noindex, nofollow, nocache`), unknown token and missing tenant address answer "Not found", 360 px without horizontal scroll, revoked link answers "Not found". The walk found one real defect, fixed: a task whose activity was written by the system (git integration link, rules, recurrences) has no `actor_user_id`, and the people lookup crashed the task page on `null.slice`; `nameFor` now returns "System" for it. The expected 404 responses of the public page checks appear in the browser log as network errors, not as app errors.
Data left in tenant `uitest` (project `SHAPE`): copies SHAPE-3 / SHAPE-4 of the first task, a task moved to another project, git integration `walk-seed` and the PR link on SHAPE-1, extra custom roles from earlier test runs. `npm run build` was not run (dev server running); the lead builds.
