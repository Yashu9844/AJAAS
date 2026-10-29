# M2 Projects & Tasks - product UI notes

Status (2026-10-29): first product UI built at `/workspace` (not the test console). Code in `frontend/src/modules/projects/`, routes in
`frontend/src/app/workspace/`. The test console at `/dev-test` is untouched. The UI uses the public API contract only
(`backend/api/swagger.yaml`); call shapes mirror `backend/pkg/pmclient`.

## Routes

| Route | Page |
|---|---|
| `/workspace/sign-in` | Sign-in (organisation slug, e-mail, password; "Server" section holds API host and port) |
| `/workspace` | My work (landing page after sign-in): overdue / today / this week / later / no date with counts; "Calendar feed" section (iCalendar tokens: create, list, revoke; the feed address with the raw token is shown once) |
| `/workspace/projects` | Projects: search, status filter, favourites, role badge, create dialog, "load more" |
| `/workspace/projects/{projectId}/board` | Kanban with drag and drop, quick add, WIP markers, task drawer |
| `.../list` | Table with filters (text, column, priority, assignee, overdue, blocked), sort, direction, cursor paging; saved views bar; row checkboxes, select page and a bulk bar (move to column, set priority, archive after a confirmation) that lists each task outcome |
| `.../calendar` | Month grid from `GET /projects/{id}/calendar` (42-day window) |
| `.../gantt` | Gantt: 90-day window (earlier / later / today), week ticks, today line, milestone markers, bars (start to due) or one-day markers, `blocks` arrows, horizontal scroll, "Load more tasks" (cursor), "Show as a list" accessible fallback |
| `/public/projects/{token}?t=&h=&p=` | Anonymous read-only public project page (columns and tasks, "Load more tasks"); outside `/workspace`, no session |
| `.../time` | Time report: period (default last 30 days), group by task or person, logged time against the total estimate |
| `.../backlog` | Backlog and sprints (tab shown only when `mode = scrum`): create, start, complete, move backlog task to sprint, sprint report (committed vs completed counts and estimates; live numbers until the sprint is completed) |
| `.../settings` | Four sections. General: columns (reorder, WIP limit, add, delete empty), project labels (with the organisation labels of the project shown and marked), organisation labels (list, create, edit, delete; hidden on 403), milestones, custom field definitions (create, rename, options, delete), archive/restore. Access: members and roles (built-in and custom roles in the pickers), custom roles (list, create, edit, delete), group rules (list, add, remove, refresh). Automation: rules (list, enable/disable, edit, runs, dry-run test, delete, create; the editor takes up to 10 conditions and 10 actions and keeps parts it does not know) and recurring tasks (list, pause/resume, run now, edit schedule, delete, create). Webhooks & links: webhooks (create, enable/disable, test ping, rotate secret, delete, deliveries with redeliver), git integrations (GitHub / Gitea: create, rotate secret, delete, payload URL) and public links (create, revoke; full page URL shown once) |
| `/workspace/tasks/KEY-12` | Full-page task (deep link, target of search results). The same task opens as a drawer from board, list, backlog and calendar. Task actions (duplicate, archive/restore, move to project), watchers (watch / stop watching, watcher list with names) and linked pull requests are part of it |

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

Closed in the last rounds and therefore not listed any more: multi-condition rule editing, organisation labels, watchers, time and sprint reports, calendar feed tokens, attachment upload against the dev store, the automated accessibility audit.

