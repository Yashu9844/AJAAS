# M0 Identity & Access — Plan

Updated 2026-10-20.

## 1. Status

Backend complete and hardened (partner branch `fix/module-0-2-hardening`, merged to `main`): 29 routes, 13 tables,
8 goldens (G0-1 … G0-8) green in `backend/tests/integration`. Console page `/dev-test/identity` (10 scenarios incl. access model,
all passing 2026-10-20).

## 2. Does M0 need changes for the new architecture? — Yes

The review compared the code with `docs/ARCHITECTURE.md` §3 (universal rules), §4 (ownership matrix) and §5 "M0".
Nothing in M0 is wrong for a single-process system, but the target (independently deployable services, durable
events, idempotent commands) needs the changes below. Items marked **gate** change schema or public API.

## 3. B0 — access model (owner-approved 2026-10-20, `docs/ARCHITECTURE.md` §17)

**Status 2026-10-20: built.** Unit tests green (access 100%, services 94.1%, middleware 91%); integration suite green incl. 6 new TestAccess* goldens; migration 000057 up/down/up clean and AutoMigrate-parity identical; swagger updated; console Access tab added (tsc/eslint/next build clean). Open: console scenarios not yet run live in the browser; automatic-role sweep runs every 5 min plus at login and on demand (not 60 s).

Today: flat roles holding permissions; `tenant_admin` bypass; no scopes, no inheritance, no per-user exceptions, no
predefined roles. Target design:

**Schema (migration 000057, owner GO given 2026-10-20):**

| Table | Change |
|---|---|
| `permissions` | + `module` (catalogue grouping), + `is_sensitive` |
| `roles` | + `parent_role_id` (same tenant, acyclic, depth ≤ 10), + `template_key` (unique per tenant when set), + `is_automatic`, + `template_managed` (true until a tenant edits the role) |
| `role_permissions` | + `scope` (`own`, `direct_reports`, `all_reports`, `department`, `tenant`; default `tenant`) |
| `user_roles` | + `source` (`manual` or `automatic`) |
| `user_permission_grants` (new) | tenant, user, permission, scope, `expires_at`, reason, granted_by; one row per user + permission |
| `user_permission_blocks` (new) | tenant, user, permission, reason, created_by; one row per user + permission |
| `tenants` | + `access_version` (bumped in the same transaction as any access write; part of the cache key) |

**Behaviour:**

- Effective access = assigned roles (manual + automatic) with their ancestors' grants ∪ unexpired user grants, minus
  blocks; widest scope wins per permission. Cached in Redis per user under the tenant's `access_version` (5 min TTL);
  any access write bumps the version, so changes apply on the next request.
- Catalogue registration: services register permission definitions (module, sensitive flag, and default scopes per
  predefined role) through `identity.Module.RegisterCatalogue`; M0's own definitions use the same call. HR modules
  switch from writing the `permissions` table to this call.
- Predefined roles per tenant (created at tenant creation and back-filled at boot): executive ⊃ hr_admin ⊃
  hr_manager ⊃ manager ⊃ employee; side roles payroll_admin, payroll_approver, finance, recruiter, interviewer,
  it_admin, auditor (parent employee). Boot sync adds newly registered default grants only to roles still
  `template_managed`; the first tenant edit sets it false and the role is never touched again.
- Automatic roles `employee` and `manager` (source `automatic`) cannot be assigned or removed by hand. A reconciler
  sets them from HR facts (active employee profile; has active direct reports) through a port implemented in
  `internal/app` — every 60 s and on demand (`POST /access/automatic-roles/sync`). Phase C replaces polling with
  HR events.
- Safe rollout: the existing `RequirePermission` requires **tenant** scope (routes written before scopes assume
  tenant-wide access), so a narrower template grant can never widen access. `RequireScopedPermission(resource,
  action, scopes…)` admits the narrower scopes a route declares and puts the caller's scope in the request context
  for the owning service to enforce; HR routes adopt it in Phase C.
- Anti-escalation with scopes: a non-admin caller can only grant (to a role or a user) a permission they hold at an
  equal or wider scope; only `tenant_admin` grants `tenant_admin`; blocks and grants are audited.
- APIs: role create/update/grant accept `grants: [{permission_id, scope}]` and `parent_role_id` (old
  `permission_ids` still accepted = tenant scope); `POST /roles/{id}/clone`; `GET/POST /users/{id}/grants`,
  `DELETE /users/{id}/grants/{permission_id}`; `GET/POST /users/{id}/blocks`, `DELETE /users/{id}/blocks/{permission_id}`;
  `GET /users/{id}/access` (explain: each effective permission, scope, sources, blocked); `GET /auth/me/access`;
  `POST /access/automatic-roles/sync`. New permissions `access:read`, `access:manage`.

## 4. Gaps and Phase B steps

