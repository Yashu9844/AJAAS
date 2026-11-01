# M3 Meetings - research: feature list and user journeys

**Status:** research input (2026-11-02), not a specification. Priorities are proposals for the owner.
**Priority key:** MUST = phase 1 (usable module), SHOULD = phase 2, LATER = phase 3+ or needs another module first.
**Ownership rule (from `specification.md`):** M3 owns meetings, participants, agenda, minutes, decisions, attendance and
*references* to tasks; M2 owns tasks, M0 owns every permission decision, M8 owns files, M10 owns delivery, M9 owns AI.

## 1. Feature list

### 1.1 Scheduling

| Id | Feature | Pri | Notes |
|---|---|---|---|
| SC-1 | Create / edit / cancel a meeting: title, description (markdown), start, end, IANA time zone, location text, all-day flag | MUST | Store instants as `timestamptz` plus the IANA zone the organizer chose (needed for DST-correct recurrence). |
| SC-2 | Optimistic concurrency (`version`, `If-Match`) and `Idempotency-Key` on create | MUST | Same as M2 (DPJ-04, DPJ-33). |
| SC-3 | Meeting status `draft -> scheduled -> in_progress -> ended -> minutes_published`, `cancelled` | MUST | `in_progress`/`ended` derived from time unless set manually. |
| SC-4 | Free/busy view of invitees before booking (internal only: M3 meetings + M1 leave/holiday facts) | SHOULD | Only busy blocks, never titles, unless the viewer can read the meeting. |
| SC-5 | "Find a time" suggestion (first N common free slots in working hours) | SHOULD | Working hours/time zone per person come from M1/M12 settings. |
| SC-6 | Reschedule with reason; history kept | MUST | Change log entry + `meeting.rescheduled` event. |
| SC-7 | Meeting types/templates (1:1, standup, board, review, interview) with default agenda, duration, roles | SHOULD | Copy of Fellow/Hypercontext templates. |
| SC-8 | Public booking pages (Calendly-style) | LATER | External invitees = new public surface; needs a security review like DPJ-47. |
| SC-9 | Buffers, minimum notice, max meetings per day | LATER | |

### 1.2 Recurrence

| Id | Feature | Pri | Notes |
|---|---|---|---|
| RC-1 | RFC 5545 RRULE subset: DAILY/WEEKLY/MONTHLY/YEARLY, INTERVAL, BYDAY (incl. `2MO`, `-1FR`), BYMONTHDAY, COUNT or UNTIL | MUST | Reject SECONDLY/MINUTELY/HOURLY and BYSETPOS in phase 1 (bounded cost). |
| RC-2 | Hard bounds: COUNT <= 500 or UNTIL <= 2 years ahead; no infinite series stored without a horizon | MUST | Architecture rule "recurrence expansion bounded". |
| RC-3 | Edit one occurrence / this and following / all (RECURRENCE-ID overrides, series split) | MUST | Split creates a new series with UNTIL on the old one, as Google and sabre/dav do. |
| RC-4 | Cancel one occurrence (EXDATE) | MUST | |
| RC-5 | Per-occurrence agenda, minutes, attendance | MUST | Notes always belong to an occurrence, not to the series. |
| RC-6 | Carry-over of open agenda items and action items to the next occurrence | SHOULD | Fellow/Hypercontext core feature. |
| RC-7 | DST and time-zone change correctness (wall-clock stays 09:00 in the organizer zone) | MUST | Golden tests for EU and US transitions. |

### 1.3 Participants and RSVP

