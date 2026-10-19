# M7 Approvals

**Status:** NOT STARTED — design pending. Proposed contract in `specification.md`; logical data model, ER diagram and
service diagram in `model/`.

**Purpose:** Reusable workflow state; originating service owns final business record.

**Depends on:** M0 actor/role claims, PostgreSQL, RabbitMQ, Notifications

**Publishes / consumes:** Publishes workflow decisions; consumes approval requests from HR, Finance, Assets and other domains

**Before this module:** An unnumbered `module-x-approvals` template bundle and an empty `backend/internal/approvals` package existed; no design content. HR/Leave, HR/Attendance and HR/Payroll currently run single-step approvals inside their own contexts; those move behind M7 when it is built (section 8 phase 4).

## Files

| File | Holds |
|---|---|
| `specification.md` | proposed purpose, data model, API shapes, rules, events and acceptance tests (from `docs/ARCHITECTURE.md` section 5) plus the proposed entities |
| `model/db-model.md` | proposed tables, relationships, dependencies, reliability tables, indexes (owner bundle, verbatim) |
| `model/erd.mmd` | Mermaid ER diagram |
| `model/architecture.mmd` | Mermaid service interaction diagram |

When the module enters design, `design.md`, `testing.md`, `plan.md` and `decisions.md` are added and the
specification is frozen (see `docs/ENGINEERING.md`, documentation standard).
