# M3 Meetings - research: internals of reference implementations

**Read on 2026-11-02** from shallow clones in `D:\.pycache\personal\JASS\meetings-src\` (outside the JAAS repo, read only,
nothing installed, built or executed; `cal.com` is a sparse checkout of `packages/prisma`, `packages/features/bookings`,
`packages/lib`). Clones: `sabre-dav` (sabre-io/dav, BSD-3), `go-webdav` (MIT), `golang-ical` (Apache-2.0),
`rrule-go` (MIT), `cal.com` (now calcom/cal.diy, MIT). No code is copied into JAAS; this file records ideas only.

## 1. sabre/dav (CalDAV server used by Nextcloud and Baikal)

**Data model** (`examples/sql/pgsql.calendars.sql`):

| Table | Key columns | Idea for JAAS |
|---|---|---|
| `calendars` | `id`, `synctoken` (int, starts 1), `components` | One monotonically increasing token per collection. |
| `calendarinstances` | `calendarid`, `principaluri`, `access` (1 owner, 2 read, 3 readwrite), `share_invitestatus`, `timezone`, `transparent` | Sharing = an *instance* row per principal pointing to one calendar; ACL lives on the instance. |
| `calendarobjects` | `calendardata` (raw iCalendar bytes), `uri`, `etag` (md5), `uid`, `componenttype`, **`firstoccurence`, `lastoccurence`** (unix ints), unique `(calendarid, uri)` | Store the source of truth as a blob but **denormalise the time span** so range queries can pre-filter with an index. |
| `calendarchanges` | `uri`, `synctoken`, `calendarid`, `operation` (1 add, 2 modify, 3 delete); index `(calendarid, synctoken)` | Change journal for RFC 6578 `sync-collection`; the same shape as our outbox, but per collection. |
| `schedulingobjects` | per-principal inbox of iTIP messages | iTIP (RFC 6638 scheduling) delivery as stored messages. |

**Recurrence handling** (`lib/CalDAV/Backend/PDO.php::getDenormalizedData`):
- Non-recurring: `lastoccurence = DTEND` (or DTSTART + DURATION, or +1 day for all-day).
- Recurring: iterate with `VObject\Recur\EventIterator` until the end; **if the rule is infinite, `lastoccurence` is set to
  `MAX_DATE = 2038-01-01`** (32-bit timestamp limit). Expansion happens at write time only to compute the span; occurrences
  are never stored.
- Query (`calendarQuery`): SQL pre-filter `lastoccurence > :start AND firstoccurence < :end`, then a PHP post-filter
  (`CalendarQueryValidator`) that expands recurrences for the window. Cost is proportional to the number of recurring
  series overlapping the window, each expanded from DTSTART.

**Conflict detection:** none. CalDAV allows overlaps; free/busy (`FreeBusyQueryReport`, schedule outbox) only *reports*
busy time. Room booking in Nextcloud is a separate resource backend with its own checks.

**Sync:** every write increments `calendars.synctoken` and appends a `calendarchanges` row. `getChangesForCalendar` reads
changes `>= token`, collapses multiple changes of one URI to the last one, and supports `limit` with a "truncated" flag
(fetches limit+1). Old tokens are valid as long as the journal is kept.

**Take-aways:** (1) rule text + denormalised `[first, last]` span is enough for range queries; (2) infinite rules need an
explicit horizon; (3) a per-collection change journal gives cheap incremental sync for feeds/CalDAV clients.

## 2. emersion/go-webdav (Go CalDAV)

- Server side is an interface `caldav.Backend` (`CreateCalendar`, `ListCalendars`, `GetCalendarObject`,
  `ListCalendarObjects`, `QueryCalendarObjects`, `PutCalendarObject` with `IfMatch/IfNoneMatch`, `DeleteCalendarObject`) plus
  `UserPrincipalBackend`. JAAS would implement it over the `meetings` schema - storage stays ours.
- `caldav.Filter`/`Match` evaluate a query in memory; time-range matching on recurring components calls
  `comp.RecurrenceSet(loc)` (go-ical -> rrule-go) and `rset.Between(start, end, true)`; a code comment notes the inclusive/
  exclusive boundary is not exact (start inclusive, end exclusive per RFC 4791 9.9). Only VEVENT is handled.
- ETags are honoured (`If-Match` on PUT); **server-side `sync-collection` is not implemented** (only the client has
  `SyncCollection`). A JAAS CalDAV endpoint would therefore be read-only first and full-listing, or we add sync ourselves.

## 3. teambition/rrule-go

- Port of python-dateutil: `RRule` (iterator `Iterator()`, `All()`, `Between(after, before, inc)`, `Before`, `After`) and
  `Set` (RRULE + RDATE + EXDATE; EXDATEs are matched with `time.Equal`, so the exception must be the exact instant).
- **`between()` iterates from DTSTART every time** (util.go): cost is O(occurrences from series start to window end),
  not O(window). For a daily meeting started 2 years ago, a one-week window still walks ~730 candidates. Mitigation:
  cache "next occurrence" or materialise a bounded horizon.
- `All()` on an unbounded rule loops until `MAXYEAR = 9999` - must never be called; always use `Between` or a capped iterator.
- Times are generated with `time.Date(..., dtstart.Location())`, so DTSTART must be in the **IANA zone of the meeting**
  (not UTC) for "09:00 every Tuesday" to survive DST. Go normalises non-existent local times (spring-forward gap) by
  moving forward; tests must pin the expected behaviour.
- No releases are tagged; last push 2024-08. Stable but low activity -> pin a commit hash and keep our own golden tests.

## 4. arran4/golang-ical

- Parser/serialiser for VCALENDAR/VEVENT/VTODO etc.; `recurrence.go` parses and formats RRULE (incl. `WeekdayNum` such as
  `-1SU`) but **does not expand** occurrences - pair it with rrule-go.
- `timezones.go`: `WindowsTimezoneToIANA` / `IANAToWindowsTimezone` - useful for Outlook-generated `.ics` (Windows zone ids).
- Has fuzz tests (`calendar_fuzz_test.go`) - good for an untrusted-input parser.
- `ParseCalendarFromUrl` exists: JAAS must never call it (SECURITY.md section 9, no server-side fetch of user URLs).

## 5. Cal.diy / Cal.com (`packages/prisma/schema.prisma`, `packages/features/bookings`)

**Data model:** `Booking` (`uid` unique, `idempotencyKey` unique nullable, `startTime`, `endTime`, `status`,
`recurringEventId`, `iCalUID`, `iCalSequence`, `fromReschedule`) with indexes `(startTime, endTime, status)`,
`(userId, status, startTime)`, `(recurringEventId)`. `Attendee` rows per booking. `BookingReference` stores the
provider-side ids (`type`, `uid`, `meetingId`, `meetingUrl`, `meetingPassword`, `externalCalendarId`,
`thirdPartyRecurringEventId`) per integration. `SelectedCalendar` keeps watch-channel state (`channelId`,
`channelResourceId`, `channelExpiration`). `CalendarCacheEvent` mirrors external events (`externalId`, `externalEtag`,
`iCalUID`, `start`, `end`, `recurringEventId`, `originalStartTime`), unique `(selectedCalendarId, externalId)`.

**Recurrence:** **materialised.** `RecurringBookingService` receives the list of occurrence dates from the client and
creates one `Booking` per occurrence sharing `recurringEventId`; availability is checked only for the first
`numSlotsToCheckForAvailability = 1` occurrence. Upside: every occurrence is a normal row (reschedule/cancel one is trivial).
Downside: later occurrences can be double-booked; editing "all" touches N rows.

**Conflict detection:** in application code. `checkForConflicts` sorts busy intervals and tests overlap with the
half-open slot `[start, start+length)`; busy intervals come from Cal bookings plus cached/external calendars. No database
constraint - a race between two bookings is possible and is mitigated by the idempotency key.

**Idempotency:** `IdempotencyKeyService.generate` = UUIDv5 of `startTime.endTime.userId[.reassignedById]`; the unique
column turns a duplicate submission for the same host and slot into a constraint error. It is a *natural* key (slot +
host), not a client-supplied header.

**Sync:** Google/Microsoft watch channels per selected calendar with explicit expiration stored and renewed; events cached
with etag for change detection; `iCalUID` + `iCalSequence` keep outgoing invitations consistent (SEQUENCE increments on
each significant change, RFC 5546).

## 6. Summary of lessons for M3

| Topic | sabre/dav | Cal.diy | Lesson for JAAS |
|---|---|---|---|
| Recurrence storage | rule + span, expand on read | materialise every occurrence | Hybrid: rule is the truth, **materialise a bounded horizon** of occurrence rows (needed for per-occurrence notes, attendance, room locks). |
| Infinite rules | span to 2038 | client sends a finite list | Forbid infinite in phase 1 (COUNT/UNTIL cap), or rolling horizon job. |
| Conflicts | none (free/busy only) | app-level, first occurrence only | Use a **Postgres exclusion constraint** on room bookings per occurrence; attendee conflicts are warnings, not errors. |
| Sync | per-collection token journal | provider watch channels + etag cache | Outbox already exists; add per-user feed token + change journal later for CalDAV. |
| Idempotency | ETag / If-None-Match | natural key UUIDv5 | Keep JAAS `Idempotency-Key` middleware; add natural unique keys (series id + recurrence id). |
| iTIP | stored scheduling objects | iCalUID + sequence | Keep `ical_uid` and `sequence` per meeting from day one, even before e-mail invites exist. |
