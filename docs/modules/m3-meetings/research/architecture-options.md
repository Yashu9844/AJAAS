# M3 Meetings - research: architecture options and recommendation

**Status:** research input (2026-11-02) for the design gate. Nothing here is approved. It follows the patterns already
used by M2 (`docs/modules/m2-projects-tasks/decisions.md`): own Go package and Postgres schema, ports to other modules,
transactional outbox/inbox, `Idempotency-Key` middleware, `version` + `If-Match`, M0 decides every permission
(DPJ-49, `docs/modules/m0-identity-access/resource-roles.md`), AI only as seams (DPJ-37).

## 1. Fixed context (same for every option)

| Concern | Owner | M3 holds |
|---|---|---|
| Identity, tenant, permissions, resource roles | M0 | meeting membership records (who is invited, source), never role data |
| People facts (department, manager, working hours, leave) | M1 HR via a People port | group-rule input only |
| Tasks (action items) | M2 | `action_item_links(task_id)` + optional cached snapshot marked stale |
| Files (attachments, minutes PDF, recordings, transcripts) | M8 (until then shared `storage` port) | document ids |
| Delivery of reminders/invites | M10 | events only |
| AI (summaries, transcription, agents) | M9 | read-only context packs, draft-minutes write seam |
| Approvals (board minutes, restricted rooms) | M7 | approval reference |

