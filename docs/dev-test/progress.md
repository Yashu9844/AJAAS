# /dev-test verification console — running notes (resume file)

Task: build temporary Module 0–2 verification UI at `/dev-test` (frontend/src/app/dev-test, frontend/src/modules/devtest), run it against the real backend, produce final report `docs/dev-test/VERIFICATION_REPORT.md`.

## Environment (how to resume)
- Infra: `docker compose up -d postgres redis` (rabbit not running → NoOp publisher).
- Backend: `cd backend; DATABASE_PASSWORD=postgres DATABASE_DBNAME=jaas_identity RABBITMQ_HOST= go run cmd/main.go` (port 8080). Log: /tmp/jaas_backend.log
- Frontend: `cd frontend; npx next dev -p 3000` → http://localhost:3000/dev-test
- Seed admin (no API can create 1st user): `bash scripts/dev-test-bootstrap-admin.sh <slug>` (admin@<slug>.com / Secret123!) — also seeds the 16 permissions (backend never seeds them).
- Tenant-scoped API calls need Host `<slug>.localhost:8080` (TenantResolver parses subdomain).
- Login rate limit 10/15min per IP; flush with `docker exec ajass-postgres-1 true; docker exec ajass-redis-1 redis-cli FLUSHALL` if 429.
- Existing tenant `probeco` seeded. Need a 2nd tenant (e.g. `probeco2`) + admin for isolation scenario (session B).

## Status
- [x] Phase 1/2 inventory (see report once written)
- [x] Core console: types/store/api/runner/specs/components/auth/shell/modules, scenarios0 (6 scenarios)
- [x] scenarios1.ts + scenarios2.ts written (all 3 modules have scenarios)
- [x] Run all scenarios, tsc + eslint + next build OK
- [x] Final report docs/dev-test/VERIFICATION_REPORT.md written. TASK COMPLETE (browser driver saved at docs/dev-test/browser-driver.js)

## Backend findings so far (do NOT patch backend; report only)
1. POST /roles with unknown (valid uuid) permission_id → HTTP 500 INTERNAL_ERROR (expected 400/404/422).
2. /api/v1/tenants (POST/GET/PATCH/suspend/activate) have no auth: anonymous can create/list tenants (spec NFR-SEC005 says 401).
3. UserResponse.roles is always `null` on POST/GET/list /users even though user_roles rows ARE persisted (DB verified).
4. Validation errors from some controllers use a bare string `{"error":"Key: 'X.Y' Error:Field validation…"}` (login, users create, tenants, "Invalid user ID") instead of the `{error:{code,message,details}}` envelope.
5. No API to create first user/admin of a tenant; no permission seed; no create-permission API; no audit-log read API; no "activate user" API; no unassign role/permission API.
6. Deactivated-user login → 403 ACCOUNT_NOT_ACTIVE (ok, noted).

7. POST /auth/logout ALWAYS 401 "Unauthorized context mapping": controller needs tenant_id but route has no tenantResolver and Authenticate never sets it → logout/session revocation is non-functional; refresh token still works after "logout".
8. Role responses never include `permissions` (RoleResponse.Permissions not preloaded) although role_permissions rows exist → no API shows a role's/user's permissions.
9. POST /roles with unknown permission → 500 AND the role row is still created (partial write, no rollback).
10. Duplicate department code / team code / designation code → 500 INTERNAL_ERROR (should be 409).
11. Department deactivate counts INACTIVE teams (CountByDepartment) → cannot deactivate a dept whose only team is inactive (409 "active teams or mappings") without force.
12. POST /mappings/:id/deactivate without a JSON body → 400 "EOF" (departments/teams/designations accept no body).
13. GET /org-chart cached 60s per (tenant,max_depth,include_inactive) with NO invalidation → stale reads after writes (verified: created dept absent from warm key).
14. Dept DTO `path` = ancestors only (excludes self); depth 0 for roots.
15. Rate limiter: 10 logins/15min counts failed attempts → heavy testing hits 429; flush redis.
16. Employee: spec rules likely missing — see s2 results (transition matrix FR-ED002, exit→user deactivation FR-ED005, ?unmasked=true bypass of PII masking for any employee:read user).

## Console state
- Module 0 scenarios run: errors(3 real fails), tenant PASS, user(roles null), role(perms not returned), rbac PASS, session(logout broken), isolation PASS.
- Module 1 scenarios run: dept/team/desig/map fails are backend bugs 10-12; chart scenario rewritten for cache; rbac+errors PASS.
- NEXT: run Module 2 scenarios, fix console issues, lint/tsc/next build, manual UI smoke of form cards (Create tenant→fetch created record), write VERIFICATION_REPORT.md.
- Tenants probeco (A) and probeco2 (B) exist w/ admin@<slug>.com / Secret123!.
- Driver: `await import('/dev-test-driver.js'); await window.runAll('Scenarios')` in browser JS (frontend/public/dev-test-driver.js).
