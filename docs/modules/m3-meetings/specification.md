# M3 Meetings — Specification (PROPOSED)

> Not frozen. This is the target-state contract from `docs/ARCHITECTURE.md` section 5, combined with the logical data
> model from `model/db-model.md`. API paths are proposed shapes. Universal rules (tenancy, idempotency, outbox/inbox,
> envelopes, security, latency) in `docs/ARCHITECTURE.md` section 3 apply in full.

**Purpose:** plan meetings and capture decisions/action items.

**Data model:** Meeting, Participant, AgendaItem, MinutesVersion,
AttendanceRecord, ActionItemReference, MeetingResource, RecurrenceRule,
MeetingChangeLog.

**Representative APIs:**
- `GET/POST /api/v1/meetings`
- `GET/PATCH /api/v1/meetings/{id}`
- `POST /api/v1/meetings/{id}/participants`
- `POST /api/v1/meetings/{id}/agenda-items`
- `PUT /api/v1/meetings/{id}/minutes`
- `POST /api/v1/meetings/{id}/publish-minutes`
- `POST /api/v1/meetings/{id}/action-items`

**Rules:** timezone-aware start/end times; recurrence expansion bounded;
meeting visibility distinct from participant visibility; minutes version
history; action items should link to task IDs and must not duplicate
task truth in the Meetings service.

**Events:** meeting scheduled/updated/cancelled, participant changed,
minutes published, action item created. Notifications handles reminders
and changes.

**Acceptance tests:** timezone/DST cases; duplicate create retry;
recurrence bounds; private meeting ACL; minutes versioning; event
replay; action item link consistency.

## Proposed entities

| Entity/table | Key fields (illustrative) |
|---|---|
| `Meeting` | meeting_id, tenant_id, title, organizer_id, starts_at, ends_at, timezone, visibility, status, version |
| `MeetingParticipant` | meeting_id, user_id, response, attendance_state |
| `AgendaItem` | agenda_item_id, meeting_id, position, title, owner_id |
| `MinutesVersion` | minutes_version_id, meeting_id, version, body_ref, published_at |
| `RecurrenceRule` | meeting_id, rule, until_at, occurrence_limit |
| `MeetingActionReference` | meeting_id, task_id, title, owner_id, due_at, status_ref |
| `MeetingResource` | meeting_id, resource_ref, resource_type |
| `MeetingAudit` | audit_id, meeting_id, actor_id, action, occurred_at |

## Relationships and invariants

Meeting 1:N MeetingParticipant, AgendaItem and MinutesVersion. Meeting 0:1 RecurrenceRule. MeetingActionReference stores external task IDs, not duplicate authoritative task state. Enforce timezone-aware timestamps and visibility ACL.

## Service dependencies

M0 identity context; HR provides stable participant IDs; Projects & Tasks owns action-item tasks; Documents stores attached files/minutes artifacts; Notifications sends reminders.
