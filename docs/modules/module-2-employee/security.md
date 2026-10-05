# Module 2 — Security Specification

## 1. Multi-Tenant Data Isolation
- Every table (`employee_profiles`, `employment_details`, `employee_contacts`, `employee_statutory`, `employee_documents`, `employee_timelines`, `employee_events_outbox`) includes `tenant_id` UUID FK.
- Repositories inject `tenant_id` into all queries. Cross-tenant access returns `404 Not Found`.

## 2. RBAC Permissions Matrix
- `employee:read`: View basic directory and profile info (masked statutory).
- `employee:create`: Onboard new employees.
- `employee:update`: Modify employee records, employment terms, and status.
- `employee:read_sensitive`: View unmasked bank account and tax IDs.
- `employee:update_sensitive`: Modify bank details and statutory identifiers.
- `employee:admin`: Verify documents and force status modifications.

## 3. PII & Data Privacy
- Tax IDs and bank account numbers are stored in `employee_statutory` and sanitized on general responses.
- Self-service endpoints (`/api/v1/employees/me`) validate `userID` directly from verified JWT claims.
