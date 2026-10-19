# HR / Recruitment (ATS) — Decisions

> Part of **M1 HR Management** (bounded context `recruitment`). Formerly the stand-alone "module-6-recruitment" bundle; module numbers in older text were renamed to the canonical M0–M12 / HR-context names.

- D6-01 HR/Recruitment = Recruitment & ATS (masterplan). The Approvals scaffold that sat at 6 is now `module-x-approvals` (unnumbered). Earlier references to "M7 Approvals" in HR/Attendance, HR/Leave decisions (approval chains) now mean that unnumbered Approvals module.
- D6-02 Hire uses M0 Identity `InviteUser` (status invited, no password handled by recruitment); the new user gets no roles (RS-T5).
- D6-03 HR/Organization gains read-only accessors `DepartmentService()` and `DesignationService()` on its Module so job openings can validate references (recorded in module-1 decisions).
- D6-04 Hire is a three-step idempotent saga (architecture § Hire flow) because HR/Employee `CreateEmployee` runs its own transaction and must see the committed user row.
- D6-05 Offer decisions are recorded by recruiters on the candidate's behalf; a candidate portal is out of scope.
- D6-06 No automatic payroll assignment on hire: CTC is assigned in HR/Payroll by HR (keeps salary decisions in payroll).

- D6-07 (P5) Moving a candidate out of `offer` (rejected/withdrawn) withdraws its open offer in the same transaction, so RC-006 never leaves an orphan open offer.
- D6-08 (P5) A new offer is refused while one is open **or accepted** (OFFER_EXISTS); hire uses the accepted one. Accepting an offer past expires_on → OFFER_STATE.
- D6-09 (P5) Hire is allowed for jobs in open or on_hold; closed/filled → JOB_NOT_OPEN. If the candidate's email already belongs to a tenant user, M0 Identity InviteUser refuses (409) and hire stops before any employee is created.

## Changelog

2026-10-09 — P1 DONE: HR/Recruitment = Recruitment (D6-01); design frozen.
2026-10-09 — P2 DONE: models, pipeline graphs, migrations 000045–000050.
2026-10-09 — P3 DONE: DTOs, validators.
2026-10-09 — P4 DONE: repositories.
2026-10-09 — P5 DONE: services, hire saga, events, relay; D6-07..D6-09.
2026-10-09 — P6 DONE: HTTP, wiring, swagger.
2026-10-09 — P7 + P9 DONE: integration 63/63; backend CLOSED.
