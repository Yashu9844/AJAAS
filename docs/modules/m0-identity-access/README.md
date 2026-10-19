# M0 Identity & Access

Platform foundation: tenants, user identities and credentials, sessions and tokens, roles and permissions, security
audit. Every other module depends on it; it depends on no other JAAS module. Architecture authority:
`docs/ARCHITECTURE.md` §5 "M0".

**Status (2026-10-20):** built and hardened (29 routes, 13 tables, integration-tested). Gaps against the adopted
target architecture are listed in `plan.md` §3 and scheduled as restructure Phase B.

## What it does today

| Area | Capability |
|---|---|
| Tenants | platform-key-guarded create (with first `tenant_admin`), list, get, update, activate, suspend; subdomain resolution |
| Authentication | login (failure-only rate limit), refresh-token rotation with reuse detection, logout revoking session + refresh, forgot/reset password (1 h single-use tokens), `GET /auth/me` |
| Users | create, list, get, update, activate, deactivate, assign/unassign roles, invite (used by the HR hire saga) |
| Roles & permissions | role CRUD, grant/revoke permissions with anti-escalation, permission catalogue (seeded idempotently), `tenant_admin` bypass |
| Audit | immutable audit log for mutations, `GET /audit-logs` |
| Security | JWT access 15 min (HS256), refresh 7 days, bcrypt 12, uniform error envelope, security headers, body limit, CORS allowlist, trusted-proxy-aware client IPs |

## Files in this bundle

| File | Holds |
|---|---|
| `specification.md` | requirements, business rules, API contracts, events, edge cases, acceptance criteria (design-time contract) |
| `design.md` | system architecture, workflows, security, component/sequence diagrams, DBML schema |
| `testing.md` | golden tests and their integration-suite locations, test strategy, the 2026-10-06 hardening report |
| `plan.md` | status, gaps against the target architecture, Phase B steps |
| `decisions.md` | decision log and changelog |
| `model/` | owner-supplied logical DB model, ER diagram, service diagram (verbatim) |

Code: `backend/internal/identity/` (controllers, dto, events, middleware, models, repositories, routes, services,
validators, `module.go`); migrations `000001`–`000012`, `000025`. Generated inventory: `docs/SYSTEM_REFERENCE.md`.
