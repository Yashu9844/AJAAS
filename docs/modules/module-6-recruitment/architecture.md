# Module 6 — Architecture (FROZEN at P1 2026-10-09)

```text
Gin → TenantResolver → Authenticate → RequirePermission(recruitment:read|manage|hire)   (interviewer endpoints: auth only)
→ Controllers (job, candidate, interview, offer)
→ Services (JobService, CandidateService, InterviewService, OfferService, HireService, OutboxRelay)
→ pipeline (pure: stage graph RC-002, job status graph FR-JB003)
→ Repositories (job, candidate + stage events, interview, offer, outbox)
→ PostgreSQL · RabbitMQ (outbox) · Module 0 Audit + UserService.InviteUser · Module 1 Department/Designation · Module 2 EmployeeService
```

## Hire flow (RC-010, D6-04)
1. tx: lock the candidate and job rows; check RC-007/RC-008; if `hired_user_id` is empty → `InviteUser(tx)` and store it; commit.
2. Module 2 `CreateEmployee(user_id, code, names, employment_type, joining_date)` in its own transaction; if the user already has a profile, load it by user id.
3. tx: candidate → `hired` (+ stage event, `hired_employee_id`), job `hired_count + 1` (→ `filled` at headcount), outbox `candidate_hired`; commit.

A failure after step 1 leaves an invited user linked to the candidate; a retry completes steps 2–3 without a second invite.
