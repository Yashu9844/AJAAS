# M3 Meetings - Specification (phase 1, approved 2026-11-04)

Phase 1 scope follows `decisions.md` (DMT-01..20). Feature ids are from `research/features.md`; journeys J1-J12 are in
`research/user-journeys.md`. The universal rules in `docs/ARCHITECTURE.md` section 3 apply in full: tenancy,
idempotency, outbox/inbox, envelopes, security and latency.

**Purpose:** plan meetings and capture agenda, notes, decisions, attendance and action items. Tasks stay owned by M2.

## 1. Phase plan

| Phase | Scope (feature ids) |
|---|---|
| 1 | SC-1/2/3/6, RC-1..5/7 (open-ended per DMT-03), PA-1/3/4, AG-1/2/5, MN-1..4/8, DE-1, AI-1..3, AT-1, CS-1/2, VL-1, RM-1/3, PV-1/2/3/5, RT-1, SE-1, recording-consent seam (DMT-13) |
| 2 | PA-2 group invites, SC-4/5/7 free/busy, find-a-time and templates, RR-1/2 rooms, RC-6 + AG-3/4 carry-over and agenda templates, DE-2..4, AI-4..6, AT-2/4, RM-2, DC-1, AS-1, SE-2, RP-1..4, RT-2, MN-7 |
| 3+ | CS-3..6, PA-6/7, VL-2..4, MN-5/6, RR-3..5, AS-2..4, SC-8/9, AT-3, RP-5, RT-3, DC-2 |

## 2. Rules

| Id | Rule |
|---|---|
| MR-01 | Instants are `timestamptz`. Each series stores its IANA zone and the local start, and expansion runs in that zone (DST keeps the wall-clock time). |
| MR-02 | RRULE whitelist and bounds as DMT-03. Single meetings are series with no rule. |
| MR-03 | Every series and occurrence carries a `version`. Writes need `If-Match`; a stale version returns 409. Creates take `Idempotency-Key`. |
| MR-04 | "This occurrence" writes an exception row. "This and following" ends the old series (UNTIL) and creates a new one with copied participants, roles and settings. "All" regenerates future occurrences that are not exceptions. Occurrences with notes, attendance, decisions or actions are never deleted. |
| MR-05 | Status `scheduled -> in_progress -> ended -> minutes_published`, or `cancelled`. `in_progress` and `ended` follow the clock unless set by hand. |
| MR-06 | Participants are series members (`required|optional`); their role is an M0 resource role on the series (DMT-09). RSVP is per series and can be overridden per occurrence. |
| MR-07 | Agenda items have fractional ranks (as DPJ-09). Attendees with `agenda:propose` add items before the occurrence ends; `agenda:manage` reorders or removes them. |
| MR-08 | Notes are per agenda item plus one general section, with append-only versions (DMT-04). Publishing freezes a minutes version. A correction after publishing needs a reason and creates a new published version. |
| MR-09 | Private talking points are readable and writable only by their author (DMT-07). |
| MR-10 | Action items create or link M2 tasks as the caller (DMT-05). M3 never stores task truth: a snapshot is a display fallback, marked stale. |
| MR-11 | Visibility and existence rules as DMT-12. Auditor scope as DMT-11. |
| MR-12 | Conferencing URL: https only, no credentials, ≤ 2048 characters, never fetched. |
| MR-13 | List windows ≤ 62 days. Pages are cursor-based, with at most 200 rows per page. |

## 3. API (phase 1, `/api/v1`)