| # | Gap (evidence) | Change | Gate |
|---|---|---|---|
| B1 | Events are published directly and best-effort (`_ = s.publisher.Publish(...)` in `auth_service.go`, `role_service.go`); lost if RabbitMQ is down or the process dies after commit | identity outbox table written in the same transaction, relay like HR's; versioned envelope (`event_version`, `correlation_id`, `causation_id`) from the shared `events` package | gate (schema) |
| B2 | No idempotency on retryable commands (tenant create, user create/invite, role grants) | shared `idempotency` middleware + `identity.idempotency_records` | gate (API header + schema) |
| B3 | Invitations have no token or accept flow: `InviteUser` creates an `invited` user with a random unusable password; the invitee can only get in through forgot-password | `invitations` table (hashed single-use token, expiry, role), `POST /invitations`, `POST /invitations/{token}/accept` (sets password, activates), resend/revoke; hire saga uses it | gate (schema + API) |
| B4 | Services that verify tokens must share the HS256 secret (`token_service.go`), which does not work for separately deployed services | asymmetric signing (EdDSA or RS256) with key ID and a `/.well-known/jwks.json` endpoint; rotation runbook; HS256 accepted during a transition window | gate (API) |
| B5 | Other services authorize by reading M0 tables (`RequirePermission` with M0 repositories) and seed their permissions by writing M0's `permissions` table | in-process `Authorize` + `RegisterPermissions` contract now (consumed through `hr/identityport`); permission version counter per user for cache invalidation; HTTP form later | no (internal contract) |
| B6 | Single `/health` liveness | `/health/live` (process) and `/health/ready` (Postgres, Redis; RabbitMQ not required — outbox covers it) | no |
| B7 | The request logger accepts any `X-Request-ID` value unvalidated (contexts then `uuid.Parse` it) and the architecture's `X-Correlation-ID` header is not recognised | accept `X-Correlation-ID` (fall back to `X-Request-ID`), validate as UUID or replace, echo it, and carry it into audit and events everywhere | no |
| B8 | Ownership: `tenant_settings` table belongs to M12 Settings; `mfa_configs` table exists without any MFA flow | keep both read-only until M12 / an MFA decision; document as owned-elsewhere; no new writes | no |
| B9 | Login identity is `(tenant, email)` per tenant — matches §3.5; no change. Password minimum 8, bcrypt 12, reset 1 h, access 15 min, refresh 7 days — match §3.10 | none (re-check before release) | — |
| B10 | Tables in schema `public` | move to schema `identity` together with HR's schema split (HR plan step C.7) | gate (schema) |

**Status 2026-10-20 — Phase B done except B10 (runs with HR Phase C step 7):**

| Step | Result | Commit |
|---|---|---|
| B0 | access model (scopes, inheritance, predefined + automatic roles, grants, blocks, explain) | `110107a` |
| B7 | `X-Correlation-ID` validated, echoed, carried into audit (`audit_logs.correlation_id`, migration 000058) and events | `cf8a8af` |
| B6 | `/health/live`, `/health/ready` (Postgres + Redis; RabbitMQ deliberately not required) | `cf8a8af` |
| B1 | shared `internal/shared/outbox`; identity events written in-transaction to `identity_events_outbox` (000059) and relayed; envelope `event_version`, `causation_id`; events carry no emails/names/tokens; secret-delivery port + dev mailbox (`GET /api/v1/dev/mailbox`, platform key, development/test only) | `843ebd7` |
| B2 | shared `internal/shared/idempotency`: `Idempotency-Key` on every write, DB-claimed (`idempotency_records`, 000060), replay / 409 reuse / 409 in-progress, 5xx releases, 24 h retention | `415f99a` |
| B3 | invitations (000061): `POST/GET /invitations`, resend, revoke, `POST /auth/accept-invitation`; hire saga invites through it; invite-time roles get escalation checks; login checks the password before revealing account state | `72306d0` |
| B4 | EdDSA (Ed25519) access tokens with `kid`, `/.well-known/jwks.json`; `jwt.signing_key` required in production; HS256 accepted only while `accept_legacy_hs256` is true | `23fddd9` |
| B5 | contract for other services: `identity.Module.Authorizer()` (scope resolution) and `RegisterCatalogue` (permission registration) — consumed through `hr/identityport` in Phase C | `110107a` |

Remaining limitation (documented): the idempotency middleware stores the outcome after the handler's own transaction
commits, so a crash between the two leaves the key "in progress" until it expires (24 h); high-impact HR commands
(payroll finalize, hire) are additionally idempotent by their state machines.

Original order: B0 → B7 → B6 → B1 → B2 → B3 → B5 → B4 → B10 (B10 runs with HR Phase C step 7). Each step: RED tests
first, integration ring green, console page updated, docs updated, one commit.

## 5. Later (not in Phase B)

- MFA (TOTP) — needs a product decision; table exists.
- SSO / SAML / OIDC for enterprise tenants.
- Tenant lifecycle events consumed by every service for deactivation (§5 "M0" consistency note).
