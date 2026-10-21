# HR / Compensation — Decisions

- **DCO-01** Compensation owns the annual base; payroll owns the salary structure and the calculation. Applying a
  record calls the payroll `CTC` port inside the same transaction. The worker keeps their payroll structure, and
  a worker without a payroll assignment is recorded with `payroll_synced = false` (CP-003).
- **DCO-02** Records never overlap. This is enforced twice: a per-worker advisory lock in the service and a gist
  exclusion constraint. A new record must start after the open one (`BACKDATED` otherwise); corrections are new
  records with reason `correction`, nothing is deleted (CP-002).
- **DCO-03** Off-cycle maker-checker (CP-007): a worker's first record and raises up to 10% apply at once. Cuts and
  raises above 10% wait for a `compensation:approve` holder who is not the requester (`COMP_SELF_APPROVAL`). The
  threshold is a constant for v1; a tenant setting comes with M7 Approvals.
- **DCO-04** A merit cycle is approved by someone other than the user who opened it. Applying it is one transaction;
  a second apply is `CYCLE_STATE`, so applying twice never writes twice.
- **DCO-05** A department with no budget row has a budget of zero. Every positive proposal there needs
  `override_budget` with a justification, so a forgotten budget fails safe instead of being unlimited.
- **DCO-06** Proposals snapshot base, compa band, rating band and manager when the cycle opens (CP-005). Later
  changes in core or performance do not move an open cycle. Workers without a current record or department get no
  proposal.
- **DCO-07** Amounts are `kernel.Money` (paise) and percentages are hundredths; a merit amount is rounded to the
  rupee. No floating point anywhere.
- **DCO-08** Events and audit metadata carry ids, dates and reasons only, never amounts, percentages or ratings
  (HR-009).