| Id | Feature | Pri | Notes |
|---|---|---|---|
| PA-1 | Invite users (M0/M1 ids), required / optional | MUST | |
| PA-2 | Invite groups resolved through HR (department, team, manager's reports) | SHOULD | Same mechanism as M2 DPJ-11/22 (materialised). |
| PA-3 | RSVP: accepted / declined / tentative / needs-action, with optional comment; per occurrence or series | MUST | |
| PA-4 | Meeting roles: organizer, co-organizer, note-taker, attendee, viewer | MUST | M0 resource-scoped roles on `meeting` (see architecture-options). |
| PA-5 | Delegate / forward invitation (if allowed by organizer) | SHOULD | |
| PA-6 | External guests by e-mail (no account) | LATER | Needs M10 e-mail + iTIP (RFC 5546) and a guest identity model. |
| PA-7 | Propose new time | LATER | |

### 1.4 Rooms and resources

| Id | Feature | Pri | Notes |
|---|---|---|---|
| RR-1 | Room catalogue (name, site, capacity, equipment, bookable flag) | SHOULD | Could live in M4 Assets later; M3 keeps a booking table either way. |
| RR-2 | Book a room with conflict prevention (DB exclusion constraint) | SHOULD | PostgreSQL `EXCLUDE USING GIST (room WITH =, during WITH &&)`. |
| RR-3 | Equipment (projector, car, phone booth) as bookable resources | LATER | |
| RR-4 | Auto-release a room if nobody checks in within N minutes | LATER | |
| RR-5 | Room approval for restricted rooms (board room) via M7 Approvals | LATER | |

### 1.5 Agenda

| Id | Feature | Pri | Notes |
|---|---|---|---|
| AG-1 | Ordered agenda items: title, description, owner, time box, type (info/discussion/decision) | MUST | Fractional ranks as M2 (DPJ-09). |
| AG-2 | Attendees add topics before the meeting (if role allows) | MUST | |
| AG-3 | Agenda templates per meeting type | SHOULD | |
| AG-4 | Mark item discussed / deferred (deferred moves to next occurrence) | SHOULD | |
| AG-5 | Talking points private to the author (1:1 "private notes") | SHOULD | Visible only to author; never in published minutes or AI packs. |

### 1.6 Live notes and minutes

| Id | Feature | Pri | Notes |
|---|---|---|---|
| MN-1 | Shared notes per occurrence (markdown) edited by note-takers | MUST | |
| MN-2 | Version history: each save = immutable version; diff between versions | MUST | Architecture rule "minutes version history". |
| MN-3 | Publish minutes (frozen version), notify attendees through M10 | MUST | `minutes.published` event. |
| MN-4 | Correction after publish creates a new published version with reason | MUST | |
| MN-5 | Real-time co-editing (several cursors) | SHOULD | Option: CRDT (Yjs) vs section locks; see architecture-options. |
| MN-6 | Approval of minutes (board/committee) via M7 | LATER | |
| MN-7 | Export minutes to PDF/DOCX stored in M8 | SHOULD | |
| MN-8 | Personal private notes on a meeting | SHOULD | Owner-only rows. |

### 1.7 Decisions

| Id | Feature | Pri | Notes |
|---|---|---|---|
| DE-1 | Record a decision: text, agenda item, decided-by, date, status (proposed/approved/superseded) | MUST | First-class rows, not just text in minutes, so they are searchable and reportable. |
| DE-2 | Decision log across meetings (filter by team/project/date) | SHOULD | |
| DE-3 | Supersede a decision (link old -> new) | SHOULD | |
| DE-4 | Link a decision to an M2 project or M8 document | SHOULD | Reference ids only. |

### 1.8 Action items (tasks stay in M2)

| Id | Feature | Pri | Notes |
|---|---|---|---|
| AI-1 | Create an action item from a meeting: M3 calls the M2 task API as the user; stores `(meeting, occurrence, agenda_item, task_id)` | MUST | No duplicate task truth; title/status shown live from M2 (or cached snapshot marked stale). |
| AI-2 | Link an existing M2 task | MUST | Caller must be able to read the task. |
| AI-3 | Action item list on the meeting with live status | MUST | Batch read from M2; tasks the viewer cannot read show as "restricted". |
| AI-4 | Review open items at the start of the next occurrence | SHOULD | |
| AI-5 | Fallback when the user is not a member of any project: personal "meeting actions" project in M2 or a default project per meeting type | SHOULD | Open question for owner. |
| AI-6 | Consume `task.completed` / `task.deleted` events to refresh the cached snapshot | SHOULD | Inbox-deduplicated. |

### 1.9 Attendance

| Id | Feature | Pri | Notes |
|---|---|---|---|
| AT-1 | Mark attendance per occurrence: present / late / absent / excused | MUST | Set by organizer or note-taker. |
| AT-2 | Self check-in (button, QR in room) | SHOULD | |
| AT-3 | Import attendance from video provider (join/leave times) | LATER | Needs provider integration. |
| AT-4 | Attendance report per person/team (for committees, training) | SHOULD | Personal data: visible to manager/HR only by permission. |

### 1.10 Calendar sync

| Id | Feature | Pri | Notes |
|---|---|---|---|
| CS-1 | Read-only personal iCal feed (token URL), like M2 DPJ-29 | MUST | Cheapest interoperability with Google/Outlook/Apple. |
| CS-2 | `.ics` attachment / download per meeting (METHOD:REQUEST/CANCEL) | MUST | Built with an iCal library; e-mail sending via M10. |
| CS-3 | Two-way sync with Google Calendar (OAuth, events.watch + syncToken) | LATER | Outbound HTTP to Google = exception to SECURITY.md section 9; needs a decision like DPJ-41. |
| CS-4 | Two-way sync with Microsoft 365 (Graph delta + subscriptions) | LATER | Same. |
| CS-5 | CalDAV server endpoint (Thunderbird, Apple Calendar, DAVx5) | LATER | Read-only first. |
| CS-6 | Busy-time import from external calendars (free/busy only) | LATER | |

### 1.11 Video link integration

| Id | Feature | Pri | Notes |
|---|---|---|---|
| VL-1 | Free-text conferencing URL (https only, never fetched) | MUST | SECURITY.md section 9. |
| VL-2 | Generated Jitsi/LiveKit room link with signed join token per participant | SHOULD | Self-hosted Jitsi (JWT) or LiveKit (JWT access token). |
| VL-3 | Create Zoom/Teams/Meet meetings through provider APIs | LATER | Outbound integration. |
| VL-4 | Recording reference (file in M8) | LATER | |

### 1.12 Reminders (M10)

| Id | Feature | Pri | Notes |
|---|---|---|---|
| RM-1 | M3 publishes `meeting.scheduled/updated/cancelled`, `occurrence.starting_soon` | MUST | M3 never sends e-mail itself. |
| RM-2 | Reminder offsets per meeting (default 10 min) and per user preference (in M10) | SHOULD | M3 emits a due-reminder event from a scheduler; M10 decides channel. |
| RM-3 | Minutes published / action item assigned notifications | MUST | Events only. |

### 1.13 Documents (M8)

| Id | Feature | Pri | Notes |
|---|---|---|---|
| DC-1 | Attach document references (M8 ids) to a meeting or agenda item | SHOULD | Until M8 exists: shared `storage` port as M2 DPJ-07/30. |
| DC-2 | Published minutes archived as an M8 document version | LATER | |

### 1.14 AI seams (M9 owns the AI)

| Id | Feature | Pri | Notes |
|---|---|---|---|
| AS-1 | Read-only context pack per occurrence (agenda, published minutes, decisions, action refs), permission-filtered and capped | SHOULD | Same pattern as M2 `aihooks` (DPJ-37/39). |
| AS-2 | Transcript ingestion endpoint: M9 (or a capture worker) posts transcript segments as data; M3 stores a transcript reference (M8) | LATER | Consent flag required before any recording. |
| AS-3 | "Draft minutes / summary / action items" suggestions written as a *draft* version, accepted by a human | LATER | Never auto-publish; agent label as DPJ-40. |
| AS-4 | Tool catalogue entries (create meeting, add agenda item, propose action) for MCP | LATER | Write tools flagged `confirm`. |

### 1.15 Privacy and visibility

| Id | Feature | Pri | Notes |
|---|---|---|---|
| PV-1 | Visibility: `private` (participants only), `internal` (tenant can see it exists + title), `busy_only` (others see a busy block) | MUST | "meeting visibility distinct from participant visibility". |
| PV-2 | Confidential flag: title hidden in feeds/notifications/events | MUST | Events carry ids only anyway. |
| PV-3 | Non-readable meeting answers 404 (no existence oracle) | MUST | As M2 S2. |
| PV-4 | Recording/transcription consent per participant | LATER | Legal requirement in several jurisdictions. |
| PV-5 | Departed users lose access; history kept | MUST | Deactivation hook like M2 DPJ-23. |

### 1.16 Retention

| Id | Feature | Pri | Notes |
|---|---|---|---|
| RT-1 | Soft delete + purge after N days | MUST | |
| RT-2 | Tenant retention policy for notes/transcripts (e.g. transcripts 90 days, minutes 7 years) | SHOULD | Policies may move to M8/M12. |
| RT-3 | Legal hold blocks purge | LATER | M8 owns legal hold. |

### 1.17 Search

| Id | Feature | Pri | Notes |
|---|---|---|---|
| SE-1 | Search meetings by title, participant, date range | MUST | Postgres full text + trigram, permission-filtered. |
| SE-2 | Search inside published minutes and decisions | SHOULD | |
| SE-3 | Cross-module search via M6/M11 or M9 retrieval | LATER | Documents feed with tombstones as M2. |

### 1.18 Reports

| Id | Feature | Pri | Notes |
|---|---|---|---|
| RP-1 | My meetings this week, hours in meetings | SHOULD | |
| RP-2 | Meeting load per team (hours, count, recurring share) | SHOULD | Aggregates only; feed to M6. |
| RP-3 | Action item follow-through (open/overdue from M2) | SHOULD | |
| RP-4 | Attendance report | SHOULD | Permission-gated. |
| RP-5 | Room utilisation | LATER | |

## 2. User journeys

| Actor | Journey (happy path) | Features |
|---|---|---|
| **Organizer** | Creates "Weekly ops review", Tue 09:00 Europe/Berlin, weekly, 26 occurrences, picks template "Review" (agenda prefilled), invites the Ops team (group rule), books room B2 (conflict check), adds Jitsi link, assigns a note-taker. Moves one occurrence because of a holiday; cancels another. After each meeting checks that minutes are published and action items assigned. | SC-1..3, RC-1..4, PA-1..4, RR-2, AG-3, VL-2 |
| **Attendee** | Gets invitation (M10), accepts series but declines one occurrence, adds an agenda topic, sees the meeting in Outlook through the iCal feed, joins via link, later reads the minutes and sees the action item assigned to them in "My work" (M2). | PA-3, AG-2, CS-1, MN-3, AI-1 |
| **Note-taker** | Opens the occurrence, marks attendance, writes notes per agenda item, records two decisions, creates three action items (tasks created in M2 project "Ops"), publishes minutes; later corrects a typo creating version 3 with reason. | AT-1, MN-1..4, DE-1, AI-1 |
| **Manager** | Runs 1:1s with reports: shared agenda plus private talking points; open action items carry over; sees the decision log of the team and meeting load report; never sees private notes of others. | RC-6, AG-5, DE-2, RP-2 |
| **Admin** (tenant) | Configures rooms, meeting types and retention policy; assigns `meetings:read_all` to auditors; reviews audit trail of a confidential meeting; enables the Google sync integration (later). Cannot read private meetings without an explicit permission (all access audited). | RR-1, SC-7, RT-2, PV-1, CS-3 |

## 3. Sources (accessed 2026-11)

- RFC 5545 iCalendar: https://www.rfc-editor.org/rfc/rfc5545 ; RFC 5546 iTIP: https://www.rfc-editor.org/rfc/rfc5546 ; RFC 4791 CalDAV: https://www.rfc-editor.org/rfc/rfc4791
- PostgreSQL range exclusion constraints (room example): https://www.postgresql.org/docs/current/rangetypes.html
- Feature patterns from products in `market-research.md` and OSS in `open-source.md`.
