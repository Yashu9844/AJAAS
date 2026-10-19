# HR / Recruitment (ATS) — Design

> Part of **M1 HR Management** (bounded context `recruitment`). Formerly the stand-alone "module-6-recruitment" bundle; module numbers in older text were renamed to the canonical M0–M12 / HR-context names.

## Architecture

```text
Gin → TenantResolver → Authenticate → RequirePermission(recruitment:read|manage|hire)   (interviewer endpoints: auth only)
→ Controllers (job, candidate, interview, offer)
→ Services (JobService, CandidateService, InterviewService, OfferService, HireService, OutboxRelay)
→ pipeline (pure: stage graph RC-002, job status graph FR-JB003)
→ Repositories (job, candidate + stage events, interview, offer, outbox)
→ PostgreSQL · RabbitMQ (outbox) · M0 Identity Audit + UserService.InviteUser · HR/Organization Department/Designation · HR/Employee EmployeeService
```

### Hire flow (RC-010, D6-04)
1. tx: lock the candidate and job rows; check RC-007/RC-008; if `hired_user_id` is empty → `InviteUser(tx)` and store it; commit.
2. HR/Employee `CreateEmployee(user_id, code, names, employment_type, joining_date)` in its own transaction; if the user already has a profile, load it by user id.
3. tx: candidate → `hired` (+ stage event, `hired_employee_id`), job `hired_count + 1` (→ `filled` at headcount), outbox `candidate_hired`; commit.

A failure after step 1 leaves an invited user linked to the candidate; a retry completes steps 2–3 without a second invite.

## Connections

| # | Source → Destination | Contract | Failure |
|---|---|---|---|
| C1 | Client → HR/Recruitment (20 ops) | spec §5; JWT + tenant; recruitment:* RBAC | 400–409 |
| C2 | → M0 Identity UserService | `InviteUser(ctx, tx, tenant, {email, first_name, last_name}, corr)` | 409 CONFLICT if the email is taken |
| C3 | → M0 Identity Audit / middleware | Log after commit; RequirePermission(db, "recruitment", action, …) | best-effort / 401/403 |
| C4 | → HR/Organization Department/Designation services | `GetDepartment`, `GetDesignation` via new read accessors (D6-03) | 404 |
| C5 | → HR/Employee EmployeeService | `GetByID`, `GetByUserID`, `CreateEmployee` | 404 / 409 |
| C6 | → PostgreSQL | migrations 000045–000050 | rollback |
| C7 | → RabbitMQ `jaas.recruitment.events` | ids and stages only | outbox + relay |
| C8 | boot → M0 Identity permissions | recruitment:read, recruitment:manage, recruitment:hire | boot fails |

## Security

Permissions: `recruitment:read`, `recruitment:manage`, `recruitment:hire` (hire creates users and employees — grant narrowly). Interviewer self endpoints need authentication only and resolve the employee from the JWT.

| ID | Threat | Control |
|---|---|---|
| RS-T1 | Candidate PII leakage | no name/email/phone/CTC/feedback in events or audit (RC-011) |
| RS-T2 | Feedback forgery | only the assigned interviewer (JWT → employee) can submit; others 404 (RC-005) |
| RS-T3 | Pipeline skipping | stage graph RC-002 + stage events (RC-003) |
| RS-T4 | Over-hiring | headcount check under the job row lock (RC-008) |
| RS-T5 | Privilege via hire | invited users get no roles; hire needs recruitment:hire |
| RS-T6 | Cross-tenant IDOR | tenant-scoped repos; 404 |
| RS-T7 | Duplicate hires on retry | RC-010 idempotency |
