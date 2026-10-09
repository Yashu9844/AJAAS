# Module 6 — Specification (FROZEN at P1 2026-10-09 — contract for P2+)

Scope per masterplan (D6-01): Recruitment & Applicant Tracking. Depends on Module 0 (tenant, auth, RBAC, audit, user invite), Module 1 (departments, designations — read), Module 2 (interviewers, employee creation on hire). Consumers: Modules 7, 10, 11 (events).

## 1. Purpose
Publish job openings, track candidates through a hiring pipeline with an auditable stage history, schedule interviews and collect interviewer feedback, make and record offers, and convert an accepted candidate into an invited user + employee profile.

## 2. Glossary
- **Pipeline stages** — `applied → screening → interview → offer → hired`; terminal exits `rejected`, `withdrawn`.
- **Stage event** — append-only record of every stage change (who, when, from, to, note).
- **Hire** — creates a Module 0 user via **invite** (no password handled here) and a Module 2 employee profile.

## 3. Functional requirements
### Jobs (FR-JB)
- FR-JB001 Create {title, department_id?, designation_id?, headcount 1–1000, location?, employment_type `full_time|part_time|contract|intern`, min_experience_years 0–50, description} → status `draft`. Department/designation validated in Module 1.
- FR-JB002 List (filters status, department_id) / get / update (any field while not `closed`/`filled`).
- FR-JB003 Status transitions: draft→open; open↔on_hold; draft|open|on_hold→closed; `filled` only automatically when hired_count reaches headcount (RC-008).
### Candidates (FR-CD)
- FR-CD001 Add a candidate to an **open** job {first_name, last_name, email, phone?, source `career_site|referral|agency|linkedin|other`, resume_url?, expected_ctc?, notice_period_days?}; email unique per job (case-insensitive) → stage `applied` + stage event.
- FR-CD002 List (job_id, stage filters) / get (with stage history) / update contact fields.
- FR-CD003 Move stage (manual): only along RC-002; `offer` is entered by creating an offer, `hired` only by hire.
### Interviews (FR-IV)
- FR-IV001 Schedule {candidate_id, round_name, interviewer_employee_id, scheduled_at (future), duration_mins 15–480, meeting_link?} for a candidate in `interview`; interviewer must be a working employee (Module 2).
- FR-IV002 Interviewer self-service: list my interviews; submit feedback {rating 1–5, recommendation `strong_hire|hire|hold|reject`, notes} on my own scheduled interview → `completed`.
- FR-IV003 Cancel (manage); list by candidate (read).
### Offers (FR-OF)
- FR-OF001 Create {candidate_id, offered_ctc > 0, joining_date ≥ today, expires_on?} for a candidate in `interview` or `offer` with no open offer → status `offered`; candidate moves to `offer`.
- FR-OF002 Record decision `accepted|declined` (manage, on the candidate's behalf) or withdraw; list by candidate.
### Hire (FR-HR)
- FR-HR001 Hire {employee_code, employment_type?} for a candidate in `offer` with an `accepted` offer: (1) invite user (email, names) unless already invited for this candidate; (2) create employee profile with joining_date = offer joining date; (3) candidate → `hired`, job hired_count + 1, job `filled` when full. Idempotent on retry (RC-010).
### Events (FR-EV)
- FR-EV001 Exchange `jaas.recruitment.events`: `recruitment.job_published`, `recruitment.candidate_applied`, `recruitment.interview_scheduled`, `recruitment.candidate_hired`; outbox in-tx; relay.
- FR-EV002 Payloads carry ids, stages and job/period data only — never candidate name/email/phone, CTC or feedback.

## 4. Business rules (RC)
| ID | Rule |
|---|---|
| RC-001 | Candidates can be added only to `open` jobs (409 `JOB_NOT_OPEN`). |
| RC-002 | Manual stage moves: applied→screening\|rejected\|withdrawn; screening→interview\|rejected\|withdrawn; interview→rejected\|withdrawn; offer→rejected\|withdrawn. Anything else → 409 `INVALID_STAGE`. Terminal stages are final. |
| RC-003 | Every stage change writes a stage event in the same transaction. |
| RC-004 | Interviews only for candidates in `interview` (409 `INVALID_STAGE`); scheduled_at in the future (400). |
| RC-005 | Only the assigned interviewer submits feedback (others 404); only `scheduled` interviews (409 `INTERVIEW_STATE`). |
| RC-006 | One open (`offered`) offer per candidate (409 `OFFER_EXISTS`); withdrawing/declining frees it. |
| RC-007 | Hire requires stage `offer` and an `accepted` offer (409 `NOT_HIREABLE`). |
| RC-008 | Hire increments hired_count; at headcount the job becomes `filled`; hiring into a `filled`/`closed` job → 409 `JOB_NOT_OPEN`. |
| RC-009 | Job/candidate/interview/offer ids are tenant-scoped; foreign ids → 404. |
| RC-010 | Hire is idempotent: the invited user id is stored on the candidate before the employee is created; a retry reuses it and an existing profile for that user. |
| RC-011 | Candidate PII and feedback never appear in events or audit metadata. |

## 5. REST API (base `/api/v1`)
| # | Method & path | Permission |
|---|---|---|
| R1 | POST `/recruitment/jobs` | `recruitment:manage` |
| R2 | GET `/recruitment/jobs?status&department_id&page&per_page` | `recruitment:read` |
| R3 | GET `/recruitment/jobs/{id}` | `recruitment:read` |
| R4 | PATCH `/recruitment/jobs/{id}` | `recruitment:manage` |
| R5 | POST `/recruitment/jobs/{id}/status` `{status}` | `recruitment:manage` |
| R6 | POST `/recruitment/candidates` | `recruitment:manage` |
| R7 | GET `/recruitment/candidates?job_id&stage&page&per_page` | `recruitment:read` |
| R8 | GET `/recruitment/candidates/{id}` (with stage history) | `recruitment:read` |
| R9 | PATCH `/recruitment/candidates/{id}` | `recruitment:manage` |
| R10 | POST `/recruitment/candidates/{id}/stage` `{stage, note?}` | `recruitment:manage` |
| R11 | POST `/recruitment/candidates/{id}/hire` `{employee_code, employment_type?}` | `recruitment:hire` |
| R12 | POST `/recruitment/interviews` | `recruitment:manage` |
| R13 | GET `/recruitment/interviews?candidate_id&page&per_page` | `recruitment:read` |
| R14 | GET `/recruitment/interviews/me?status&page&per_page` | self (interviewer) |
| R15 | POST `/recruitment/interviews/{id}/feedback` | self (assigned interviewer) |
| R16 | POST `/recruitment/interviews/{id}/cancel` | `recruitment:manage` |
| R17 | POST `/recruitment/offers` | `recruitment:manage` |
| R18 | GET `/recruitment/offers?candidate_id&page&per_page` | `recruitment:read` |
| R19 | POST `/recruitment/offers/{id}/decision` `{decision: accepted\|declined}` | `recruitment:manage` |
| R20 | POST `/recruitment/offers/{id}/withdraw` | `recruitment:manage` |

## 6. Errors
400 VALIDATION_ERROR · 403 FORBIDDEN · 404 NOT_FOUND, EMPLOYEE_NOT_FOUND · 409 CONFLICT (duplicate email per job, employee code taken), JOB_NOT_OPEN, JOB_STATE, INVALID_STAGE, INTERVIEW_STATE, OFFER_EXISTS, OFFER_STATE, NOT_HIREABLE · 500 opaque.

## 7. Non-functional
NFR-SEC001 tenant scoping; NFR-SEC002 interviewer endpoints resolve the employee from JWT; NFR-SEC003 PII/feedback out of events + audit; NFR-D001 stage + event + outbox in one tx.

## 8. Data (migrations 000045–000050)
`recruitment_jobs` · `recruitment_candidates` (unique (job_id, lower(email))) · `recruitment_stage_events` (append-only) · `recruitment_interviews` · `recruitment_offers` (partial unique open offer per candidate) · `recruitment_events_outbox`.

## 9. Edge cases
EC-01 candidate for a draft job → 409. EC-02 duplicate email (any case) on the same job → 409; same email on another job allowed. EC-03 applied→interview → 409. EC-04 interview scheduled for a screening candidate → 409. EC-05 non-assigned employee posts feedback → 404. EC-06 second open offer → 409. EC-07 hire with declined offer → 409. EC-08 hire retry after employee step failure → completes without a second invite. EC-09 hire fills the last seat → job filled; next hire for that job → 409. EC-10 cross-tenant ids → 404.

## 10. Out of scope (v1)
Public careers portal / candidate self-apply, resume upload & parsing (URL only), email notifications (Module 7), calendar integration, scorecards per competency, multi-step offer approvals, automatic payroll assignment on hire (HR assigns CTC in Module 5 after hire).
