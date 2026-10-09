# Module 6 — Decisions

- D6-01 Module 6 = Recruitment & ATS (masterplan). The Approvals scaffold that sat at 6 is now `module-x-approvals` (unnumbered). Earlier references to "Module 6 (Approvals)" in Modules 3/4 decisions (approval chains) now mean that unnumbered Approvals module.
- D6-02 Hire uses Module 0 `InviteUser` (status invited, no password handled by recruitment); the new user gets no roles (RS-T5).
- D6-03 Module 1 gains read-only accessors `DepartmentService()` and `DesignationService()` on its Module so job openings can validate references (recorded in module-1 decisions).
- D6-04 Hire is a three-step idempotent saga (architecture § Hire flow) because Module 2 `CreateEmployee` runs its own transaction and must see the committed user row.
- D6-05 Offer decisions are recorded by recruiters on the candidate's behalf; a candidate portal is out of scope.
- D6-06 No automatic payroll assignment on hire: CTC is assigned in Module 5 by HR (keeps salary decisions in payroll).

- D6-07 (P5) Moving a candidate out of `offer` (rejected/withdrawn) withdraws its open offer in the same transaction, so RC-006 never leaves an orphan open offer.
- D6-08 (P5) A new offer is refused while one is open **or accepted** (OFFER_EXISTS); hire uses the accepted one. Accepting an offer past expires_on → OFFER_STATE.
- D6-09 (P5) Hire is allowed for jobs in open or on_hold; closed/filled → JOB_NOT_OPEN. If the candidate's email already belongs to a tenant user, Module 0 InviteUser refuses (409) and hire stops before any employee is created.
