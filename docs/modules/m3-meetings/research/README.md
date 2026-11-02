# M3 Meetings - research (2026-11-02)

Research input for the M3 design gate. No code, migrations or specification changes were made. Web sources accessed
2026-11-02; reference repositories cloned shallowly (read only, never run) into `D:\.pycache\personal\JASS\meetings-src\`.

## Index

| File | Holds |
|---|---|
| `features.md` | feature list in 18 groups with MUST/SHOULD/LATER and journeys for organizer, attendee, note-taker, manager, admin |
| `market-research.md` | 18 products: features, pricing model, what to copy, what to avoid |
| `open-source.md` | OSS alternatives and building blocks: licence, maturity, how it works, fit (embed/integrate/learn) |
| `internals.md` | data model, recurrence, conflict detection and sync in sabre/dav, go-webdav, rrule-go, golang-ical, Cal.diy |
| `user-journeys.md` | 14 detailed journeys (steps, failure paths, checks) for the proposed phase 1-2 |
| `decisions-validation.md` | the 12 open questions answered with evidence, alternatives and confidence |
| `architecture-options.md` | 3 options, decision matrix, recommendation, draft data model, API, events, threats, performance targets and benchmark plan |

## Recommendation (Option A, "rule + bounded occurrences, relational notes")

1. Go package `internal/meetings`, schema `meetings`, in-process like M2; same outbox/inbox, idempotency, `version`/If-Match.
2. Series hold an RFC 5545 RRULE (whitelisted, COUNT<=500, UNTIL<=2 years) in the meeting's IANA zone; occurrences are
   **materialised for a bounded horizon** with natural key `(series_id, recurrence_id)`; exceptions and cancellations are occurrence rows.
3. Notes, agenda, attendance, RSVP, decisions and action items hang off an occurrence.
4. Minutes = versioned markdown (append-only versions, publish freezes); real-time CRDT editing is a later option.
5. Rooms: PostgreSQL exclusion constraint (`btree_gist`) - double booking impossible; people conflicts are warnings.
6. Action items are M2 tasks created **as the caller** through a `Tasks` port; M3 stores links (+ stale snapshot).
7. All permissions in M0: tenant entries (`meetings:create|read_all|manage_all`, `rooms:manage`, ...) plus resource roles on a meeting series (`meeting_viewer <- attendee <- note_taker <- organizer`).
8. Calendar interop phase 1 = per-user iCal feed + `.ics` per meeting (Go: rrule-go + golang-ical); CalDAV and Google/Microsoft two-way sync later behind a provider port.
9. Video phase 1 = https link only; signed Jitsi/LiveKit join links later. Reminders = events to M10; files = M8/storage port.
10. AI = seams only (context packs, document feed, draft-minutes endpoint); transcription engines (whisper.cpp, faster-whisper, Vexa) belong to M9, with consent flags in M3.

## Open questions for the owner

Recommended answers with evidence: `decisions-validation.md`.

1. **M0 change:** extend `applies_to` / `resource_role_assignments.resource_type` with `meeting` (resource roles on a series)? Alternative: derive roles from participant kind inside M3 (conflicts with DPJ-49 "M0 decides everything").
2. Phase-1 scope: is MUST in `features.md` the right cut? Are rooms (RR-1/2) phase 1 or 2?
3. Infinite recurrence: forbid in phase 1 (COUNT/UNTIL required) or allow with a rolling 12-month horizon?
4. Action items when the note-taker has no M2 project: default "meeting actions" project per tenant/team, or require a project?
5. Real-time co-editing of minutes in phase 1 (adds a Node/Yjs service) or versioned save with conflict 409 first?
6. Does `meetings:read_all` (auditor) include published minutes of private meetings? Never private talking points?
7. Rooms owned by M3 or by M4 Assets (M3 keeps only bookings)?
8. Outbound integrations (Google/Microsoft sync, Zoom/Teams links) need a SECURITY.md section 9 exception like DPJ-41 - when?
9. Self-hosted video (Jitsi or LiveKit) in scope at all, or link-only for the foreseeable future?
10. Recording/transcription consent rules and retention defaults (e.g. transcripts 90 days, minutes 7 years) - legal input needed.
11. External guests (e-mail invites with iTIP) - needed in phase 1-2? It needs M10 e-mail and a guest identity model.
12. Optional Postgres RLS on the `meetings` schema as defence in depth, or keep the application Authorizer only (as M2)?