| Group | Routes |
|---|---|
| Series | `GET/POST /meetings` (list = occurrences in a window), `GET/PATCH/DELETE /meetings/{id}`, `POST /meetings/{id}/cancel`, `POST /meetings/{id}/split` |
| Occurrences | `GET/PATCH /occurrences/{id}`, `POST /occurrences/{id}/cancel`, `POST /occurrences/{id}/status`, `GET /occurrences/{id}.ics` |
| Participants and roles | `GET/POST /meetings/{id}/participants`, `PATCH/DELETE /meetings/{id}/participants/{user_id}` (body `{role | role_id, kind}`), `GET /meetings/{id}/roles` |
| RSVP | `PUT /meetings/{id}/response`, `PUT /occurrences/{id}/response` |
| Agenda | `GET/POST /occurrences/{id}/agenda-items`, `PATCH/DELETE /agenda-items/{id}`, `POST /agenda-items/{id}/move` |
| Notes and minutes | `GET /occurrences/{id}/notes`, `PUT /occurrences/{id}/notes/{section}` (section = agenda item id or `general`, If-Match), `GET /occurrences/{id}/notes/{section}/versions`, `POST /occurrences/{id}/minutes/publish`, `POST /occurrences/{id}/minutes/corrections`, `GET /occurrences/{id}/minutes[/versions[/{v}]]` |
| Private points | `GET/POST /occurrences/{id}/private-points`, `PATCH/DELETE /private-points/{id}`, `POST /private-points/{id}/promote` |
| Decisions | `GET/POST /occurrences/{id}/decisions`, `PATCH /decisions/{id}` |
| Action items | `GET/POST /occurrences/{id}/action-items` (`{project_id?, title, assignee_ids, due}` or `{task_id}`), `DELETE /action-items/{id}` (unlink only) |
| Attendance | `GET/PUT /occurrences/{id}/attendance` |
| Consent | `PUT /meetings/{id}/consent` (own), `GET /meetings/{id}/consent` (organizer) |
| Feeds | `POST /meeting-feed-tokens`, `DELETE /meeting-feed-tokens/{id}`, `GET /calendar/meetings/{token}.ics` (public, rate-limited) |
| My views and search | `GET /me/meetings`, `GET /meetings/search` |

## 4. Events (`jaas.meetings.events`; ids, versions and codes only)

`meeting.scheduled|updated|rescheduled|cancelled`, `occurrence.cancelled|starting_soon|ended`,
`participant.added|removed|changed`, `rsvp.changed`, `minutes.published`, `decision.recorded`, `action_item.linked`,
`attendance.marked`.

Consumed: identity deactivation (hook plus inbox) and `task.completed|deleted` (snapshot refresh; the subscription runs
in-process until a broker consumer exists).

## 5. Acceptance tests

1. Golden recurrence and DST tests: EU and US transitions; BYDAY `2MO`, `-1FR`, the 31st of the month; a COUNT, UNTIL
   or open-ended series; extending the window is idempotent.
2. Retrying a create with the same key returns the same series; 409 on a stale version; parallel saves of notes in
   different sections do not conflict.
3. Exception, split and cancel keep the data of past occurrences (MR-04).
4. A test table per role: viewer, attendee, note-taker, organizer, admin (`manage_all`), auditor (`read_all`,
   `read_confidential`), uninvited, cross-tenant.
5. Every route answers 404 for a meeting the caller cannot read; `internal` and `busy_only` show only what is allowed.
6. Private talking points are invisible to every other caller and surface (J7).
7. Action item consistency with M2: create (idempotent), link, restricted task, completed or deleted task.
8. Minutes are append-only (a database trigger refuses updates) and published versions are immutable.
9. Event replay is deduplicated by the inbox; events contain no titles.
10. Feed: the token is hashed, rotation takes effect at once, the content is filtered by permission, confidential
    meetings show as busy.
11. Deactivation ends participation, roles and feed tokens; history is kept.
12. Performance: an occurrence list over 62 days for a user with 300 series has p95 < 200 ms; a write has p95 < 200 ms.

## 6. Extension points (not built in phase 1)

Bookable resources and bookings (DMT-06), meeting types and templates, group rules (as DPJ-11/22), context pack, draft
notes by an agent, M8 document references, provider sync behind a `CalendarProvider` port, signed video links behind a
`VideoProvider` port. These tables are reserved by name; none is created early (as DPJ-12).
