# M3 Meetings - research: user journeys (proposed phase 1)

**Status:** research input (2026-11-02), not a specification. It assumes the recommendations in
`decisions-validation.md`: M0 meeting-series roles, open-ended series on a rolling 12-month window, per-agenda-item notes
with versioned saves, and rooms in phase 2. Feature ids refer to `features.md`. Each journey lists actors,
preconditions, steps, alternative and failure paths, and the checks that would become browser walks or console
scenarios (as `pm_walk*` and `ui_walk_all.mjs` are for M2).

**Personas:** Olivia (organizer, ops lead), Arun (attendee), Nora (note-taker), Mark (manager, has 1:1s with Arun), Ada
(tenant admin), Ravi (auditor with `meetings:read_all`), Eve (an employee with no invitation).

| Id | Journey | Actor | Phase |
|---|---|---|---|
| J1 | Create a recurring meeting | Organizer | 1 |
| J2 | Change one occurrence / this and following / cancel | Organizer | 1 |
| J3 | Respond and prepare | Attendee | 1 |
| J4 | Run the meeting: attendance, notes, decisions | Note-taker | 1 |
| J5 | Turn discussion into action items (M2 tasks) | Note-taker | 1 |
| J6 | Publish and correct minutes | Note-taker / organizer | 1 |
| J7 | 1:1 with private talking points | Manager + report | 1 |
| J8 | See my meetings in Outlook/Google (iCal feed) | Any user | 1 |
| J9 | Admin setup and auditor review | Admin, auditor | 1 |
| J10 | Access denied without an existence leak | Uninvited user | 1 |
| J11 | User leaves the company | System / admin | 1 |
| J12 | Search and "my week" | Any user | 1 |
| J13 | Book a room | Organizer | 2 |
| J14 | Carry over open items to the next occurrence | Organizer | 2 |

## J1 - Create a recurring meeting

**Preconditions:** Olivia has `meetings:create` (tenant permission in M0). The Ops team exists in HR.

1. Olivia opens **Meetings > New**. She enters the title "Weekly ops review" and the description (markdown); start Tue
   09:00, 45 min, zone Europe/Berlin (defaults to her profile zone); location "Room B2 / link".
2. Repeat: weekly on Tuesday, **no end date** (or after N times / until a date).
3. Visibility `internal`, not confidential. She pastes a conferencing link (https only).
4. She invites the "Ops team" group (materialised to users) and Arun as optional, and makes Nora note-taker.
5. Default project for action items: "Ops".
6. Agenda: three items with time boxes.
7. She saves. The API creates the series, materialises 12 months of occurrences, and gives every invitee a series role
   in M0 (`meeting_organizer` for Olivia, `meeting_note_taker` for Nora, `meeting_attendee` for the others). It publishes
   `meeting.scheduled`.

**Alternative and failure paths**
- A double click or network retry with the same `Idempotency-Key` returns the same series, not two.
- A disallowed rule (FREQ=HOURLY, BYSETPOS, COUNT > 500, UNTIL > 5 years) returns 422 with the field named.
- End before start, or an unknown zone, returns 422.
- A non-https conferencing link returns 422. The server never fetches it.
- Inviting a deactivated user returns 422 `PARTICIPANT_INACTIVE`.
- Without `meetings:create`: the button is hidden and the API returns 403.

**Checks:** 52 occurrences visible in the next 12 months. A DST week (end of March) still shows 09:00 Berlin. The UTC
instant moves by one hour.

## J2 - Change one occurrence, this and following, or cancel

1. A public holiday falls on Tue 5 May. Olivia opens that occurrence and moves it to Wed 6 May 09:00 with the reason
   "holiday". Only that row changes: an exception keyed by its original time. It publishes `meeting.rescheduled` and the
   change log gets an entry.
2. She cancels 26 May. The occurrence shows as cancelled and attendees are notified (event to M10).
3. From June the meeting moves to 10:00 ("this and following"). The old series ends on 2 June (UNTIL) and a new series
   starts. Participants, roles and default project are copied. Agendas and notes of past occurrences stay where they
   were.

**Failure paths:** a stale `If-Match` returns 409 and the UI reloads and shows what changed. Moving an occurrence that
already has published minutes returns 409 `OCCURRENCE_HAS_MINUTES`. A note-taker trying to reschedule gets 403, because
the role lacks `meeting:update`.

**Checks:** editing occurrence N does not touch N+1. After the split, the old series has no occurrences after the split
date and the new one has 12 months.

## J3 - Attendee responds and prepares

1. Arun sees the invitation in **My meetings** (and later through M10 notifications).
2. He accepts the series and declines 12 May with the comment "on leave".
3. He adds the agenda topic "Shift rota" to next week's occurrence (attendees may add topics; the organizer may reorder
   or remove them).
4. He opens the meeting link at start time.

**Failure paths:** adding a topic to a past or cancelled occurrence returns 409. A viewer-only participant cannot add
topics (403). Without the series role, Arun's RSVP to a series returns 404 (J10).

**Checks:** the organizer sees counts per answer (accepted / declined / tentative / no reply) per occurrence. The
declined occurrence shows Arun's comment only to people who can read the meeting.

## J4 - Run the meeting: attendance, notes, decisions

1. Nora opens today's occurrence. Status changes to `in_progress` at start time.
2. Attendance: she marks Arun absent (excused) and others present, one late.
3. She writes notes under each agenda item. Every save is a version. If Olivia edits the same item at the same time, the
   second save gets 409 and a merge view; different items never conflict.
4. She records the decision "Move to 3-shift rota from July" on agenda item 2. Status approved, decided by Olivia.

