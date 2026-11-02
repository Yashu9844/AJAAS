# M3 Meetings - research: validated answers to the 12 open questions

**Status:** research input (2026-11-02), not a decision. Each answer is a recommendation for the owner. Each one lists the
evidence (product behaviour, standards, existing JAAS decisions and code), the alternatives, and a confidence level.
Legal points are not legal advice and need counsel.

**Facts in this repository that limit the options:**
- M0 decides every permission (DPJ-49). `resource_access.go` accepts only the `project` resource type
  (`checkResourceType`), and its audit call hard-codes `"project"` as the audited resource.
- M2 (Projects & Tasks) checks access in the application only. No module uses Postgres row-level security (RLS).
- M4 Assets, M8 Documents and M10 Notifications are **NOT STARTED** (their READMEs say so).
- Outgoing HTTP is forbidden (SECURITY.md section 9). The only exception is DPJ-41, the SSRF-guarded webhook sender.

## Summary

| # | Question | Recommendation | Confidence |
|---|---|---|---|
| 1 | Meeting roles in M0? | **Yes.** Add the resource type `meeting_series` to M0 (resource roles), with four role templates | High |
| 2 | Phase-1 scope | MUST list, but **rooms move to phase 2** and **private talking points (AG-5) move up to phase 1** | Medium-high |
| 3 | Infinite recurrence | **Allow series with no end date**, materialised over a rolling 12-month window; COUNT ≤ 500, UNTIL ≤ 5 years | High |
| 4 | Action items without a project | **Require a project.** Each series has a default project; a personal M2 inbox project comes in phase 2 | Medium |
| 5 | Real-time co-editing | **No.** Versioned saves with `If-Match`/409, notes split per agenda item; upgrade path is a pure-Go Yjs server | High |
| 6 | Auditor `read_all` | Auditors see metadata, published minutes and decisions, and every read is audited. **Never** drafts or private notes | Medium-high |
| 7 | Rooms in M3 or M4 | **M3 owns bookable resources and bookings.** M4 later owns the physical asset; a room gets an optional `asset_id` | High |
| 8 | Outbound integrations | **Not before phase 3.** A new decision then, with a fixed allowlist of provider hosts and encrypted OAuth tokens | High |
| 9 | Self-hosted video | **Link only** in phases 1-2. Later, an optional signed Jitsi JWT link; JAAS never runs the video server | High |
| 10 | Consent and retention | Phase 1 stores only a `recording_allowed` flag and per-participant consent rows. Defaults: transcripts 90 days, minutes kept. **Needs legal review** | Medium |
| 11 | External guests | **Phase 3+** (needs M10 e-mail, a guest identity model and iTIP). Phase 1 offers an `.ics` download | High |
| 12 | Postgres RLS | **Not in M3 alone.** Keep application checks plus isolation tests; decide RLS once for the whole platform | Medium |

## 1. Roles on a meeting series in M0

**Recommendation:** extend `applies_to` and `resource_role_assignments.resource_type` with `meeting_series`. Add the role
templates `meeting_viewer <- meeting_attendee <- meeting_note_taker <- meeting_organizer` (each one includes the one
before). Required minimum: `meeting:read`.

**Evidence**
- DPJ-49 ("M0 decides everything") was approved for projects. If M3 worked out permissions itself, there would be two
  permission engines again, which is the split DPJ-49 removed.
- Calendars do treat permissions per event. Google Calendar has per-event guest flags (`guestsCanModify`,
  `guestsCanInviteOthers`, `guestsCanSeeOtherGuests`), and Teams meeting options set who may present or record. These
  are configurable per meeting, so they map onto a custom role, not onto a fixed participant kind.
- The M0 change is small and already shaped for it. `checkResourceType` becomes an allowlist, and the hard-coded
  `"project"` audit resource becomes `ref.Type`. That is a latent bug to fix in any case.

**Cost and risk:** there is one assignment row per participant per series. A 200-person all-hands series means 200 rows,
which is fine. Occurrences inherit the role from their series, so there are no rows per occurrence. Group invites
materialise the same way as M2 group rules (DPJ-11/22).

**Rejected alternative:** derive the role from the participant kind (organizer, required or optional) inside M3. It is
simpler, but it breaks DPJ-49, custom roles cannot be used, and the M0 role editor cannot show meeting permissions.

## 2. Phase-1 scope

**Recommendation:** use the MUST list in `features.md`, with two changes:
- **Rooms (RR-1/2) move to phase 2.** M4 is not started, and a room catalogue needs an admin UI. Nextcloud's experience
  supports this: its room management stayed CLI-only for years and is a separate backend app. Phase 1 keeps the free
  `location` text.
- **Private talking points (AG-5/MN-8) move to phase 1.** These are owner-only rows and cheap to build. The 1:1 journey
  (manager) does not work without them. Private notes are the core of Fellow's 1:1 product, and Fellow keeps them out of
  its AI search.

