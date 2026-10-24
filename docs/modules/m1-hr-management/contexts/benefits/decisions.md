# HR/Benefits — decisions

- **DB-01** Costs are per month because payroll runs monthly (specification open question resolved). A plan's costs never
  change in place: a cost change is a new version that starts after the current one and not in the past, so the history
  of what a worker was charged is exact (BN-001).
- **DB-02** Payroll is charged a whole month for every election that covers the **last day** of the period, and nothing
  otherwise. This avoids proration rules for mid-month starts and ends; an election that ends before the last day stops
  costing in that month. The employer cost is reported (`GET /reports/enrolment`) and never deducted.
- **DB-03** The payroll port moved to `payroll/api` (`BenefitLine`, `BenefitDeductions`) so HR/Benefits can implement it
  without importing payroll's services; payroll's own names are aliases, so nothing in payroll changed behaviour.
- **DB-04** Eligibility lists explicit grade ids rather than a rank range: core publishes a grade's existence and active
  flag but not its rank. An empty list means no restriction. Eligibility is evaluated on the election's start date.
- **DB-05** Windows are open enrolment (tenant-wide dates), new hire (from the joining date for `new_hire_days`) and
  life event (the event date for `life_event_days`), all inclusive. The checks are made against today for the worker's own
  actions; `benefits:manage` may end an election at any time. Life events are accepted as recorded, with no approval step.
- **DB-06** An election row is never edited except to end it. Ending sets `effective_to` (exclusive) and a reason, so
  history stays complete. A waiver is an election row with no level; electing later ends the waiver with reason
  `replaced`. A same-day replacement leaves an empty range (`effective_to = effective_from`), which the schema allows.
  One open-ended row per worker and plan is enforced by a partial unique index and an advisory lock.
- **DB-07** Termination (BN-C08) ends only open-ended active elections, at the day after the last working day, and
  records the core action id. Rescinding reinstates exactly the rows that action ended. The hook returns its error, so a
  failure rolls the termination back: a missed end would keep deducting, which is worse than a blocked action.
- **DB-08** Dependants and life events are sensitive (HR-009). Only the worker manages their dependants; HR reads them with
  `benefits:read_sensitive`. A manager gets nothing. Events, audit metadata and reports carry ids, statuses and counts only.
- **DB-09** Retiring a plan blocks new elections but leaves existing ones running until they are ended; HR ends them with
  `benefits:manage` when a plan is withdrawn.