- **Names:** shown from `display_name` of `GET /projects/{id}/members` (first and last name, never the e-mail), remembered per session by `hooks/people-store.ts` whenever a members list is loaded (project workspace, task page). A person who is no longer a member, or an API without the field, shows as `User 5a0a266c`; activity written by the system (rules, git integrations, recurrences) shows as "System". My work and search results show no people. The "add member" picker in Settings is the one place that uses the administrator-only user directory (first 100 users, name and e-mail) and is empty for other roles.
- **Not built:** saved-view editing and layouts other than the list (views are created, applied and deleted; the "blocked" filter is not part of the server view format and is not saved; a view with several values in one filter applies only the first), changing the template task of a recurrence, the per-task "force" options of move and bulk (blocked / open-subtask overrides), a page that renders the project public link inside `/workspace` (the anonymous page lives at `/public/projects/{token}`).
- Rule runs show status and action count only (the API holds no run detail). Rule conditions are all-of; the API has no any-of.
- Group rules need a reference id (department, team or the manager's user id) typed or pasted; there is no department/team directory in the API yet, so the form only validates the id format.
- Secrets and tokens (webhook and git secrets, public page address, calendar feed address) are shown once in a copy box that lives in component memory only (never stored; the walks check that browser storage holds none). Closing the box or leaving the page loses it; rotate or recreate to get a new one. The public page URL is `/public/projects/{token}?t={tenant}&h={api host}&p={api port}`: the API needs the tenant host, so the tenant slug and API address travel in the query (none of them secret; the token is the only secret). The public page uses no session, sends no authentication header, shows a neutral "Not found" for any 404 (unknown, revoked, archived) and for a missing or malformed tenant address, is `noindex, nofollow, nocache` with `referrer: no-referrer`, and has no links or actions.
- Gantt: dependencies are drawn as simple arrows only when both tasks are on screen; tasks without any date are not returned by the API and so are not shown; no drag to reschedule.
- Task actions: move sends only a project pick and the task version (the API also accepts a target column); a task with a parent or subtasks is refused by the server (409 `TASK_HAS_HIERARCHY`) and shown as a message. Linked code: only `https://` URLs without credentials become links (`target=_blank`, `rel="noopener noreferrer"`).
- Custom project roles are tenant-wide and need a tenant permission: the section shows "Not available for your role" on 403.
- Blocked marker on board cards: the board payload has no `blocked` flag, so it shows on list/task views only. Overdue on cards is computed on the client from `due_at` and the column category.
- Comments render markdown source as plain text (line breaks kept, mentions highlighted); there is no markdown renderer and no rich editor.
- The task drawer is local state; the URL does not change while it is open (deep link: `/workspace/tasks/KEY-12`).
- Board paging loads 30 cards per column and "Load more" per column; drag and drop onto a column whose later cards are not loaded places the card relative to the loaded neighbour only.
- Native HTML5 drag and drop does not work on touch screens; the per-card select is the touch and keyboard path.
- A page reload while a token refresh is in flight can lose the rotated refresh token and sign the person out (seen once in the walks when the dev server recompiled and reloaded the page mid-refresh). Production builds do not hot-reload; a server-side refresh grace period or an httpOnly cookie would remove the window.
- No automated component or end-to-end tests exist in the repo (no framework, none added); the browser walks are manual scripts kept outside the repo, the accessibility audit helper is inside it (`__checks__/a11y-audit.mjs`).

## Attachment upload and object-store CORS

The attachment flow (init, PUT to the presigned URL, complete) was exercised in the browser walk against the dev store (adobe/s3mock on `http://localhost:9090`, bucket `jaas-files`) with the page origin `http://localhost:3000`: it works with no configuration. Observed with curl: a preflight `OPTIONS` to the presigned URL with `Origin: http://localhost:3000`, `Access-Control-Request-Method: PUT` and `Access-Control-Request-Headers: content-type` answers `200` with `Access-Control-Allow-Origin: http://localhost:3000`, `Access-Control-Allow-Methods: PUT`, `Access-Control-Allow-Headers: content-type` and `Access-Control-Allow-Credentials: true`; the `PUT` itself answers `200` with `Access-Control-Allow-Origin: *`, an `ETag` and `Access-Control-Expose-Headers: *`. So no S3Mock setting and no bucket CORS call is needed for development.

Production note: the bucket must allow the UI origin (the exact origin of the product UI, https) for `PUT` with the headers the API returns in `upload.headers` (at least `Content-Type`) and expose nothing else; presigned URLs expire after 5 minutes. A bucket without that CORS rule makes the browser block the `PUT` and the UI shows "The file could not be sent to storage. The storage server may be unreachable or may not allow this site."

## Accessibility audit (automated, not a screen-reader test)

`__checks__/a11y-audit.mjs` exports `auditPage(options)`: plain JavaScript that the headless walk injects into the page (`(${auditPage.toString()})(options)`) on every main screen, in light and dark colour schemes. It checks: accessible name of every visible button, link, input, select, textarea, summary and role=button/tab/link; form controls have an associated label; images have `alt`; exactly one visible `h1` and no skipped heading levels; `header`, `nav` and `main` landmarks (the sign-in and public pages only require `main`); every `dialog` has `role="dialog"`, `aria-modal="true"` and a name; no actionable element has a negative tabindex and no custom control is unfocusable; a live region exists for async messages; duplicate ids and dangling `aria-labelledby`; text contrast against the effective background from computed styles (WCAG ratio, 4.5:1, 3:1 for large text). The helper is self-tested in the walk by injecting broken markup and requiring the matching rules to fire. The walk also tests the dialog focus behaviour with real key events: focus moves into the dialog on open, 14 Tab presses stay inside (an explicit focus trap in `Dialog.tsx`), Escape closes it and focus returns to the opener.

Result of the last run: every screen passes: sign-in, My work, Projects (also with the new-project dialog open), board (also with the task drawer open), list, calendar, Gantt, time, backlog, the four Settings sections, the full task page and the public page, and in the dark scheme the public page, My work, board, task page, Gantt and Settings. Findings fixed on the way: the favourite star was 2.45:1 (now a darker token, 5:1 and a light token in the dark scheme); dialogs lacked an explicit role and `aria-modal`; the sign-in page had no `main` landmark; the board, list, Gantt and public pages jumped from `h1` to `h3` (a visually hidden `h2` was added); the task page had no `h1` and its sections were `h3` (the title is the `h1` on the full page; sections are `h2`); Tab left the dialog for the browser chrome (explicit trap added).

What this does not cover and a human audit should: real screen reader runs (NVDA, JAWS, VoiceOver) and the reading order and wording of the announcements; focus order across the whole page and focus visibility on every state (only presence of a focus style is designed, not measured); keyboard operation of drag and drop alternatives and of the Gantt chart beyond the list fallback; contrast of text on images, hover and focus states, disabled controls and the colours chosen by users for labels and columns; zoom to 200% and 400%, text spacing and reduced-motion behaviour; target size; error identification and instructions for every form; correctness of ARIA relationships beyond names (for example `aria-expanded` on every disclosure); touch accessibility; cognitive load and plain language.

## Checks and results (2026-10-30)

Run from `frontend/`:

```bash
node_modules/.bin/tsc --noEmit                                         # clean
node_modules/.bin/eslint src/modules/projects src/app/workspace        # 0 errors, 0 warnings, no eslint-disable
node src/modules/projects/__checks__/run.mjs                           # 32 checks passed (run.mjs + run-planning.mjs, shared harness.mjs)
npm run build                                                          # run in the first round (compiled); NOT re-run for the follow-up because the dev server was running - the lead runs it
```

`run.mjs` transpiles the pure TypeScript (api, lib, types) with the repo's `typescript` into a temp folder and checks with `node:assert`:
error mapping, `{data, meta}` unwrapping, bearer and tenant URL, 401 -> refresh -> one retry, lost session, no refresh for public calls, 429 with
`Retry-After`, network errors, version in the PATCH body, path escaping, rank neighbours for drop positions, optimistic move without mutating the
rollback snapshot, server-value reconcile, WIP and paging helpers, calendar grid, due date round trip, overdue rule, mention parsing, redirect
safety, minute parsing, JWT claims. Follow-up checks: bulk selection and per-task result summary, Gantt scale / bar / marker / today / window maths, arrows and page merging, saved view <-> filter mapping, rule / recurrence / webhook form building and validation. Final round: rule draft round trip (several conditions and actions, unknown action kept as stored, limits, trigger change), logged-vs-estimate, report period, sprint summary, feed address. Previous round: role picker encode/decode, safe https links, integration payload URL, public page URL and query validation, rule / recurrence to form mapping (and which rules are not editable).

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

Final-round walks (2026-10-30, tenant `uitest`, project `SHAPE`, frontend :3000 and API :8091), every script finished with zero failed steps and no unexpected console errors: **139/139 steps** in four scripts (28 + 31 + 28 + 52). The new script (52 steps) covers: the accessibility audit on 20 screens and states, the audit helper self-test, dialog focus trap and restore with real key events, calendar feed (create, address shown once, the feed answers `200 text/calendar`, revoke, revoked feed answers 404), time report (task and person grouping), sprint report, organisation and project labels on the task page, watch toggle and watcher list, **real attachment upload to the dev S3 server through the browser (init, PUT, complete, listed, deleted)**, organisation label create / edit / delete, and a rule with two conditions and two actions created and then edited (a third condition added) without losing the others. Expected 404 responses of the public-page and revoked-feed checks show up in the browser log as network errors, not app errors. Walk-script defects fixed on the way (not app defects): the `sessionStorage` check now ignores Next.js's own `__next_debug_channel` key, rule and public-link steps follow the new UI. One walk run failed because the dev server recompiled and reloaded the page while a token refresh was in flight (see the refresh note above); the re-run passed.
Data left in tenant `uitest` (project `SHAPE`): copies of tasks, a task moved to another project, a git integration `walk-seed` and its PR link, a webhook `hook`, calendar token `probe`, organisation label `org-wide`, and extra custom roles from earlier test runs. `npm run build` was not run (dev server running); the lead builds.
