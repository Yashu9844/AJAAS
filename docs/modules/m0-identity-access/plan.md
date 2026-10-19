# M0 Identity & Access — Plan

Updated 2026-10-20.

## 1. Status

Backend complete and hardened (partner branch `fix/module-0-2-hardening`, merged to `main`): 29 routes, 13 tables,
8 goldens (G0-1 … G0-8) green in `backend/tests/integration`. Console page exists (`/dev-test/module-0`, to be renamed
in Phase D).

## 2. Does M0 need changes for the new architecture? — Yes

The review compared the code with `docs/ARCHITECTURE.md` §3 (universal rules), §4 (ownership matrix) and §5 "M0".
Nothing in M0 is wrong for a single-process system, but the target (independently deployable services, durable
events, idempotent commands) needs the changes below. Items marked **gate** change schema or public API.

## 3. Gaps and Phase B steps

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

Order: B7 → B6 → B1 → B2 → B3 → B5 → B4 → B10 (B10 runs with HR Phase C step 7). Each step: RED tests first,
integration ring green, console page updated, docs updated, one commit.

## 4. Later (not in Phase B)

- MFA (TOTP) — needs a product decision; table exists.
- SSO / SAML / OIDC for enterprise tenants.
- Tenant lifecycle events consumed by every service for deactivation (§5 "M0" consistency note).
