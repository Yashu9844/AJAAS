# Module 2 — Decisions Log

| Date | ID | Context | Decision | Impact |
|---|---|---|---|---|
| 2026-10-05 | DEC-E001 | 1:1 relationship between User and EmployeeProfile | Enforce unique `user_id` per tenant on `EmployeeProfile`. An employee must link to an authenticated User. | Clear identity separation while maintaining referential integrity. |
| 2026-10-05 | DEC-E002 | Sensitive PII separation | Store bank accounts and government tax IDs in a separate `employee_statutory` table with strict RBAC (`employee:read_sensitive`). | Zero leak of sensitive financial PII on general employee directory queries. |
| 2026-10-05 | DEC-E003 | Atomic Onboarding | Create profile, employment details, contact records, and initial timeline event inside a single DB transaction. | Prevents dangling or incomplete employee records. |
| 2026-10-05 | DEC-E004 | Reactive user deactivation convergence | When Module 0 user is deactivated, sync employee status to `inactive`/`terminated` and log timeline exit. | Guarantees consistency between authentication state and HR status. |
