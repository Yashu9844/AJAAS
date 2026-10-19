# HR / Recruitment (ATS) — Testing

> Part of **M1 HR Management** (bounded context `recruitment`). Formerly the stand-alone "module-6-recruitment" bundle; module numbers in older text were renamed to the canonical M0–M12 / HR-context names.

## Golden tests (protected)

| # | Behaviour | Expected | Location |
|---|---|---|---|
| G1 | Isolation | foreign ids 404; foreign JWT 403 | integration |
| G2 | Stage graph | every allowed/forbidden RC-002 move | pipeline unit |
| G3 | Job status graph | FR-JB003 table | pipeline unit |
| G4 | Closed job | add candidate → JOB_NOT_OPEN | service |
| G5 | Duplicate email per job (case-insensitive) | 409; another job OK | service + integration |
| G6 | Stage history | each move adds an event with actor | service + integration |
| G7 | Interview gate | screening candidate → INVALID_STAGE; past time → 400 | service |
| G8 | Feedback ownership | other employee 404; completed twice 409 | service + integration |
| G9 | One open offer | second offer 409; after decline allowed | service |
| G10 | Hire | invite + employee + hired + count; filled at headcount | service + integration |
| G11 | Hire idempotency | failure after invite → retry uses the same user, one profile | service |
| G12 | Privacy | no PII in events/audit | integration |
| G13 | RBAC | member 403; interviewer self works | integration |
| G14 | Pagination | clamped | controllers + integration |

## Test strategy and commands

RED first. Pure pipeline graphs 100% (G2, G3); services ≥ 90% (G4–G11 on fakes); controllers/routes 100%; integration G1, G5, G6, G8, G10, G12–G14; migration cycle + parity.

```bash
go test -count=1 -cover ./internal/recruitment/...
JAAS_REQUIRE_INTEGRATION=1 go test -count=1 -run Recruitment ./tests/integration/
```

## Delivery evidence (status and Definition of Done at backend close)

- P1: design frozen — 20 ops, RC-001..RC-011, 6 tables, 4 events, goldens G1–G14, decisions D6-01..D6-06.
- P2: 6 models (jobs, candidates, stage events, interviews, offers, outbox) 100%; pipeline graphs G2/G3 100%; migrations 000045–000050 cycle-verified; AutoMigrate↔SQL identical.
- P3: DTOs + validators 100%.
- P4: repositories (row locks on job/candidate for hire and stage changes) — compile-verified.
- P5: services (jobs, candidates, interviews, offers, 3-step hire saga), events (no PII), relay; goldens G4–G12 unit green; services 99.4%.
- P6: HTTP edge (20 ops, RBAC contract test), module + app wiring (coreModules incl. org), permissions seeded, relay started, swagger 14 paths / 20 ops.
- P7/P9: integration (2 tests; suite 63/63), migration cycle + parity, quality checker clean; backend CLOSED. Frontend P8 BLOCKED (identity shell).

## Manual verification checklist

From the partner frontend test guide (2026-10-09). Paths are the pre-restructure ones; after Phase C they are prefixed `/api/v1/hr`. These become the context's test-console scenarios in Phase D.

### 3. HR/Recruitment — Recruitment API (`/api/v1/recruitment`)

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

#### Recruitment checklist
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
