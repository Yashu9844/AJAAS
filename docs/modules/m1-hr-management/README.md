# M1 HR Management

One HR service for the whole employee lifecycle — from requisition to exit — built to the depth of an enterprise
HCM suite (reference: Workday HCM). It is a single product module and a single deployable service with internal
bounded contexts. Architecture authority: `docs/ARCHITECTURE.md` (sections 5 "M1", 12, 13).

**Status (2026-10-20):** seven contexts built or in progress; five planned. Restructure Phase A (docs) done; code still
lives in the pre-restructure layout until Phase C (see `plan.md`).

## Bounded contexts

| Context | What it owns | Status | Docs |
|---|---|---|---|
| `organization` | departments, teams, designations, reporting lines (mappings + manager), org chart | built (backend) | `contexts/organization/` |
| `employee` | employee profile, employment, contacts, masked statutory and bank data, document metadata, timeline | built (backend) | `contexts/employee/` |
| `attendance` | shifts, shift assignments, punches, daily attendance records, regularization, summaries | built (backend) | `contexts/attendance/` |
| `leave` | leave types, holiday calendar, ledger-backed balances, accrual, requests, approvals | built (backend) | `contexts/leave/` |
| `payroll` | salary structures, CTC assignments, maker-checker payroll runs, statutory deductions (India), payslips, payout CSV | built (backend) | `contexts/payroll/` |
| `recruitment` | job openings, candidates and stage history, interviews and feedback, offers, hire saga | built (backend) | `contexts/recruitment/` |
| `performance` | review cycles, goals and key results, check-ins, reviews, 9-box | in progress (models, engines, migrations, validators) | `contexts/performance/` |
| `core` | job architecture, positions and headcount, effective-dated job history, lifecycle business processes | planned | `specification.md` §4 |
| `onboarding` | onboarding and offboarding plans, tasks, document requirements, exit clearance | planned | `specification.md` §5 |
| `compensation` | pay grades and ranges, salary change history, merit / increment cycles | planned | `specification.md` §6 |
| `learning` | course catalogue (external links / documents with thumbnails), assignments, completions, learning plans | built 2026-10-23 | `contexts/learning/` |
| `benefits` | tenant-defined benefit plans, eligibility, enrolment windows, enrolments and dependants | built 2026-10-24 | `contexts/benefits/` |

## Files in this bundle

| File | Holds |
|---|---|
| `specification.md` | HR-wide rules, actors and permissions, cross-context contracts, Workday-parity scope for built contexts, and the requirements of the five planned contexts |
| `design.md` | HR service architecture: package layout, context map, ports, data ownership, routes, events, sagas, concurrency, security |
| `testing.md` | HR test strategy, golden-test index across contexts, integration suite, test-console scenarios |
| `plan.md` | phase plan (B–E), per-context status, open gaps |
| `decisions.md` | HR-level decisions (context-level decisions stay in each context's `decisions.md`) |
| `contexts/<context>/` | `specification.md`, `design.md`, `testing.md`, `decisions.md` for each built context |
| `model/` | owner-supplied logical DB model, ER diagram and service diagram (verbatim) |

## Depends on / consumed by

- **Depends on:** M0 Identity & Access (tenant resolution, authentication, authorization, user provisioning, audit).
  Later: M7 Approvals (multi-step workflows), M8 Documents (file lifecycle), M10 Notifications (delivery).
- **Consumed by:** M2 Projects & Tasks, M3 Meetings, M4 Assets & Inventory (employee references), M5 Finance (payroll
  export), M6 Reports & Analytics and M9 AI Assistant (events and read APIs), M7 Approvals (approval requests).

## How to run and test

See `testing.md` (commands) and `docs/TEST_CONSOLE.md` (manual verification in the browser).
