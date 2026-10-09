# Module 6 — Security

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
