# M1 HR Management — Plan

Updated 2026-10-20. Branch `restructure_architecture` (from `module_7_performance`, which is 4 commits ahead of
`main` @ `fe620fa`).

## 1. Status

| Context | Backend | Test console | Notes |
|---|---|---|---|
| organization | done (28 ops incl. locations, 2026-10-20) | `/dev-test/hr/organization` (Locations tab, `s1.loc`) | |
| employee | done (14 + 31 parity ops, 2026-10-21) | `/dev-test/hr/employee` (records tab, `hr.emp.records`) | |
| attendance | done (19 ops) | `/dev-test/hr/attendance`, 3 scenarios | |
| leave | done (20 ops) | `/dev-test/hr/leave`, 2 scenarios | |
| payroll | done (20 ops) | `/dev-test/hr/payroll`, 2 scenarios | |
| recruitment | done (20 ops) | `/dev-test/hr/recruitment`, 1 scenario | last-seat concurrent hire can orphan a profile (documented, untested) |
| performance | done (33 ops incl. calibration + continuous feedback), 2026-10-20 | `/dev-test/hr/performance`, 4 scenarios | P2: templates per role, peer nomination |
| core | done (32 ops), 2026-10-21 | `/dev-test/hr/core`, 2 scenarios (live run pending) | `contexts/core/` |
| compensation | done (24 ops), 2026-10-21 | `/dev-test/hr/compensation`, 1 scenario | `contexts/compensation/` |
| onboarding, learning, benefits | not started | — | requirements in `specification.md` §5, §7, §8 |

## 2. Phases

Exactly one phase is CURRENT. Phases with a human gate stop for the owner's GO before code changes.

| Phase | Objective | Status | Gate |
|---|---|---|---|
| A | Architecture + docs restructure (this bundle, `docs/ARCHITECTURE.md`, document cleanup) | DONE — approved 2026-10-20 | done |
| B | M0 alignment incl. access model (M0 `plan.md` §3–4) | DONE 2026-10-20 | GO given 2026-10-20 |
| C | HR service boundary (§3 below) | DONE 2026-10-20 | GO given 2026-10-20 |
| D | Test console for M0 + all built HR contexts | DONE 2026-10-20 (32/32 scenarios, `docs/TEST_CONSOLE.md`) | — |
| E | Complete HR (§4 below) | CURRENT | per-context design gate |

## 3. Phase C — HR service boundary (steps)

**Result (2026-10-20):** all steps done except the consolidated outbox/exchange (kept per context, DHR-10) and the
generic saga table (the hire saga is already resumable through `recruitment_candidates.hired_user_id`; the last-seat
orphan fix stays in E.10). Commits: `f257c5e` (move + `/api/v1/hr`), `5b6f030` (HR root, identityport, kernel,
employee/api, boundary test), `6969751` (schemas + re-baseline), `fd2d97c` (event envelope). Unit, integration
(incl. `TestSchemasAreOwnedAndIsolated`, `TestImportBoundaries`) and migration up/down/up cycles are green.

Each step ends with unit + integration rings green and a commit.

1. **Baseline**: record current test results; generate the route-mapping table old → `/api/v1/hr/...`.
2. **Shared primitives** in `backend/internal/shared/`: `events` (versioned envelope), `outbox` (one table per schema
   with `context`), `inbox`, `idempotency` (middleware + table). Unit tests first.
3. **Move packages** `internal/<context>` → `internal/hr/<context>`; add `internal/hr/module.go`; route group
   `/api/v1/hr`; update integration tests' paths only.
4. **HR kernel** `internal/hr/kernel`: unify Actor, TxRunner, pagination, Money/Days/Fixed, scope evaluation; delete
   per-context duplicates.
5. **identityport**: interface + in-process adapter; replace every `internal/identity/...` import in HR; HR audit to
   `hr.audit_events`; permission registration through the port. Import-boundary test.
6. **Context `api` packages**: move cross-context reads behind them (employee directory, org reads, LeaveSync,
   UnpaidLeave, CreateEmployee); boundary test forbids other cross-context imports.
7. **Schemas** (gate): Postgres schemas `identity` and `hr`, migration directories split; remove HR → M0 foreign
   keys; consolidate outboxes. Because nothing runs in production, the proposal is to **re-baseline** migrations
   (fresh numbered sets per schema) instead of writing data-moving migrations; dev/test databases are recreated.
   Owner GO given 2026-10-20 (DHR-09).
8. **Idempotency-Key** on every retryable HR command (HR-010) with tests; hire saga state in `saga_operations`.
9. **Events**: new envelope, `jaas.hr.events`, alias period for old routing keys; forbidden-field scan test.
10. Regenerate `docs/SYSTEM_REFERENCE.md`; update context docs for new paths; DoD.

## 4. Phase E — complete HR (order)

1. `performance` P3–P9 (DTOs, repositories, services, HTTP, wiring, integration, console page, DoD) + parity P1
   items: calibration with audit, continuous feedback.
2. `organization` locations (P1) — prerequisite for core, leave calendars, payroll PT, benefits.
3. `core` — design (freeze spec + goldens) → build → back-fill (CH-012) → console page.
4. `employee` parity P1: change requests with approval, emergency contacts, education, prior employment.
5. `compensation` — needs core; drives payroll arrears/mid-month split (payroll parity P1).
6. `onboarding` — needs core; drives payroll final settlement (payroll parity P1).
7. `attendance` parity P1: manager inbox, overtime, rosters, auto-absent job, bulk import.
8. `leave` parity P1: manager routing + team calendar, per-location calendars, comp-off, encashment, leave year.
9. `payroll` parity P1: employer PF/ESI, state-wise PT, arrears, benefits deductions, final settlement.
10. `recruitment` parity P1: position-linked requisitions, scorecards, multi-step offer approval; fix last-seat orphan.
11. `learning`.
12. `benefits`.
13. HR Definition of Done: all contexts green on every ring, every console scenario passing, `docs/ARCHITECTURE.md`
    §9 checklist items cited, `SYSTEM_REFERENCE` regenerated. Then the next module starts.

Each item runs the standard loop (`docs/ENGINEERING.md`): design → gate → RED tests → implementation → integration
→ console page → docs → commit.

## 5. Known gaps carried in

- Recruitment: concurrent last-seat hires leave an orphan employee profile from the losing request (step E.10).
- Payroll v1 limits listed in `contexts/payroll/specification.md` §10 (steps E.5, E.9).
- `internal/app.Build` is 77 lines (> 50 limit) — disappears with the HR composition root (step C.3).
- `TestPerformanceBudgets` can flake under host load; p95 for leave apply/preview not measured on a normal host.
- Events are produced but nothing consumes them into notifications (M10 not built).
