# M3 Meetings

**Status:** NOT STARTED — design pending. Proposed contract in `specification.md`; logical data model, ER diagram and
service diagram in `model/`.

**Purpose:** Scheduling, meeting records and decisions; tasks remain owned by Projects & Tasks.

**Depends on:** M0, HR identity references, PostgreSQL, RabbitMQ, Documents refs, Projects task references

**Publishes / consumes:** Publishes meeting/minutes/action-item events; consumes identity and task references

**Before this module:** An unnumbered `module-x-meetings` template bundle and an empty `backend/internal/meetings` package existed; both held no design content and were removed.

## Files

| File | Holds |
|---|---|
| `specification.md` | proposed purpose, data model, API shapes, rules, events and acceptance tests (from `docs/ARCHITECTURE.md` section 5) plus the proposed entities |
| `model/db-model.md` | proposed tables, relationships, dependencies, reliability tables, indexes (owner bundle, verbatim) |
| `model/erd.mmd` | Mermaid ER diagram |
| `model/architecture.mmd` | Mermaid service interaction diagram |

When the module enters design, `design.md`, `testing.md`, `plan.md` and `decisions.md` are added and the
specification is frozen (see `docs/ENGINEERING.md`, documentation standard).
