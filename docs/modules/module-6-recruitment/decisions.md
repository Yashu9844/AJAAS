# Module 6 — Decisions

- D6-01 Module 6 = Recruitment & ATS (masterplan). The Approvals scaffold that sat at 6 is now `module-x-approvals` (unnumbered). Earlier references to "Module 6 (Approvals)" in Modules 3/4 decisions (approval chains) now mean that unnumbered Approvals module.
- D6-02 Hire uses Module 0 `InviteUser` (status invited, no password handled by recruitment); the new user gets no roles (RS-T5).
- D6-03 Module 1 gains read-only accessors `DepartmentService()` and `DesignationService()` on its Module so job openings can validate references (recorded in module-1 decisions).
- D6-04 Hire is a three-step idempotent saga (architecture § Hire flow) because Module 2 `CreateEmployee` runs its own transaction and must see the committed user row.
- D6-05 Offer decisions are recorded by recruiters on the candidate's behalf; a candidate portal is out of scope.
- D6-06 No automatic payroll assignment on hire: CTC is assigned in Module 5 by HR (keeps salary decisions in payroll).
