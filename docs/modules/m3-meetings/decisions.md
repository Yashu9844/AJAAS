# M3 Meetings - decisions

Decisions (DMT) were proposed in `research/decisions-validation.md` and `research/architecture-options.md`. The owner
approved them on 2026-11-04 ("yes please continue and fix", in reply to the 12 recommendations). The evidence and
the rejected alternatives are in those research files.

## Architecture (DMT-01..08)

- **DMT-01 Own package, own schema.** M3 is the Go package `internal/meetings` with the Postgres schema `meetings`. It
  runs in-process like M2, with its own migrations, routes, outbox/inbox and console page. Reference projects (sabre/dav,
  Cal.diy, Fellow-style products) are studied, never copied or run.
- **DMT-02 Rule plus bounded occurrences.** A series stores a whitelisted RFC 5545 RRULE in its IANA time zone.
  Occurrences are materialised rows with the natural key `(series_id, recurrence_id)`. An exception or cancellation is
  an occurrence row. Agenda, notes, attendance, decisions and action items belong to an occurrence.
- **DMT-03 Open-ended series allowed** (answer 3). A series with no COUNT/UNTIL is materialised over a rolling 12-month
  window, which an idempotent daily job extends. Explicit COUNT ≤ 500 and UNTIL ≤ 5 years. Allowed frequencies are
  DAILY, WEEKLY, MONTHLY and YEARLY; INTERVAL ≤ 99. BYSETPOS and sub-daily frequencies are refused. Editing an occurrence
  beyond the window creates its row on demand. Each tenant has a cap on materialised rows.
- **DMT-04 Versioned notes per agenda item, no CRDT in phases 1-2** (answer 5). Each agenda item, plus one general
  section, has its own notes with append-only versions and `If-Match`; a stale version gets 409 with the current text.
  Publishing assembles every section into an immutable published minutes version. Upgrade path: a pure-Go Yjs server
  (ygo/yserve) after a maturity check.
- **DMT-05 Action items are M2 tasks** (answer 4). M3 creates or links a task through a `Tasks` port **as the caller**
  and passes the idempotency key on. It stores only the link and a snapshot marked stale. A series has a default
  project. With no usable project the API answers 422 `NO_TASK_PROJECT`. A personal M2 inbox project is a later M2
  decision.
- **DMT-06 Rooms in phase 2, owned by M3** (answers 2 and 7). Phase 2 adds `bookable_resources` and `resource_bookings`
  with `EXCLUDE USING gist` (needs `btree_gist`). M4 later owns the physical asset, referenced by an optional `asset_id`.
  Phase 1 has a free-text location only.
- **DMT-07 Private talking points in phase 1** (answer 2). These are owner-only rows on an occurrence. No permission
  reaches them: not admin, not `read_all`, not context packs, search, exports or events. The author can promote a point
  to a shared agenda item.
- **DMT-08 No Postgres RLS in M3 alone** (answer 12). M3 uses the same application authorizer as M2: `tenant_id` on
  every query and cross-tenant isolation tests. RLS, if wanted, is decided once for the whole platform.

## Permissions (DMT-09..11)

- **DMT-09 Meeting-series resource roles in M0** (answer 1, extends DPJ-49). M0 accepts the resource type
  `meeting_series` (`applies_to`, `resource_role_assignments.resource_type`, `checkResourceType` becomes an allowlist).
  - Role templates: `meeting_viewer <- meeting_attendee <- meeting_note_taker <- meeting_organizer`; each one includes
    the one before. Required minimum: `meeting:read`.
  - Permissions on a series: `meeting:read`, `meeting:update`, `meeting:cancel`, `participant:manage`, `agenda:propose`,
    `agenda:manage`, `minutes:edit`, `minutes:publish`, `decision:record`, `action:create`, `attendance:mark`.
  - There is one assignment per participant per series; occurrences inherit it. The last organizer cannot be removed.
    Group rules never grant organizer.
- **DMT-10 Tenant permissions** in the M0 catalogue: `meetings:create`, `meetings:read_all`,
  `meetings:read_confidential`, `meetings:manage_all`, `meeting_types:manage`, `meetings:attendance_report`. Phase 2
  adds `rooms:manage`.
- **DMT-11 Auditor scope** (answer 6). `meetings:read_all` gives metadata, participants, published minutes and
  decisions of every meeting, but never draft notes or private talking points. Confidential meetings also need
  `meetings:read_confidential`. Each read through these permissions of a meeting the reader is not invited to writes an
  M0 audit entry. eDiscovery and legal hold belong to M8.

## Visibility, privacy, retention (DMT-12..15)

- **DMT-12 Visibility.**
  - `private`: participants only.
  - `internal`: every employee sees title and time, not notes.
  - `busy_only`: others see a busy block.
  - `confidential` flag: title hidden in feeds, notifications and events; events carry ids only in any case.
  - A meeting the caller cannot read answers 404 on every route (no existence oracle).
- **DMT-13 Recording consent seam** (answer 10). M3 stores `recording_allowed` per series and `consent` rows per
  participant (`granted|declined`, time, method). M9 must check them before any capture. Anyone declining means no
  recording, and that person keeps a view-only experience. M3 itself never records.
- **DMT-14 Retention defaults**, configurable per tenant: drafts purged 30 days after publish; published minutes and
  decisions kept; transcripts (M9) 90 days; soft-deleted series purged after 30 days. **The legal basis (consent or
  legitimate interest) and these defaults need legal review before production.**
- **DMT-15 Departed users.** The deactivation hook (as DPJ-23) removes the user from future occurrences, revokes their M0
  series roles and revokes their feed tokens. History is kept and shown as "(inactive)". A series without an organizer
  is flagged for an admin.

## Integration (DMT-16..20)

- **DMT-16 Calendar interop, phase 1 = pull only** (answers 8 and 11). This means a personal iCal feed with a hashed,
  revocable token (as DPJ-29) and `.ics` per occurrence or series. Libraries: `teambition/rrule-go` and
  `arran4/golang-ical`; the URL-fetching helpers are banned. Google/Microsoft two-way sync, CalDAV and
  external guests (iTIP, M10 e-mail) are phase 3+, each behind a new decision. Outbound provider calls then use a fixed
  host allowlist and encrypted OAuth tokens.
- **DMT-17 Video = link only** (answer 9). https, validated, stored, never fetched. An optional signed Jitsi JWT link is
  a later option; JAAS never operates a media server.
- **DMT-18 Events only, no delivery.** M3 publishes `jaas.meetings.events` (ids, versions and codes only), including
  `occurrence.starting_soon` from a scheduler. M10 delivers reminders and invitations once it exists.
- **DMT-19 People conflicts warn, never block.** An overlap query on the invitees' occurrences returns warnings in the
  create/update response. Only rooms (phase 2) block.
- **DMT-20 AI = seams only** (as DPJ-37/39/40): a read-only, permission-filtered context pack per occurrence, plus
  agent-labelled draft notes that only a human publishes. Phase 2.
