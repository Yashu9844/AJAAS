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
| `.../list` | Table with filters (text, column, priority, assignee, overdue, blocked), sort, direction, cursor paging |
| `.../calendar` | Month grid from `GET /projects/{id}/calendar` (42-day window) |
| `.../backlog` | Backlog and sprints (tab shown only when `mode = scrum`): create, start, complete, move backlog task to sprint |
| `.../settings` | Columns (reorder, WIP limit, add, delete empty), labels, members and roles, archive/restore |
| `/workspace/tasks/KEY-12` | Full-page task (deep link, target of search results). The same task opens as a drawer from board, list, backlog and calendar |

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

- **Names:** the API carries user ids only. Names are looked up one id at a time with `GET /users/{id}` (cached per session); that is administrator-only, so other users see `User 5a0a266c`. The member picker in Settings lists only the first 100 users. A people directory endpoint readable by project members is needed (lead decision).
- Gantt view, saved views, milestones UI, group rules UI, custom role management, custom field definition UI (values are editable, definitions are not), bulk update, duplicate/move-project/archive task actions, watchers, time report, sprint report, iCalendar feed tokens: not built.
- Blocked marker on board cards: the board payload has no `blocked` flag, so it shows on list/task views only. Overdue on cards is computed on the client from `due_at` and the column category.
- Comments render markdown source as plain text (line breaks kept, mentions highlighted); there is no markdown renderer and no rich editor. Mentions are inserted with a "Mention" picker as `@[Name](user:uuid)` and otherwise stay plain text.
- Attachment upload PUTs to the presigned URL from the browser: the object store must allow the UI origin (CORS on the store) or the upload shows "The file could not be sent to storage". Not verified against the dev store in the browser walk.
- `Retry-After` is not in `Access-Control-Expose-Headers`, so a browser cannot read it and the 2 s default applies. Adding it to `corsExposedHeaders` in `backend/internal/shared/middleware/cors.go` fixes that (outside this task's files).
- The task drawer is local state; the URL does not change while it is open (deep link: `/workspace/tasks/KEY-12`).
- Board paging loads 30 cards per column and "Load more" per column; drag and drop onto a column whose later cards are not loaded places the card relative to the loaded neighbour only.
- Native HTML5 drag and drop does not work on touch screens; the per-card select is the touch and keyboard path.
- Accessibility was designed (labels, native dialog with focus trap, `aria-current`, live regions, visible focus) but not audited with a screen reader.
- No automated component or end-to-end tests exist in the repo (no framework, none added); the browser walk below is a manual script kept outside the repo.

## Checks and results (2026-10-29)

Run from `frontend/`:

```bash
node_modules/.bin/tsc --noEmit                                         # clean
node_modules/.bin/eslint src/modules/projects src/app/workspace        # 0 errors, 0 warnings, no eslint-disable
node src/modules/projects/__checks__/run.mjs                           # 21 checks passed
npm run build                                                          # compiled; /workspace routes listed
```

`run.mjs` transpiles the pure TypeScript (api, lib, types) with the repo's `typescript` into a temp folder and checks with `node:assert`:
error mapping, `{data, meta}` unwrapping, bearer and tenant URL, 401 -> refresh -> one retry, lost session, no refresh for public calls, 429 with
`Retry-After`, network errors, version in the PATCH body, path escaping, rank neighbours for drop positions, optimistic move without mutating the
rollback snapshot, server-value reconcile, WIP and paging helpers, calendar grid, due date round trip, overdue rule, mention parsing, redirect
safety, minute parsing, JWT claims.

Headless Chrome smoke walk (DevTools protocol, script in the session scratchpad, not in the repo) against the lead's servers (frontend :3000, API :8091,
tenant `uitest`): **28/28 steps passed, no console errors**: guard redirect to sign-in, sign-in through the form, My work, projects list and search,
create project and land on its board, quick add of three tasks, drag to another column, drop above a card, reorder inside a column, order kept after
reload (session restored from the refresh token), task drawer with all sections, comment, time log, priority edit with version, list / calendar /
settings tabs, Backlog tab only for the scrum project, task deep link with custom fields, global search, no horizontal scroll at 360 px on the board
and My work, sign out and session gone. Screenshots (scratchpad only):
`C:\Users\Admin\AppData\Local\Temp\claude\D---pycache-personal-JASS\1757393a-4e63-44d1-81b5-d0d81f93332e\scratchpad\shots\` (`02-mywork`, `05-board`, `06-drawer`, `09-taskpage`, `11-mobile-board`, ...).
The walk created data in tenant `uitest` (projects `W...` and `SHAPE`).
