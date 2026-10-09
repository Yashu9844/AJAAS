# JAAS — Frontend Test Guide: Module 5 (Payroll) + Module 6 (Recruitment)

**Branch:** `module_6_recruitment`. It is built on `main` (Modules 0–4 with the hardening) and already contains `module_5_payroll`, so merging it into `main` brings both modules in. Once it is merged, `main` is the branch to test. **Backend status:** both backends are done and verified against real Postgres. The frontend work (P8) is open for you.

Setup, tenant addressing, the response envelope, pagination and the 404/403 isolation rules are the same as in [FRONTEND_TEST_GUIDE_M3_M4.md](FRONTEND_TEST_GUIDE_M3_M4.md) §2. Differences:
- Migrations now run 000001–000050. **Use a fresh database.**
- The seeded permissions add `payroll:read|manage|approve` and `recruitment:read|manage|hire`.

---

## 1. What's done

| Module | Scope | Endpoints | Tables | Events | Proof |
|---|---|---|---|---|---|
| 5 Payroll | salary structures (components), CTC assignments (history), monthly runs with maker-checker, Indian statutory deductions (PF, ESI, PT, TDS new regime FY 2025-26), LOP from Module 4, payslips, payout CSV | 20 | 7 (`000038–000044`) | 4 on `jaas.payroll.events` | calc 100%, `tests/integration/payroll_test.go` (2 tests) |
| 6 Recruitment | job openings, candidates and pipeline stages with history, interviews and interviewer feedback, offers, hire (invites a user and creates the employee profile) | 20 | 6 (`000045–000050`) | 4 on `jaas.recruitment.events` | services 99.4%, `tests/integration/recruitment_test.go` (2 tests) |

Whole backend: `go build`, `go vet` and `go test ./...` all pass, and the integration suite passes 63/63. Full specs are in `docs/modules/module-5-payroll/specification.md` and `docs/modules/module-6-recruitment/specification.md`. Swagger: `backend/api/swagger.yaml`.

---

## 2. Module 5 — Payroll API (`/api/v1/payroll`)

Money is a JSON number in rupees with at most 2 decimals (for example `1200000` or `41666.67`).

| # | Method & path | Who | Notes |
|---|---|---|---|
| P1 | POST `/structures` | manage | `{name, description?, pf_enabled, esi_enabled, pt_enabled, tds_enabled, components:[{code,name,kind:earning\|deduction,calc:fixed\|percent_of_ctc\|percent_of_basic\|balance,value?}]}`. Exactly one `BASIC`; at most one `balance`. |
| P2–P5 | GET list / GET one / PATCH / POST `/{id}/deactivate` | read / manage | Components are immutable once a structure is assigned. |
| P6 | POST `/assignments` | manage | `{employee_id, structure_id, annual_ctc, effective_from}`. The previous assignment closes automatically. |
| P7 | GET `/assignments?employee_id` | read | History. |
| P8 | GET `/assignments/me` | self | Own CTC + monthly breakdown; `404` if none. |
| P9 | POST `/assignments/preview` | manage | `{structure_id, annual_ctc}` returns a breakdown with no save. Good for a live calculator UI. |
| P10 | POST `/runs` | manage | `{month, year}`; not in the future; one per period (`409 RUN_EXISTS`). |
| P11–P12 | GET list / GET one | read | Totals, warnings (`NET_CAPPED`, `STRUCTURE_OVERFLOW`, `NO_ASSIGNMENT`). |
| P13 | POST `/runs/{id}/calculate` | manage | Recalculating replaces the payslips. |
| P14 | POST `/runs/{id}/approve` | **approve** | The approver must differ from the last calculator (`403 SELF_APPROVAL_FORBIDDEN`). |
| P15 | POST `/runs/{id}/finalize` | approve | After approve only (`409 RUN_STATE`). |
| P16–P18 | GET `/runs/{id}/payslips`, `/runs/{id}/payout.csv`, `/payslips/{id}` | read | CSV only for finalized runs. |
| P19–P20 | GET `/payslips/me`, `/payslips/me/{id}` | self | Finalized runs only; someone else's payslip returns `404`. |

### Payroll checklist
- [ ] Run flow: create, calculate, approve (as a **different** user with `payroll:approve`), finalize. Approving as the calculator shows the maker-checker error.
- [ ] A member sees no payslip before finalize and exactly their own after it.
- [ ] Two approved unpaid-leave days in the month reduce payable days (for example 28/30) and gross pro-rata.
- [ ] Statutory values match the preview: PF = 12% × min(Basic, ₹15,000); ESI only when the monthly gross is ≤ ₹21,000; PT ₹200 when gross is ≥ ₹25,000; TDS 0 at or below ₹12 L taxable.
- [ ] The CSV downloads with the header `employee_code,employee_name,net_pay,...`.
- [ ] Exact-grant RBAC: `payroll:read` can view but not calculate; a member gets `403` on admin endpoints.

---

## 3. Module 6 — Recruitment API (`/api/v1/recruitment`)

Pipeline stages run `applied → screening → interview → offer → hired`. A candidate can leave any non-final stage via `rejected` or `withdrawn`.
- `offer` is entered only by creating an offer.
- `hired` is entered only by the hire endpoint.
- Final stages never change.

Job statuses: `draft → open ⇄ on_hold`, and any non-final status → `closed`. `filled` is set automatically when `hired_count` reaches `headcount`.

