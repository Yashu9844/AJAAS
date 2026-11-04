# M3 Meetings - Design (phase 1, approved 2026-11-04)

The architecture was chosen in `research/architecture-options.md` (Option A) and refined by `decisions.md`. This file
is the build design. It changes when the build differs, and its "As built" section records what was built.

## 1. Boundary and packages

`backend/internal/meetings/` runs in-process like `internal/projects`. Other modules are reached only through ports.

| Package | Holds |
|---|---|
| `api` | DTOs, `Actor`, error codes, the routes the HTTP layer exposes |
| `series` | series, participants, roles (through the M0 port), RRULE validation, materialiser, split/exception/cancel |
| `occurrence` | occurrence reads and writes, status clock, RSVP, attendance, consent |
| `agenda` | agenda items, private talking points, ranks |
| `minutes` | note sections with versions, publish, corrections, decisions |
| `actions` | action item links, M2 `Tasks` port adapter, snapshot refresh |
| `feed` | feed tokens, iCal rendering (`golang-ical`), `.ics` per occurrence |
| `authz` | the single `Authorizer` (DMT-09..12) |
| `jobs` | window extension, status clock, `starting_soon` scheduler, retention purge, outbox relay (`shared/outbox.TableRelay`) |
| `http` | handlers, swagger section `Meetings` |

Ports (in `internal/shared`): `identityport` (users, M0 resource roles with `meeting_series`, tenant permissions,
audit), `tasksport` (new: create a task as the caller, read many tasks, check readability; adapter over the M2 service),
`peopleport` (phase 2 group rules). An import-boundary test forbids `meetings` from importing `projects` or `hr`
packages directly.

## 2. Data model (schema `meetings`; UUID ids; every row has `tenant_id`, `created_at`, `updated_at`)

| Table | Key columns | Constraints and indexes |
|---|---|---|
| `series` | id, title, description_md, organizer_id, tz, dtstart_local, duration_min, all_day, rrule (null = single), count, until_at, window_until, visibility, confidential, location, video_url, default_project_id, recording_allowed, ical_uid, sequence, status, version, deleted_at | unique (tenant_id, ical_uid); btree (tenant_id, window_until) where rrule is not null |
| `occurrences` | id, series_id, recurrence_id, start_at, end_at, is_exception, title_override, location_override, status, status_manual, version | unique (series_id, recurrence_id); btree (tenant_id, start_at); gist (tenant_id, tstzrange(start_at, end_at)) |
| `participants` | series_id, user_id, kind, active, added_by | unique (series_id, user_id); btree (tenant_id, user_id) |
| `responses` | series_id, occurrence_id (null = series), user_id, rsvp, comment | unique (series_id, occurrence_id, user_id) nulls not distinct |
| `agenda_items` | id, occurrence_id, rank COLLATE "C", title, body_md, owner_id, timebox_min, kind, state | unique (occurrence_id, rank) |
| `private_points` | id, occurrence_id, author_id, body_md | btree (occurrence_id, author_id) |
| `note_versions` | id, occurrence_id, section (agenda item id or nil = general), version, body_md, author_id, source | unique (occurrence_id, section, version) nulls not distinct; append-only trigger |
| `minutes` | id, occurrence_id, version, body_md, sha256, published_by, reason | unique (occurrence_id, version); append-only trigger |
| `decisions` | id, occurrence_id, agenda_item_id, text, status, decided_by, decided_at, recorded_by, version | |
| `action_links` | id, occurrence_id, agenda_item_id, task_id, created_by, snapshot jsonb, stale_at | unique (occurrence_id, task_id) |
| `attendance` | occurrence_id, user_id, state, marked_by | unique (occurrence_id, user_id) |
| `consents` | series_id, user_id, state, method, decided_at | unique (series_id, user_id) |
| `feed_tokens` | id, user_id, token_sha256, revoked_at, last_used_at | unique (token_sha256) |
| `change_log` | id, series_id, occurrence_id, actor_id, action, changes jsonb, reason | append-only |
| reliability | `idempotency_records` (shared), `meetings_events_outbox`, `meetings_inbox`, `dispatch_cursor` | as M2 |

## 3. Key mechanisms

**Materialiser.** RRULE parsing with `rrule-go`, after the whitelist check. Expansion runs in the series zone.
- On create or "all" edits: in the same transaction, up to `min(window, first 200)` occurrences; the job fills the rest
  of the window.
- Insert `ON CONFLICT (series_id, recurrence_id) DO NOTHING`, so it is idempotent.
- The daily job claims series with `window_until < now()+12 months` using `FOR UPDATE SKIP LOCKED`, in batches of 100.
- Exceptions are never regenerated. A rule change cancels future non-exception rows that have no data and inserts the
  new ones; rows that have data are kept and marked `orphaned`.
- Tenant cap: 200,000 materialised occurrences (configurable).

**Authorizer.** `Can(actor, series, perm)` is checked in this order:
1. `meetings:manage_all` allows everything (audited when the actor is not a participant).
2. The M0 resource role on the series allows what it grants.
3. For `meeting:read`: `read_all` plus `read_confidential` when needed (audited), then visibility for non-participants
   (`internal` gives metadata only, `busy_only` a busy block).
4. Otherwise not found.

Lists join a readable predicate built from `RolesFor` (series ids of the caller) plus the visibility rules, so pages are
never post-filtered. Private points use their own check (`author_id = actor`), with no override.

**Notes.** `PUT /notes/{section}` with `If-Match: <section version>` inserts version n+1 (`base` = n). A stale version
gets 409 `NOTES_STALE` with the current body. Publishing reads the latest version of every section in agenda order,
renders one markdown document, inserts `minutes` version k, sets the occurrence status and writes `minutes.published` to
the outbox, all in one transaction.

**Action items.**
1. `tasksport.Create(ctx, actor, project, input, idemKey)` runs in M2's own transaction first.
2. Then M3 inserts the link and the outbox row.
3. If that fails, a retry with the same key returns the same task (M2 idempotency), so no task is orphaned.

Reads use `tasksport.ReadMany(actor, ids)`; a task the caller cannot read shows as `restricted`.

**Feed.** A 32-byte random token, of which only the SHA-256 is stored. The feed renders the owner's readable series in
a window of −30 days to +12 months, and the RRULE plus EXDATE/RECURRENCE-ID for series without data overrides.
Confidential meetings render as `Busy`. It is rate-limited per token and per IP.

## 4. Performance

Indexes are as in section 2. The list for a 62-day window uses the occurrence range index and the caller's series ids
(at most a few thousand). The seed for the benchmarks: 1 tenant, 2,000 users, 20,000 series, 1 million occurrences. Gate
targets are in `specification.md` section 5 (item 12). Methods as `research/architecture-options.md` section 10.

## 5. Risks

| Risk | Mitigation |
|---|---|
| Expensive recurrence edits on long series | data rows are kept and not rewritten; the window is bounded; edits are batched in one transaction |
| An M0 change touches every module | it is additive (a new `applies_to` value); project behaviour is unchanged; the M0 and M2 suites run again |
| A cross-module call to M2 inside a request | M2 first, then the M3 transaction; idempotency key passed on; a timeout gives 503 and the client retries |
| Legal uncertainty on consent and retention | defaults are configurable; flagged in DMT-14 for review before production |