The final phase-1 list, as journeys, is in `user-journeys.md` (J1-J12).

## 3. Infinite recurrence

**Recommendation:** allow series with no end date. Materialise occurrences for a **rolling 12-month window**; a nightly
job extends it. Explicit `COUNT` stays ≤ 500 and `UNTIL` ≤ 5 years. Editing or cancelling an occurrence beyond the
window creates its row on demand (the natural key is `(series_id, recurrence_id)`). The iCal feed exports the RRULE
itself, so external calendars expand it on their side.

**Evidence**
- Google Calendar stores a recurring event as a parent with the RRULE. Instances are derived and carry
  `recurringEventId` and `originalStartTime`, and an exception is an instance row. Google also warns that editing many
  instances one by one creates many exceptions and slows access. This is the same rule-plus-exception-rows model as
  Option A, so the rolling window fits.
- Outlook and Google both offer "no end date" in their UI. A weekly 1:1 or standup "forever" is the common case, so
  forcing an end date would be a visible usability regression.
- The architecture rule says "recurrence expansion bounded". A fixed window bounds the cost of storage and queries, and
  it is a time limit, not a limit on what the user can create.

**Guardrails:** cap materialised rows per tenant (protection against abuse); only DAILY and coarser frequencies; DAILY
uses the same 12-month window (≈ 365 rows). The window job is idempotent and runs under a lock.

## 4. Action items when the user has no project

**Recommendation:**
- **Phase 1:** an action item always becomes an M2 task in a project that the caller can create tasks in.
  - The series has a **default project** (set by the organizer), and it is pre-selected.
  - If the caller cannot create tasks there, they pick another project.
  - If they have no project at all, the API answers 422 `NO_TASK_PROJECT` and the UI explains how to fix it.
- **Phase 2:** M2 gets a per-user personal "inbox" project (private, single member), created on first use. This is an
  M2 change and needs its own decision.

**Evidence:** Fellow allows action items that belong to no project (on a personal action-items page). That works because
Fellow itself owns the task truth. JAAS's spec forbids that ("must not duplicate task truth"), so an M3-owned task list
is ruled out. A personal inbox project gives the same result while M2 stays the only owner of tasks.

## 5. Real-time co-editing of minutes

**Recommendation:** phase 1 uses versioned saves:
- Each save sends `If-Match: version`; a stale version gets 409 and the UI shows a merge screen.
- Notes are stored **per agenda item**, so two note-takers working on different items never collide.
- Every save is a version; publishing freezes one.

**Upgrade path (phase 3, if needed):** `ygo`/`yserve` is a pure-Go Yjs port that runs as an HTTP handler inside the
Go backend, with no Node, Redis or CGO. It works with standard Hocuspocus and y-websocket clients and saves versions
periodically. That removes the main cost of Option C (a separate Node service). Its maturity must be checked before it
is adopted.

**Why not now:** a minutes editor has few writers (usually one note-taker), and a CRDT adds a websocket surface, a
binary document format to keep and a second source of truth next to the versions.

## 6. Does `meetings:read_all` include private meetings?

**Recommendation:**
- `meetings:read_all` sees a private meeting's metadata, participants, **published** minutes and decisions.
- It never sees draft notes, private talking points or personal notes. Those are owner-only, even for admins (as in
  Fellow).
- Every read through `read_all` of a meeting the reader is not invited to writes an audit entry.
- Meetings marked **confidential** also require a separate `meetings:read_confidential`.
- Full eDiscovery (search everything, legal hold) belongs to M8.

**Evidence:** Microsoft Teams meeting notes are shared only with the people on the original invite. Compliance access
goes through eDiscovery, a separate privileged and audited tool, not through the meeting UI. Fellow says that no other
attendee, admin or manager can see private notes.

## 7. Who owns rooms: M3 or M4?

**Recommendation:** M3 owns a small **bookable resource** table (room or equipment; name, site, capacity, `bookable`)
and the **bookings**. The booking table has the exclusion constraint `EXCLUDE USING gist (resource_id WITH =, during
WITH &&)`. When M4 exists, the physical asset record (serial number, maintenance, depreciation) lives in M4, and the M3
resource gets a nullable `asset_id` reference.

**Evidence:** Exchange/Microsoft 365 and Google Workspace model a room as a calendar resource: a mailbox or calendar that
auto-accepts or declines by conflict, sometimes via a delegate. Facilities asset data is kept elsewhere. Booking is a
calendar problem; the asset lifecycle is an inventory problem. M4 is not started, so waiting for it would block rooms
indefinitely.

## 8. Outbound integrations (Google or Microsoft sync, Zoom or Teams links)

**Recommendation:** none in phases 1-2. At the phase-3 gate, write a decision like DPJ-41, but adapted to the different
risk:
- **Fixed provider hosts** in an allowlist, unlike the user-supplied URLs of webhooks.
- OAuth tokens encrypted at rest through the secrets service, with refresh and revoke handled.
- Change tracking through sync tokens or delta queries (Google `syncToken`, Graph delta), with webhook receivers
  verified.

