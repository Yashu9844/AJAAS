# Module 2 — Golden Tests (Candidates G1..G10)

Status: FROZEN 2026-10-05 (plan.md P1).

| Test ID | Name | Scenario | Expected Outcome |
|---|---|---|---|
| G1 | Multi-Tenant Isolation | Tenant B attempts GET /employees/{id_A} | 404 Not Found (no cross-tenant leakage) |
| G2 | Atomic Onboarding | Create employee with valid user_id and profile | 201 Created; Profile, Employment, Contact, Timeline all exist in DB |
| G3 | Duplicate Employee Code | Attempt to create employee with existing employee_code | 409 Conflict |
| G4 | Non-existent / Wrong Tenant User | Create employee with user_id belonging to another tenant | 404 User not found in tenant |
| G5 | Self-Service Guard | Employee updates their own address via /employees/me | 200 OK; address updated; status and code untouched |
| G6 | Privilege Escalation Block | Non-admin user attempts PATCH /employees/{id} to change status | 403 Forbidden |
| G7 | Sensitive Statutory Privacy | Regular employee:read gets profile | Bank account and Tax ID are masked |
| G8 | Sensitive Statutory Access | Admin with employee:read_sensitive queries statutory | 200 OK with unmasked bank/tax details |
| G9 | Status Transition Validation | Transition employee from probation -> active | 200 OK; confirmation_date recorded; timeline updated |
| G10 | Reactive Deactivation Sync | Module 0 emits identity.user.deactivated | Employee status transitions to terminated/inactive; timeline recorded |
