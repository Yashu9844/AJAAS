# HR/Learning — decisions

- **DL-01** Content is never hosted. A course is metadata plus an https link or an opaque document reference (the file
  belongs to M8 later). Links must be absolute https URLs with a host and no user-info, so `javascript:`, `data:` and
  plain `http:` never reach a learner's browser. A thumbnail is an https URL or an opaque reference, never inline data.
- **DL-02** Versioning covers content only (title, description, provider, content, duration, tags). Switching
  `verification` or `self_enrol` is a policy change and does not create a version, so history shows what learners
  actually read.
- **DL-03** An enrolment's dedupe key is `assignment · course · cycle`. Two different assignments of the same course give
  a worker two enrolments on purpose (for example a department rule plus a personal assignment with its own due date);
  running one assignment twice, a hook firing twice or two recurrence runs never do.
- **DL-04** Rule matching reads placement from HR/Core on the day and the employee directory. The current workforce is
  `active`, `probation`, `notice` and `on_leave`; an exited or inactive worker is never enrolled. `job_profile`,
  `location` and `department` ids are not validated for existence (core exposes no read for them), so a mistyped id matches
  nobody and the response says `matched: 0`.
- **DL-05** The joiner and mover hook runs inside core's transaction but in a savepoint and always returns nil. A
  learning failure (database error, a retired course) is swallowed and the worker is picked up the next time the hook or
  a recurrence run touches them; HR actions are never blocked or rolled back by learning (LN-C07).
- **DL-06** Overdue is computed, not stored, so no job has to flip statuses and a change of time zone is reflected at
  once. Due dates are calendar dates compared with today in the tenant's learning time zone (default `Asia/Kolkata`;
  HR-007). The hourly recurrence loop looks one day ahead of UTC and re-checks each assignment in its own tenant zone.
- **DL-07** Recurrence advances one cycle per assignment per run, from the scheduled date and not from today, so a long
  outage is caught up cycle by cycle instead of skipping compliance periods. A new cycle's due date is `due_in_days`
  from the day the cycle starts; recurring assignments therefore refuse a fixed `due_date`.
- **DL-08** A completion is immutable. `verified` ones are final; a correction writes a new HR-attested completion and
  marks the old `superseded`, so the audit trail keeps every version. A rejected completion stays as history and the
  learner may submit again.
- **DL-09** Verification authority follows the course: `self` verifies at once; `manager` is the learner's direct manager
  (from HR/Organization mappings) or `learning:manage`; `hr` is `learning:manage` only. Nobody verifies their own, even
  with `learning:manage`. Others get 404, not 403, so completions of strangers are not discoverable.
- **DL-10** A waiver cannot hide a pending verification (409 `COMPLETION_PENDING`): the completion must be decided first,
  or HR corrects the enrolment.
- **DL-11** Plan progress counts the worker's enrolment in each plan course whatever its source, newest cycle first, so
  completing a plan's course on your own still counts towards the plan. Waived counts as done.
- **DL-12** Events (`learning.course.published|retired`, `learning.assignment.created|cancelled`,
  `learning.enrolment.completed|waived`) go to `lrn_events_outbox` with ids and statuses only and are published to `jaas.learning.events` by the shared
  `kernel.OutboxRelay` (30 s tick, claim with `FOR UPDATE SKIP LOCKED`, 10 attempts, `TestOutboxRelay`). Scores, evidence and
  comments never leave the context (HR-009).
