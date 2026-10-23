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

---

## Parity decisions (2026-10-23)

- **DR-01** The seat is reserved in the first hire step, in the same transaction that locks the job, so a failed hire can
  never leave a user or profile behind for a seat that does not exist. Capacity is `headcount − hired − reserved`. The
  reservation is turned into `hired_count` when the hire completes, and `POST /candidates/{id}/hire/release` gives it
  back while no employee profile exists. A retry reuses its own reservation.
- **DR-02** Requisitions read positions through `core/api.Positions`, a new port beside `Directory` so existing fakes
  keep compiling. Seats are the whole open FTE today. Seats already requested by submitted requisitions, or approved
  ones whose job is still live, are subtracted, so two requisitions cannot both claim the last seat.
- **DR-03** Approving a requisition creates a **draft** job, never an open one: HR still reviews the posting before it is
  published.
- **DR-04** Scorecard weights are percentages in the API (`60` means 60.00) and must total exactly 100. Averages are
  computed in hundredths with integer arithmetic and the overall score is the weighted sum, so there is no float
  rounding. Competency names match case-insensitively.
- **DR-05** Only the assigned interviewer scores an interview; anyone else gets 404, as for feedback. A repeated call
  replaces the earlier scores. Scores and notes never reach events or audit metadata (RG5).
- **DR-06** Offer approval is a hook on offer creation (`OfferGate`) so a tenant with no policy is unchanged. A
  `pending_approval` offer counts as a live offer for the one-open-offer rule and is withdrawn when the candidate leaves
  the pipeline. A rejection withdraws the offer; it does not move the candidate back, because the existing withdrawal
  rules already decide that when HR acts.
- **DR-07** `recruitment:approve` is a new sensitive permission, defaulted to the HR admin template only. The creator of
  a requisition or offer can never approve it, whatever role they hold.
