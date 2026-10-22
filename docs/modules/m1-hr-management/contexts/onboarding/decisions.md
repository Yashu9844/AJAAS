# HR / Onboarding — Decisions

- **DOB-01** Checklists are started by hooks that run inside the transaction of the event that causes them
  (`core/api.ActionHook`, `recruitment/api.HireHook`). A failed start rolls the hire or action back, so a worker is
  never hired without their checklist. The alternative, consuming outbox events, was rejected: it is eventually
  consistent and needs a consumer per source.
- **DOB-02** Starting is idempotent twice over: a unique index on `(tenant, kind, source_id)` and one open checklist
  per worker and kind. A recruitment hire followed by a core hire for the same worker therefore yields one checklist.
  The manual start returns 409 instead of silently doing nothing.
- **DOB-03** Rescinding a core action cancels the checklist it started (OB-03). Work already done stays visible as
  history on the cancelled checklist.
- **DOB-04** The most specific matching templates apply, not a merge of all matches (OB-02). Ties apply together in
  creation order. A template with no criteria is the tenant default.
- **DOB-05** Manager tasks go to the worker's manager on the anchor date. Without a manager they fall into the HR pool
  rather than being lost.
- **DOB-06** Document tasks are backed by `employee_documents.verified_at`. Onboarding stores only the evidence id;
  the file lifecycle stays in employee (M8 later).
- **DOB-07** Offboarding has four fixed clearance areas (manager, hr, it, finance). Completing the last one stamps the
  checklist, emits `hr.onboarding.clearance_completed` and is what payroll reads through `onboarding/api.Clearance`.
- **DOB-08** Exit-interview answers are readable by the worker and `onboarding:exit_interview` only. The `read` and
  `manage` permissions deliberately do not include them, and reads by the permission holder are audited. Answers are
  never in events or audit metadata.
- **DOB-09** The worker's login is deactivated by core only after the last working day, so the departing worker can
  still reach their offboarding tasks and the exit interview until then.