Ports M3 needs: `identityport` (shared, existing; `Resources()` for resource roles), `ports.People` (HR group rules and
working hours, existing pattern), `ports.Names` (display names, DPJ-48), new `ports.Tasks` (create/link/read-batch task as
the caller - in-process adapter calling M2's task use case, HTTP later), `storage` (existing), `bus` (outbox relay).

## 2. Candidate architectures

### Option A - "Rule + bounded occurrences, relational notes" (lean core)

- **Recurrence:** `meeting_series` stores RRULE text, DTSTART in IANA zone, COUNT/UNTIL (capped). A worker materialises
  `meeting_occurrences` rows for a rolling horizon (e.g. 12 months or 200 occurrences, whichever first). Overrides are
  columns on the occurrence (`is_exception`, own start/end/title); cancellations are `status = cancelled` rows
  (EXDATE). "This and following" splits the series (old UNTIL, new series), as Google/Cal/sabre do.
- **Single meetings** are a series of one (one code path) - or `meeting_series.rrule IS NULL`.
- **Notes:** markdown per occurrence; each save appends an immutable `minutes_versions` row (body, hash, author,
  `base_version`); last-writer conflict detected by `base_version` (409, client merges). Publish freezes a version.
- **Video:** URL field (https, never fetched) in phase 1; signed Jitsi/LiveKit join links as an adapter in phase 2.
- **Calendar sync:** outbound only: per-user iCal feed (token as M2 DPJ-29) and `.ics` per meeting; no OAuth in phase 1.
- **Rooms:** `room_bookings(occurrence_id, room_id, during tstzrange)` with `EXCLUDE USING gist (tenant_id WITH =, room_id WITH =, during WITH &&) WHERE (status = 'active')`.
- **Deployment:** Go package `backend/internal/meetings/...`, schema `meetings`, in-process like M2.

### Option B - "iCalendar-native store, expand on read, CalDAV first"

- Store each series as an iCalendar blob (like sabre/dav) plus denormalised `first_at/last_at`; expand on read with
  rrule-go for each query window; overrides are VEVENTs with RECURRENCE-ID in the same blob.
- Notes, agenda, attendance keyed by `(series_id, recurrence_id)` (no occurrence table).
- CalDAV endpoint via `emersion/go-webdav` from the start, two-way (clients may PUT events); Google/Microsoft sync next.
- Room conflicts checked in application code after expansion (no DB constraint possible on non-materialised times).

### Option C - "Collaboration-heavy"

- Option A's occurrence model, plus: real-time co-editing of notes with Yjs/Tiptap through a separate Hocuspocus (Node)
  service persisting CRDT updates into `meetings.note_updates` and snapshotting markdown for versions; embedded LiveKit
  video (self-hosted SFU, egress recordings to storage) and two-way Google/Microsoft sync in the first release.

## 3. Decision matrix (1 = poor, 5 = best)

| Criterion | A lean core | B iCal-native | C collaboration-heavy | Notes |
|---|---|---|---|---|
| Security | **5** | 3 | 2 | B parses untrusted iCalendar on every write and accepts CalDAV PUTs; C adds a Node service, an SFU, OAuth tokens, recordings (consent, PII). |
| Complexity | **4** | 2 | 1 | B: per-query expansion, RECURRENCE-ID matching, no DB constraints. C: three runtimes. |
| Effort (to a usable phase 1) | **4** (~M2 phase 1-2 size) | 2 | 1 | |
| Scalability | 4 | 3 | 4 | A: reads are indexed range scans on occurrences; writes bounded by horizon. B: read cost grows with series age (rrule-go walks from DTSTART). |
| Offline / mobile | 3 | 4 | **5** | B gives native calendar apps via CalDAV; C gives offline CRDT notes. A: feed + REST; offline notes later. |
| AI-readiness | **5** | 3 | 4 | A has structured rows (decisions, actions, occurrences) that context packs can project; B hides structure in blobs. |
| Interoperability | 3 | **5** | 4 | A covers read-only feeds; two-way sync is a later adapter. |
| Fit with existing JAAS patterns | **5** | 2 | 3 | A reuses M2 mechanisms one-to-one (outbox, idempotency, ranks, versions, iCal feed, storage, M0 resource roles). |
| **Total (equal weights)** | **33** | 24 | 24 | |

## 4. Recommendation: Option A, with planned upgrade paths

1. Occurrence rows are needed anyway: notes, attendance, RSVP-per-occurrence, room locks and action items all hang off a
   concrete occurrence. A real row lets PostgreSQL enforce room conflicts with an exclusion constraint (no race), which
   neither Cal.diy (app-level, first occurrence only) nor sabre/dav (none) does.
2. Expand-on-read (B) costs O(occurrences since DTSTART) per series per query with rrule-go (see `internals.md`); a
   bounded horizon pays that once per series write.
3. Lowest attack surface: no inbound iCalendar parsing, no OAuth tokens, no extra runtime in phase 1; the only public
   route is the token feed, already proven in M2.
4. Upgrade paths kept open: CalDAV read-only endpoint over the same tables (go-webdav backend), Google/Microsoft
   adapters behind a `CalendarProvider` port (outbound HTTP needs a SECURITY.md exception like DPJ-41), CRDT editing as an
   optional notes engine that still writes `minutes_versions` snapshots, video provider port (`link|jitsi|livekit`).

## 5. Key mechanisms (Option A)

**Recurrence model.** `meeting_series(rrule, dtstart_local, tz, count, until, horizon_until)`; rule validated against a
whitelist (FREQ DAILY..YEARLY, INTERVAL<=99, BYDAY, BYMONTHDAY, BYMONTH, COUNT<=500, UNTIL<=now+2y). Materialiser: on
series create/edit (same transaction, up to the first 200 occurrences) and a daily job extending `horizon_until`
(claim with `FOR UPDATE SKIP LOCKED`). Natural key `UNIQUE(series_id, recurrence_id)` where `recurrence_id` = original
start instant (RFC 5545 RECURRENCE-ID semantics) makes materialisation idempotent. Editing "all" regenerates future
non-exception occurrences in one transaction; occurrences with notes/attendance are never deleted (marked
`cancelled`/`orphaned` and kept). DST: expand in the series' IANA zone, store `timestamptz`.

**Conflict detection.** Rooms: exclusion constraint (hard). People: soft warning computed by an indexed overlap query on
`occurrence_participants` joined to occurrences (`tstzrange(start_at,end_at) && $window`, GiST index) plus M1 leave
facts; returned in the create/update response, never blocking.

**Visibility (RLS-like).** One `Authorizer` (as M2) decides: tenant admin override (`meetings:manage_all`), resource role on
the series via M0 `PermissionsOn(tenant, user, "meeting", series_id)`, then meeting visibility (`private|internal|busy_only`)
for non-participants. Unreadable -> 404. Every list query joins a `readable` predicate built from the caller's series ids
(`RolesFor`) - no post-filtering of pages. Optional Postgres RLS with `app.tenant_id` as defence in depth is an open question.

**Permissions (M0 catalogue).** Tenant level: `meetings:create`, `meetings:read_all` (auditor; does not include private
notes), `meetings:manage_all`, `rooms:manage`, `meeting_types:manage`, `meetings:attendance_report`. Resource level
(requires M0 `applies_to` to accept `meeting`, today `tenant|project` - an M0 change, human gate):
`meeting:read`, `meeting:update`, `meeting:cancel`, `participant:manage`, `agenda:propose`, `agenda:manage`,
`minutes:edit`, `minutes:publish`, `decision:record`, `action:create`, `attendance:mark`. Templates
`meeting_viewer <- meeting_attendee <- meeting_note_taker <- meeting_organizer` (every role holds `meeting:read`).
Roles are assigned on the **series** (one row per participant per series), not per occurrence. Private talking points
are owner-only data, not a permission.

**Action items.** `POST /occurrences/{id}/action-items` -> `ports.Tasks.Create(ctx as caller, project_id, title,
assignees, due)` -> M2 checks `task:create`; on success M3 writes `action_item_links` + outbox in its own transaction. If M3
fails after M2 succeeded, retry with the same `Idempotency-Key` forwarded to M2 returns the same task (no orphan). Status is
read live in batch (`ports.Tasks.ReadMany`) with an M2 event-fed snapshot as fallback.

**Outbox / events / idempotency.** `meetings_events_outbox`, `meetings_inbox(consumer, event_id)`; global
`Idempotency-Key` middleware on writes; natural unique keys as above; events carry ids, versions and codes only (no
titles - confidential meetings).

## 6. Draft data model (schema `meetings`, every table has `tenant_id`, `created_at`, `updated_at`)

| Table | Key columns | Constraints / indexes |
|---|---|---|
| `meeting_types` | id, key, name, default_duration_min, default_agenda jsonb, default_visibility | unique (tenant_id, key) |
| `meeting_series` | id, type_id, title, description_md, organizer_id, tz, dtstart_local, duration_min, rrule (null = single), count, until_at, horizon_until, visibility, confidential, video_kind, video_url, ical_uid, sequence, status, version, deleted_at | unique (tenant_id, ical_uid) |
| `meeting_occurrences` | id, series_id, recurrence_id (timestamptz), start_at, end_at, is_exception, title_override, status (`scheduled|cancelled|in_progress|ended|minutes_published`), version | unique (series_id, recurrence_id); gist (tenant_id, tstzrange(start_at,end_at)); btree (tenant_id, start_at) |
| `series_participants` | series_id, user_id, kind (`required|optional`), source (`direct|group`), active | unique (series_id, user_id) - membership only, role in M0 |
| `occurrence_responses` | occurrence_id, user_id, rsvp (`needs_action|accepted|tentative|declined`), comment, responded_at | unique (occurrence_id, user_id); series-level RSVP = row with occurrence_id null |
| `participant_group_rules` | series_id, rule_kind (`department|team|reports_of`), ref_id | as M2 DPJ-22 |
| `agenda_items` | id, occurrence_id, rank (COLLATE "C"), title, body_md, owner_id, timebox_min, kind, state (`open|discussed|deferred`), carried_from_id | unique (occurrence_id, rank) |
| `private_talking_points` | id, occurrence_id, author_id, body_md | readable by author only |
| `minutes_versions` | id, occurrence_id, version, body_md, body_sha256, author_id, base_version, published, publish_reason, source (`human|agent:<label>`) | unique (occurrence_id, version); append-only trigger (as DPJ-21) |
| `decisions` | id, occurrence_id, agenda_item_id, text, status (`proposed|approved|superseded`), superseded_by, decided_at, recorded_by | |
| `action_item_links` | id, occurrence_id, agenda_item_id, task_id, created_by, snapshot jsonb (title, status, due, stale_at) | unique (occurrence_id, task_id) |
| `attendance` | occurrence_id, user_id, state (`present|late|absent|excused`), marked_by, checked_in_at | unique (occurrence_id, user_id) |
| `rooms` | id, name, site, capacity, equipment text[], bookable, requires_approval | unique (tenant_id, name) |
| `room_bookings` | id, occurrence_id, room_id, during tstzrange, status | `EXCLUDE USING gist (tenant_id WITH =, room_id WITH =, during WITH &&) WHERE (status='active')` (needs `btree_gist`) |
| `meeting_documents` | id, occurrence_id or series_id, document_ref, kind | |
| `feed_tokens` | id, user_id, token_sha256, revoked_at | as M2 |
| `change_log` | id, series_id, occurrence_id, actor_id, source, action, field_changes jsonb | append-only |
| reliability | `idempotency_records`, `meetings_events_outbox`, `meetings_inbox`, audit to M0 | as `model/db-model.md` |

## 7. Draft API (`/api/v1`, natural root names as DPJ-15)

| Group | Routes |
|---|---|
| Series & meetings | `GET/POST /meetings` (list = occurrences in window <= 62 days), `GET/PATCH/DELETE /meetings/{series_id}`, `POST /meetings/{id}/cancel`, `POST /meetings/{id}/split` (this-and-following) |
| Occurrences | `GET /occurrences/{id}`, `PATCH /occurrences/{id}` (exception), `POST /occurrences/{id}/cancel`, `POST /occurrences/{id}/status` |
| Participants | `GET/POST /meetings/{id}/participants`, `DELETE /meetings/{id}/participants/{user_id}`, `GET /meetings/{id}/roles`, `PUT /meetings/{id}/participants/{user_id}/role`, `POST /meetings/{id}/group-rules` |
| RSVP | `PUT /occurrences/{id}/response`, `PUT /meetings/{id}/response` (series) |
| Agenda | `GET/POST /occurrences/{id}/agenda-items`, `PATCH/DELETE /agenda-items/{id}`, `POST /agenda-items/{id}/move`, `POST /agenda-items/{id}/defer` |
| Minutes | `GET /occurrences/{id}/minutes`, `GET /occurrences/{id}/minutes/versions[/{v}]`, `PUT /occurrences/{id}/minutes` (If-Match = base version), `POST /occurrences/{id}/minutes/publish` |
| Decisions | `GET/POST /occurrences/{id}/decisions`, `PATCH /decisions/{id}`, `GET /decisions` (log) |
| Action items | `GET/POST /occurrences/{id}/action-items` (create task in M2 or link `task_id`), `DELETE /action-items/{id}` (unlink only) |
| Attendance | `GET/PUT /occurrences/{id}/attendance`, `POST /occurrences/{id}/check-in` |
| Rooms | `GET/POST /rooms`, `PATCH /rooms/{id}`, `GET /rooms/{id}/availability`, `POST/DELETE /occurrences/{id}/room` |
| Scheduling help | `POST /meetings/free-busy` (busy blocks only), `POST /meetings/suggest-times` |
| Feeds | `POST/DELETE /meeting-feed-tokens`, `GET /calendar/meetings/{token}.ics` (public, rate limited), `GET /occurrences/{id}.ics` |
| My views | `GET /me/meetings`, `GET /me/action-items` (from links), `GET /meetings/search` |
| AI seams | `GET /ai-hooks/meetings/tools`, `GET /context/occurrences/{id}`, `GET /meetings/documents` (incremental feed with tombstones), `POST /occurrences/{id}/minutes/drafts` (agent draft, never published) |

## 8. Events (`jaas.meetings.events`, ids/versions/codes only)

`meeting.scheduled`, `meeting.updated`, `meeting.rescheduled`, `meeting.cancelled`, `occurrence.cancelled`,
`occurrence.starting_soon` (scheduler, for M10 reminders), `occurrence.ended`, `participant.added|removed|changed`,
`rsvp.changed`, `agenda_item.added`, `minutes.version_saved`, `minutes.published`, `decision.recorded`,
`action_item.linked` (task_id), `attendance.marked`, `room.booked|released`. Consumed: identity deactivation (hook +
inbox), `task.updated|completed|deleted` (snapshot refresh), HR org changes (group-rule refresh).

## 9. Security threats (style of `m2-projects-tasks/security.md`)

| # | Threat | Mitigation |
|---|---|---|
| M1 | Cross-tenant access | `tenant_id` on every table/query, tenant from token only, foreign ids 404 |
| M2 | Reading a private/confidential meeting, or learning it exists | Authorizer + 404; `busy_only` shows start/end only; free/busy returns blocks without ids; confidential titles never in events, feeds, notifications |
| M3 | Private talking points / personal notes leaking (to organizer, admin, AI packs, exports) | Owner-only rows, excluded from `read_all`, context packs, search and exports; test per surface |
| M4 | Privilege escalation via meeting roles | Roles are M0 resource roles; assigning organizer needs `participant:manage` + organizer; last organizer guard; group rules cannot grant organizer |
| M5 | Object-level bypass (agenda, minutes version, action item, attendance ids) | Every child id resolved through its occurrence and series |
| M6 | Action item creates a task in a project the caller cannot write | M3 calls M2 *as the caller*; M2 checks `task:create`; linking needs `task:read` |
| M7 | Lost updates / double booking | `version` + If-Match; minutes `base_version`; room exclusion constraint; materialiser unique key |
| M8 | Recurrence DoS (huge COUNT, SECONDLY, BYSETPOS bombs) | Rule whitelist, COUNT<=500, UNTIL<=2y, horizon cap, window <= 62 days, expansion time budget |
| M9 | Malicious iCalendar input (later import/CalDAV) | Fuzz-tested parser, size limit 1 MiB, no URL fetch (`ParseCalendarFromUrl` banned) |
| M10 | Feed token abuse | 32-byte random, hash stored, revocable, 30/min/IP, owner must be active, private meetings show as busy unless owner participates |
| M11 | SSRF via video/doc URLs | https only, stored not fetched (SECURITY.md section 9); provider APIs only via the DPJ-41-style guarded sender |
| M12 | Stored XSS in notes | JSON only, markdown is data; client renders sanitised |
| M13 | Unconsented recording / transcription | Per-meeting `recording_consent` flag and per-participant consent before M9 capture; banner; retention policy |
| M14 | Personal data in events/audit | ids and codes only; attendance reports permission-gated |
| M15 | Departed users | Deactivation hook ends participation and resource roles; history kept |
| M16 | AI agent overreach / prompt injection via notes | User token only, read-only packs, drafts never auto-published, `X-JAAS-Agent` label never authorises |
| M17 | Video join link sharing | Jitsi/LiveKit JWT per user, short TTL (e.g. meeting end + 15 min), room name = random id |

## 10. Performance targets and how to benchmark later

Platform budgets (ARCHITECTURE section 3.9 / AGENTS.md): single resource p95 < 200 ms, list p95 < 500 ms, query p95 < 50 ms.

| Operation | Target (proposal) | Data set for the test |
|---|---|---|
| `GET /occurrences/{id}` incl. agenda, decisions, links | p95 < 150 ms | 1 occurrence, 30 agenda items, 20 links (M2 batch read in-process) |
| `GET /me/meetings` 7-day window | p95 < 200 ms, DB < 30 ms | user in 200 series, tenant with 1 M occurrences |
| `GET /meetings` 62-day window, tenant `read_all` | p95 < 500 ms | 50 k occurrences in window, page 100 |
| Create weekly series (materialise 200) | p95 < 300 ms | 20 participants, room booking per occurrence |
| Room conflict check (exclusion insert) | p95 < 5 ms per row | 500 rooms x 2 years of bookings (~1 M rows) |
| People soft-conflict query | p95 < 40 ms | 20 invitees x 200 series each |
| Expansion of one rule for 200 occurrences (rrule-go) | < 2 ms CPU (to be measured) | worst allowed rule |
| Minutes save (new version, 50 KB) | p95 < 120 ms | 500 versions per occurrence |
| iCal feed (90 days) | p95 < 300 ms | 300 events |

**How to benchmark:** (1) Go `testing.B` microbenchmarks for the expander (rule shapes x series age x window) and the
materialiser - run with `-benchmem`, record ns/op; (2) a seed tool generating tenant-scale data (1 M occurrences,
1 M room bookings) in the `meetings` schema and `EXPLAIN (ANALYZE, BUFFERS)` for the hot queries, saved in `testing.md`;
(3) HTTP load with k6 or vegeta against the console stack (steady 50 rps, burst 300 rps) collecting p50/p95/p99;
(4) concurrency test: 50 goroutines booking the same room/slot - exactly one wins, rest 409.

**External numbers (labelled external, not measured by us):**
- No published benchmark comparing materialised vs expand-on-read occurrences was found (search 2026-11-02).
- sabre/dav caps infinite rules at `MAX_DATE = 2038-01-01` and expands at write time only to compute the span (source read).
- forceCalendar issue #40 reports its engine 19x (daily), 907x (weekly BYDAY) and 40x (monthly) slower than the `rrule`
  JS library - evidence that BYDAY/DST handling dominates cost: https://github.com/forceCalendar/core/issues/40
- sabre/vobject ships `bin/rrulebench.php` that times parse vs expand separately (no published numbers):
  https://codesearch.dokuwiki.org/xref/plugin/davcal/vendor/sabre/vobject/bin/rrulebench.php
- Microsoft Graph: event subscriptions live at most 10,080 min (under 7 days), 1,440 min with resource data; minimum 45 min;
  calendar notification latency < 1 min average, 3 min max: https://learn.microsoft.com/en-us/graph/api/resources/subscription
- Microsoft Graph `calendarView/delta` requires a start/end window; delta/skip tokens carry the query:
  https://learn.microsoft.com/en-us/graph/api/event-delta
- Google Calendar: incremental sync via `nextSyncToken`; HTTP 410 = token invalid, full resync; push channels have no
  body, must be renewed manually, "a small percentage of messages" may be dropped:
  https://developers.google.com/workspace/calendar/api/guides/sync , https://developers.google.com/workspace/calendar/api/guides/push
- PostgreSQL range exclusion constraint with `btree_gist` (room example): https://www.postgresql.org/docs/current/rangetypes.html

## 11. Sources

See the links inline above, `internals.md` (code read from clones) and `open-source.md`. Accessed 2026-11-02.
