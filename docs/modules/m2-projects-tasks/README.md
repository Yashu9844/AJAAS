# M2 Projects & Tasks

**Status:** BUILDING — design approved 2026-10-28 (answers in `decisions.md`). P1 foundations done; P2 (project context) next.

**Purpose:** Project delivery and work tracking; employee identity is referenced by IDs.

**Depends on:** M0, HR identity references, PostgreSQL, RabbitMQ, Documents refs, Notifications events

**Publishes / consumes:** Publishes project/task events; consumes identity lifecycle and notification-related configuration

**Before this module:** An empty `backend/internal/projects` package existed; no design content.

## Files

| File | Holds |
|---|---|
| `specification.md` | the contract: requirement ids (PR, TK, CM, TM, AT, SP, VW, EV, SC), extension points and golden tests |
| `design.md` | service boundary, packages, data model, mechanisms, storage layer, API groups, performance |
| `decisions.md` | proposed decisions and the open questions to answer |
| `plan.md` | steps P0–P8 with gates |
| `testing.md` | required test coverage |
| `reference-kaneo.md` | feature matrix against the open-source Kaneo reference (no code copied) |
| `model/db-model.md` | proposed tables, relationships, dependencies, reliability tables, indexes (owner bundle, verbatim) |
| `model/erd.mmd` | Mermaid ER diagram |
| `model/architecture.mmd` | Mermaid service interaction diagram |

The specification freezes when the design is approved (see `docs/ENGINEERING.md`, documentation standard).
