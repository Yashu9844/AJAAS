# Module 0 — Golden Tests (G0-1 … G0-8)

Never edit to make a red test pass (MASTER_PROMPT §15). Implemented in `backend/tests/integration`; see `docs/dev-test/GOLDEN_MAP.md`.

| ID | Golden behaviour | Expected |
|---|---|---|
| G0-1 | Tenant isolation | JWT of tenant A on host B → 403; foreign ids → 404; no cross-tenant list leak |
| G0-2 | RBAC enforcement | Missing permission → 403; grant/revoke effective immediately; tenant_admin bypass |
| G0-3 | Platform routes | `/tenants*` need `X-Platform-Key` (401/403); 503 when unconfigured |
| G0-4 | Token lifecycle | Refresh rotates; reuse of a rotated token revokes all sessions; logout revokes access + refresh |
| G0-5 | No privilege escalation | Non-admins cannot assign tenant_admin or hand out permissions they do not hold |
| G0-6 | Secret hygiene | No password/token hash in any payload |
| G0-7 | Error envelope | `{error:{code,message,details}}` everywhere |
| G0-8 | Audit | Successful mutations audited (readable via `GET /audit-logs`), failures not |
