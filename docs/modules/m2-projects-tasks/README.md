# M2 Projects & Tasks

**Status:** NOT STARTED — design pending. Proposed contract in `specification.md`; logical data model, ER diagram and
service diagram in `model/`.

**Purpose:** Project delivery and work tracking; employee identity is referenced by IDs.

**Depends on:** M0, HR identity references, PostgreSQL, RabbitMQ, Documents refs, Notifications events

**Publishes / consumes:** Publishes project/task events; consumes identity lifecycle and notification-related configuration

**Before this module:** An empty `backend/internal/projects` package existed; no design content.

## Files

| File | Holds |
|---|---|
| `specification.md` | proposed purpose, data model, API shapes, rules, events and acceptance tests (from `docs/ARCHITECTURE.md` section 5) plus the proposed entities |
| `model/db-model.md` | proposed tables, relationships, dependencies, reliability tables, indexes (owner bundle, verbatim) |
| `model/erd.mmd` | Mermaid ER diagram |
| `model/architecture.mmd` | Mermaid service interaction diagram |

When the module enters design, `design.md`, `testing.md`, `plan.md` and `decisions.md` are added and the
specification is frozen (see `docs/ENGINEERING.md`, documentation standard).