Phase 1 gets almost the same benefit with no outgoing call: the iCal feed (CS-1) and `.ics` files (CS-2) are pulled by
Google, Outlook and Apple.

## 9. Self-hosted video

**Recommendation:** phases 1-2 keep a link only (https, validated, never fetched by the server). If self-hosting is ever
wanted, add an **optional** Jitsi integration that signs a per-participant JWT join link. JAAS never operates the media
server; the tenant runs it.

**Evidence:** sources (some vendor-authored) agree that Jitsi suits "host our own Zoom alternative" and LiveKit suits
"build a custom video product". Running either means a TURN server, an SFU and recording infrastructure. That is a
product of its own, outside an ERP module.

## 10. Recording and transcription consent; retention defaults

**Recommendation for M3 (phase 1 does no recording):**
- Store `recording_allowed` per meeting and a `consent` row per participant (`granted | declined`, time, method).
- M9 must check these before it captures anything.
- If someone declines, the meeting cannot be recorded. Following Teams, they keep a view-only experience.
- Default retention, configurable per tenant:
  - transcripts: 90 days (vendor defaults are 30 or 90);
  - drafts: purged 30 days after publishing;
  - published minutes and decisions: kept (no automatic delete) until a tenant policy says otherwise;
  - legal hold: M8 later.

**Evidence:**
- Teams has an admin "explicit recording consent" policy; whoever declines gets view-only.
- Zoom shows a recording disclaimer, which guests always get.
- US law: all-party consent states differ by source (10-13 states), so the safest rule is to ask everyone.
- GDPR: a transcript is personal data. Sources disagree on whether consent or legitimate interest is the right legal
  basis between employer and employee. **That choice needs legal input.**

## 11. External guests

**Recommendation:** phase 3 or later. It needs:
- M10 e-mail (not started);
- iTIP (RFC 5546) REQUEST, REPLY and CANCEL;
- a guest identity model (a signed RSVP link, no account);
- a security review like DPJ-47 (public surface).

Phase 1: the organizer downloads the `.ics` and forwards it from their own mail client.

## 12. Postgres row-level security on the `meetings` schema

**Recommendation:** not as an M3-only feature. M3 follows M2: an application authorizer, `tenant_id` on every query, and
isolation tests that cross tenants. Platform-wide RLS (set through `set_config('app.tenant_id', …, true)` per
transaction, `FORCE ROW LEVEL SECURITY`, separate roles for migrations and jobs) should be one decision for all
modules, otherwise isolation behaves differently per module.

**Evidence:**
- Table owners and superusers bypass RLS unless `FORCE` is set.
- A plain `SET` leaks across pooled connections; only the transaction-scoped `set_config` is safe.
- Every job, migration and admin script must set the tenant.
- One practitioner report dropped RLS at scale because of that operational load (anecdotal).
- JAAS runs a pooled GORM connection with background dispatchers, so adding RLS only to M3 would add these risks for
  one module.

## Sources (accessed 2026-11-02)

- Google Calendar recurring events: https://developers.google.com/calendar/api/guides/recurringevents
- Microsoft Teams explicit recording consent: https://learn.microsoft.com/en-us/MicrosoftTeams/meeting-recording , https://mc.merill.net/message/MC937915
- Teams meeting notes access (invitees only): https://mc.merill.net/message/MC1218712 ; eDiscovery for Teams: https://learn.microsoft.com/purview/ediscovery-teams-investigation
- Zoom recording disclaimer: https://uis.jhu.edu/zoom/new-setting-recording-disclaimer-enabled
- Fellow private notes: https://help.fellow.app/en/articles/4274961-private-notes ; action items without a meeting: https://help.fellow.app/en/articles/3956757-write-action-items-not-attached-to-a-meeting
- Exchange room mailbox auto-accept: https://technet.microsoft.com/en-us/library/jj215781 ; Nylas room booking: https://developer.nylas.com/docs/cookbook/calendar/book-room-resources.md
- Nextcloud rooms/resources: https://help.nextcloud.com/t/resources-in-calendar-app/2409 , https://docs.nextcloud.com/server/30/admin_manual/groupware/index.html
- ygo / yserve (pure-Go Yjs): https://pkg.go.dev/github.com/Deln0r/ygo@v1.16.0
- Jitsi vs LiveKit: https://ayedo.de/en/posts/webrtc-im-grossen-stil-der-wechsel-von-jitsi-zu-livekit-auf-kubernetes/
- Recording law overviews (vendor sources, disagree): https://www.gong.io/?p=25252 , https://fireflies.ai/blog/is-it-illegal-to-record-a-conversation , https://docs.buildbetter.ai/pages/FAQ/consent-and-compliance
- Postgres RLS: https://oneuptime.com/blog/post/2026-01-21-postgresql-row-level-security/view , https://dev.to/canu/why-we-stopped-using-row-level-security-after-1m-rows-4dpb
