# M1 HR Management — Design

> Service-level design. Each built context's internal design (layers, flows, locks, threats) is in
> `contexts/<context>/design.md`. "Now" describes the code at the start of the restructure; "Target" is what
> restructure Phase C delivers (`plan.md`). Universal rules: `docs/ARCHITECTURE.md` §3.

## 1. Shape of the service

One deployable HR service containing bounded contexts. Each context is a vertical slice with its own tables, rules,
events and HTTP routes, and talks to other contexts only through their published `api` package (target) — never
through their repositories, models or services.

```text
Client ─▶ Gateway ─▶ HR service (/api/v1/hr/...)
                      ├─ shared HR kernel: actor, scopes, money/days, effective dating, request lifecycle
                      ├─ organization ─┐
                      ├─ employee ─────┤  each: routes → controllers → services → engines → repositories
                      ├─ attendance ───┤        + events (outbox) + api (ports for other contexts)
                      ├─ leave ────────┤
                      ├─ payroll ──────┤
                      ├─ recruitment ──┤
                      ├─ performance ──┤
                      ├─ core, onboarding, compensation, learning, benefits (planned)
                      └─ identityport ─▶ M0 Identity & Access (authn, authz, users, audit)
                      ▼
            Postgres schema `hr` (target) · Redis (cache/limits only) · RabbitMQ (outbox relay)
```

## 2. Package layout

| | Now | Target |
|---|---|---|
| Root | `backend/internal/<context>/` for 7 contexts, wired by `internal/app/app.go` (organization, employee) and `internal/app/time_modules.go` (`buildPeopleModules` for attendance … recruitment) | `backend/internal/hr/` with `module.go` (composition root: builds every context, one `RegisterRoutes(rg)` under `/hr`, one relay supervisor) |
| Context | `models, dto, validators, repositories, services, controllers, routes, events, module.go`, plus pure engines (`calc` in attendance/leave/payroll, `score` + `lifecycle` in performance, pipeline graphs in recruitment) | same internal layers; adds `api/` (exported read ports + DTOs other contexts may use) |
| Shared HR kernel | duplicated per context (Actor, TxRunner, ParsePage, money/days types) | `backend/internal/hr/kernel/` — `Actor`, scope evaluation (self/manager/permission), effective-dating helpers, request lifecycle (HR-013), `Money`, `Days`, `Fixed` |
| Platform shared | `backend/internal/shared/` (config, database, errors, logger, cache, queue, middleware, utils) | adds `idempotency`, `outbox`, `inbox`, `events` (versioned envelope) used by M0 and HR alike |
| Identity access | contexts import `identity/middleware` and `identity/repositories` | `backend/internal/hr/identityport/` interface + in-process adapter over M0; HR packages never import `internal/identity/...` (import-boundary test) |

## 3. Context map

```text
               M0 Identity (identityport)
                       │ authn · authz · users · audit · invite
   ┌───────────────────┼──────────────────────────────────────────────┐
   │ organization ◀── core (planned: assignments drive reporting lines) │
   │      │ reporting lines            │                               │
   │      ▼                            ▼                               │
   │  performance ◀──────────── employee ◀──── recruitment (hire saga) │
   │      │ ratings                │ directory                         │
   │      ▼                        ▼                                   │
   │ compensation ──▶ payroll ◀── leave ──▶ attendance                  │
   │                    ▲  ▲         (unpaid days)   (on_leave days)   │
   │      benefits ─────┘  └──── onboarding (clearance → settlement)   │
   │      learning ◀── core events (joiners, movers)                   │
   └───────────────────────────────────────────────────────────────────┘
```

Rules: arrows point from consumer to provider for synchronous reads; state changes that other contexts react to
travel as outbox events (even inside one process) once the inbox primitive exists, so the same handlers keep working
when a context is deployed separately. No cycles in synchronous dependencies (checked by the import-boundary test).

## 4. HR ↔ M0 boundary

| Need | Now | Target |
|---|---|---|
| Tenant from host | `identity` TenantResolver middleware | `identityport.ResolveTenant` (in-process adapter; later gateway-forwarded claim) |
| Authentication | `identity` Authenticate middleware (JWT + session checks) | `identityport.Authenticate` |
| Permission check | `RequirePermission` reading M0 `user_roles` / `role_permissions` tables | `identityport.Authorize(actor, resource, action)`; adapter may cache per §3.9 (tenant + user + permission version in the key) |
| Permission catalogue | each context inserts its permissions into M0's `permissions` table at boot | `identityport.RegisterPermissions(catalogue)` — M0 owns the write |
| User provisioning (hire) | recruitment calls `UserService.InviteUser` | `identityport.InviteUser` (idempotent by operation ID) |
| Audit | M0 `AuditService.Log` writes M0 `audit_logs` | HR writes its own `hr.audit_events` (§3.10 fields); M0 keeps security audit |
| Login deactivation (termination) | not built | `identityport.DeactivateUser(effective_at)` from `core` terminate |

## 5. Data ownership

- **Target:** every HR table lives in Postgres schema `hr`; HR's migrations are the only writer of that schema. HR
  holds no foreign keys to M0 tables. Today HR migrations declare 36 `REFERENCES tenants` and 8 `REFERENCES users`
  constraints; Phase C replaces them with checked IDs (tenant validity comes from the request context and M0 tenant
  lifecycle events; user IDs are validated through `identityport` on write).
- Tables by context (now, all in `public`):