**Failure paths:** attendance on a future occurrence returns 409. An attendee editing notes gets 403 (the
`meeting_attendee` role has no `minutes:write`).

**Checks:** version history lists each save with author and time, and a diff between any two versions. The decision
appears in the decision log.

## J5 - Turn discussion into action items

1. Under agenda item 2, Nora adds the action "Draft new rota" for Arun, due 30 June.
2. M3 calls the M2 tasks port **as Nora**, in the series' default project "Ops". The task is created in M2. M3 stores
   only the link (meeting, occurrence, agenda item, task id) and a snapshot.
3. She links an existing M2 task "Hire 2 operators" (she must be able to read it).
4. The action list shows live status from M2. Arun sees the task in **My work** (M2).

**Failure paths:**
- Nora cannot create tasks in "Ops": the UI asks for another project. With no project at all, 422 `NO_TASK_PROJECT` and
  guidance (decision 4).
- A linked task she cannot read returns 404.
- When a viewer of the meeting cannot read a task, the action shows as "restricted" with no title.
- The task is completed or deleted in M2: the snapshot refreshes through the event.

**Checks:** M3 holds no task title or status truth (only the snapshot, marked stale). A retry of the create does not
make two tasks (idempotency key passed through).

## J6 - Publish and correct minutes

1. Nora publishes. The current version is frozen as version 1 (published). It publishes `minutes.published`, M10
   notifies the attendees, and the status becomes `minutes_published`.
2. Next day she fixes a typo: "Correct minutes" asks for a reason and creates published version 2. Version 1 stays
   visible in history.

**Failure paths:** publishing with no notes returns 422. Correcting without a reason returns 422. An attendee trying to
publish gets 403.

**Checks:** attendees see only published versions, not drafts. The diff between published versions 1 and 2 shows the
change and its reason.

## J7 - 1:1 with private talking points

**Preconditions:** Mark creates a weekly 1:1 with Arun, `private`, no end date.

1. Both add shared agenda points. Each also writes **private talking points**, visible only to their author.
2. During the meeting, Mark drags one private point into the shared agenda (it becomes a shared item; the private copy
   is removed).
3. Shared notes and action items work as in J4-J5.

**Failure paths:** no route returns another user's private rows. Not Ada with all permissions, not Ravi with
`read_all`, not the AI context pack. Any such request returns 404.

**Checks:** an isolation test with admin, auditor, the other participant and a cross-tenant user. Private rows never
appear in published minutes, search, exports or events.

## J8 - My meetings in Outlook or Google

1. Arun opens **Settings > Calendar feed** and creates a personal feed URL (token shown once).
2. He subscribes in Outlook. Meetings he can read appear with their RRULE and exceptions. Confidential meetings show
   "Busy" with no title.
3. On a single meeting he can download a `.ics` file.
4. He rotates the token; the old URL returns 404 at once.

**Checks:** the feed contains only meetings the token's owner can read now. A token from another tenant does not work.
Rate limit per token.

## J9 - Admin setup and auditor review

1. Ada opens the M0 role editor and sees meeting roles (applies to `meeting_series`). She clones `meeting_attendee` into
   "Guest attendee" without "add agenda topic" (allowed permissions only).
2. She grants Ravi `meetings:read_all` and sets retention: draft notes purged 30 days after publish.
3. Ravi opens a private board meeting he was not invited to. He sees metadata, participants, published minutes and
   decisions. He does not see drafts or private points. For a confidential meeting he also needs
   `meetings:read_confidential`. Each such read writes an audit entry.
4. Ada reviews the audit trail of that meeting: who read it, who changed what.

**Failure paths:** deleting a meeting role that is in use returns 409 (same as projects). A role whose permissions do
not match the resource type returns 422.

## J10 - Uninvited user (no existence leak)

Eve guesses a private meeting id. GET, the RSVP, agenda, notes and feed routes all return **404**, the same as for a
non-existent id (no hint that it exists). An `internal` meeting shows Eve the title and time but no notes. A `busy_only`
meeting shows a busy block.

## J11 - User leaves the company

HR deactivates Arun (an M0/M1 event). M3 removes him from future occurrences and revokes his series roles in M0, but
keeps his attendance and authorship in history. If he organised series, they are flagged so an admin assigns a new
organizer. His feed token stops working.

**Checks:** past minutes still show "Arun (inactive)". He cannot log in or use the feed. A series with no organizer
appears in the admin list.

## J12 - Search and "my week"

Any user searches by title, participant or date range. Only meetings they can read come back. Published minutes and
decision search is phase 2. **My week** lists the next 7 days of their occurrences and their RSVP state.

## J13 - Book a room (phase 2)

Olivia picks Room B2 for the series. An occurrence that clashes with an existing booking is refused by the database
constraint (409, with the conflicting slot). People clashes are only warnings. Admins manage the bookable resource list.

## J14 - Carry over open items (phase 2)

At the start of the next occurrence, agenda items not discussed and open action items (live from M2) are proposed for
carry-over. The organizer accepts them.

## Acceptance set to automate (from the journeys)

- Recurrence and DST golden tests: EU and US transitions, an open-ended series, the window extension job.
- Idempotent create; 409 on a stale version, and per agenda item (no conflict across items).
- Tests per role: viewer, attendee, note-taker, organizer, admin, auditor, cross-tenant.
- 404 with no existence oracle on every route (J10). Private-row isolation (J7).
- Action item link consistency with M2 (create, link, completed, deleted, restricted).
- Event replay (the inbox removes duplicates); feed content filtered by permission.
