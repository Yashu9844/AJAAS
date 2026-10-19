# M5 Finance

**Status:** NOT STARTED — design pending. Proposed contract in `specification.md`; logical data model, ER diagram and
service diagram in `model/`.

**Purpose:** Financial operations excluding payroll calculation, which remains HR-owned.

**Depends on:** M0, PostgreSQL, RabbitMQ, Approvals, Documents, HR payroll export/reference where agreed, Notifications

**Publishes / consumes:** Publishes expense/reimbursement/budget events; consumes approval outcomes

**Before this module:** No prior bundle. HR/Payroll owns payroll calculation; Finance must not recalculate payroll.

## Files

| File | Holds |
|---|---|
| `specification.md` | proposed purpose, data model, API shapes, rules, events and acceptance tests (from `docs/ARCHITECTURE.md` section 5) plus the proposed entities |
| `model/db-model.md` | proposed tables, relationships, dependencies, reliability tables, indexes (owner bundle, verbatim) |
| `model/erd.mmd` | Mermaid ER diagram |
| `model/architecture.mmd` | Mermaid service interaction diagram |

When the module enters design, `design.md`, `testing.md`, `plan.md` and `decisions.md` are added and the
specification is frozen (see `docs/ENGINEERING.md`, documentation standard).