| Context | Tables |
|---|---|
| organization | `departments`, `teams`, `designations`, `mappings`, `org_events_outbox` |
| employee | `employee_profiles`, `employment_details`, `employee_contacts`, `employee_statutory`, `employee_documents`, `employee_timelines`, `employee_events_outbox` |
| attendance | `shifts`, `shift_assignments`, `attendance_records`, `attendance_punches`, `attendance_regularizations`, `attendance_events_outbox` |
| leave | `leave_types`, `leave_holidays`, `leave_balances`, `leave_requests`, `leave_ledger`, `leave_events_outbox` |
| payroll | `payroll_structures`, `payroll_components`, `payroll_assignments`, `payroll_runs`, `payroll_payslips`, `payroll_payslip_lines`, `payroll_events_outbox` |
| recruitment | `recruitment_jobs`, `recruitment_candidates`, `recruitment_stage_events`, `recruitment_interviews`, `recruitment_offers`, `recruitment_events_outbox` |
| performance | `performance_cycles`, `performance_goals`, `performance_key_results`, `performance_checkins`, `performance_reviews`, `performance_events_outbox` |

- Target additions in `hr`: `idempotency_records`, `inbox_events`, `audit_events`, `saga_operations` (one each for the
  service, per `docs/architecture/universal_db_model_and_dependencies.md`), and the per-context outboxes consolidated
  into one `outbox_events` table with a `context` column (one relay instead of seven).
- Cross-context references inside HR may use foreign keys (same owner). Context code still never reads another
  context's tables directly; it goes through that context's `api` port, so a later split needs no query rewrite.

## 6. HTTP surface

- Now: 115 HR operations under `/api/v1/{departments, teams, designations, mappings, org-chart, employees, attendance,
  shifts, leave, payroll, recruitment}` plus performance (designed, 22 operations under `/api/v1/performance`, not
  wired yet). Full list with middleware chains: `docs/SYSTEM_REFERENCE.md` Part 2 §A.
- Target: every HR operation under `/api/v1/hr/`, keeping each context's sub-paths, e.g.
  `/api/v1/hr/organization/departments`, `/api/v1/hr/employees`, `/api/v1/hr/attendance/punches`,
  `/api/v1/hr/leave/requests`, `/api/v1/hr/payroll/runs`, `/api/v1/hr/recruitment/jobs`,
  `/api/v1/hr/performance/cycles`. Exact mapping table is produced in Phase C and recorded in `decisions.md`.
  This is a breaking change accepted because nothing is in production (decision DHR-03).
- Conventions (unchanged): `{data, meta}` / `{error: {code, message, details}}`, `Idempotency-Key` on retryable
  commands (Phase C adds it everywhere listed in HR-010), `X-Correlation-ID` propagated.

## 7. Events

- Exchange per context now (`jaas.<context>.events`; organization and employee use the same pattern through their
  outbox tables). Envelope today: `{event_id, event_type, routing_key, version "1.0", occurred_at, producer,
  tenant_id, correlation_id, payload}`.
- Target envelope adds `event_version` (integer, replaces string `version`), `causation_id`, `aggregate_id`,
  `aggregate_version`; one exchange `jaas.hr.events` with routing keys `hr.<context>.<entity>.<action>`; old keys kept
  as aliases for one release.
- Payload policy (HR-009) is enforced by tests that scan every event type for forbidden fields.

## 8. Sagas and cross-context processes

| Process | Steps | Durable state |
|---|---|---|
| Hire (built, in-process) | recruitment locks job + candidate → M0 invite user → employee profile → candidate hired → event | target: `saga_operations` row with operation ID, step, status, last error; retry and repair command; never deletes employee data as compensation |
| Approved leave → attendance → payroll (built) | leave approval consumes ledger and calls `attendance` LeaveSync in the same transaction; payroll reads unpaid days at calculation | target: attendance marks via event handler (inbox) |
| Terminate → offboarding → settlement (planned) | core terminate → onboarding checklist → clearance complete → payroll final settlement → M0 deactivation on last day | saga row per termination |
| Promotion with pay change (planned) | core action → compensation record → payroll CTC from effective date | linked by action ID |
| Merit cycle apply (planned) | compensation applies cycle → records → payroll CTC updates | idempotent by cycle ID |

## 9. Concurrency and consistency patterns (in use, kept)

- Transaction per command via `TxRunner`; outbox row written in the same transaction; audit and publish after commit;
  relay retries every 30 s, batch 100, max 10 attempts.
- Advisory locks per aggregate (`leave:<tenant>:<employee>`, attendance punch per employee-day, payroll per run);
  `SELECT … FOR UPDATE` on job and candidate rows for hires and stage changes.
- Partial unique indexes for "one pending/primary/active" invariants (pending regularization, primary mapping,
  active shift assignment).
- Fixed-point types implement `Value/Scan` and JSON so exactness holds from DB to API.

## 10. Security

Law: `docs/SECURITY.md`. HR specifics:

- Sensitive classes: statutory and bank data (`employee`), salary and payslips (`payroll`, `compensation`), ratings
  and review text (`performance`), candidate PII and CTC (`recruitment`), dependants and life events (`benefits`),
  exit interviews (`onboarding`).
- Masking by default, separate `*_sensitive` permissions, never in events/audit (HR-009), field-level checks in
  services (not only in controllers).
- Object-level authorization through the scope order in `specification.md` §2.2; tested per context with
  exact-grant integration tests (a user holding only the named permission).
- Threat → evidence tables live in each context's `design.md` (Security section).