| # | Method & path | Who | Notes |
|---|---|---|---|
| R1 | POST `/jobs` | manage | `{title, headcount 1–1000, employment_type, description, department_id?, designation_id?, location?, min_experience_years?}` → `draft`. An unknown department or designation returns `400`. |
| R2–R4 | GET `/jobs?status&department_id`, GET `/jobs/{id}`, PATCH `/jobs/{id}` | read / manage | PATCH on a `closed`/`filled` job returns `409 JOB_STATE`. |
| R5 | POST `/jobs/{id}/status` `{status: open\|on_hold\|closed}` | manage | Illegal moves return `409 JOB_STATE`. |
| R6 | POST `/candidates` | manage | `{job_id, first_name, last_name, email, source: career_site\|referral\|agency\|linkedin\|other, phone?, resume_url?, expected_ctc?, notice_period_days?}`. The job must be `open` (`409 JOB_NOT_OPEN`). The email is unique per job, ignoring case (`409 CONFLICT`). |
| R7 | GET `/candidates?job_id&stage` | read | |
| R8 | GET `/candidates/{id}` | read | Includes `history` (from/to stage, actor, note, time). |
| R9 | PATCH `/candidates/{id}` | manage | Contact fields only. |
| R10 | POST `/candidates/{id}/stage` `{stage: screening\|interview\|rejected\|withdrawn, note?}` | manage | Anything off the graph returns `409 INVALID_STAGE`. Leaving `offer` withdraws the open offer. |
| R11 | POST `/candidates/{id}/hire` `{employee_code, employment_type?}` | **hire** | Needs stage `offer` and an accepted offer (`409 NOT_HIREABLE`). Returns `{candidate_id, user_id, employee_id, job_status}`. The user is created as `invited`. |
| R12 | POST `/interviews` | manage | `{candidate_id, round_name, interviewer_employee_id, scheduled_at (future, RFC 3339), duration_mins 15–480, meeting_link?}`. The candidate must be in `interview`; the interviewer must be a working employee. |
| R13 | GET `/interviews?candidate_id=` | read | `candidate_id` is required. |
| R14 | GET `/interviews/me?status` | self | Interviews where I am the interviewer. Without an employee profile: `404 EMPLOYEE_NOT_FOUND`. |
| R15 | POST `/interviews/{id}/feedback` `{rating 1–5, recommendation: strong_hire\|hire\|hold\|reject, notes?}` | self | Only the assigned interviewer (others get `404`). Only once (`409 INTERVIEW_STATE`). |
| R16 | POST `/interviews/{id}/cancel` | manage | |
| R17 | POST `/offers` `{candidate_id, offered_ctc, joining_date (YYYY-MM-DD, ≥ today), expires_on?}` | manage | The candidate must be in `interview` or `offer`. One open or accepted offer at a time (`409 OFFER_EXISTS`). |
| R18 | GET `/offers?candidate_id=` | read | |
| R19 | POST `/offers/{id}/decision` `{decision: accepted\|declined}` | manage | Recorded on the candidate's behalf. Accepting after `expires_on` returns `409 OFFER_STATE`. |
| R20 | POST `/offers/{id}/withdraw` | manage | |

### Recruitment checklist
- [ ] Full journey for a headcount-1 job:
  1. create → open;
  2. add a candidate → screening → interview;
  3. schedule an interview with a member as the interviewer;
  4. the member sees it in "my interviews" and submits feedback;
  5. create an offer → accept → hire.
  - Expected: the job shows `filled`, the new employee appears in `/employees`, and adding another candidate gives `JOB_NOT_OPEN`.
- [ ] A duplicate email in different case on the same job gives `409`; the same email on a different job is fine.
- [ ] Trying `applied → interview` directly shows `INVALID_STAGE`. The pipeline board should only offer legal moves.
- [ ] Offers:
  - a second open offer shows `OFFER_EXISTS`;
  - after declining, a new offer is allowed;
  - hiring with only a declined offer shows `NOT_HIREABLE`.
- [ ] The candidate detail page shows the stage history in order, with the actor.
- [ ] Exact-grant RBAC:
  - `recruitment:read` can view but not move;
  - `recruitment:manage` can move but **cannot hire**, which needs `recruitment:hire`;
  - a member gets `403` on all admin endpoints but `200` on `/interviews/me`.
- [ ] Another tenant's job or candidate id returns `404`.

---

## 4. Known limitations (expected, not bugs)

**Payroll:**
- No employer PF/ESI contributions.
- One PT slab.
- New tax regime only, with no marginal relief.
- A mid-month CTC change is not split.

See the D5 decisions.

**Recruitment:**
- No public careers page or candidate self-apply.
- Resumes are URLs only.
- No emails (Module 7).
- No calendar integration.
- Hire does not assign payroll CTC; HR does that in Module 5 (D6-06).
- If the candidate's email already belongs to a user in the tenant, hire stops with `409` before creating anything (D6-09).

## 5. Where things live
- Code: `backend/internal/payroll`, `backend/internal/recruitment`; wiring in `backend/internal/app/time_modules.go`.
- Tests: `backend/internal/payroll/calc` (statutory goldens), `backend/internal/recruitment/services/golden_test.go`, `backend/tests/integration/{payroll,recruitment}_test.go`.
- Docs: `docs/modules/module-5-payroll/`, `docs/modules/module-6-recruitment/`.
