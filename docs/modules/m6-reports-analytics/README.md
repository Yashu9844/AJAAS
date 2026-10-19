# M6 Reports & Analytics

**Status:** NOT STARTED — design pending. Proposed contract in `specification.md`; logical data model, ER diagram and
service diagram in `model/`.

**Purpose:** Read models and analytics; never the transactional source of truth.

**Depends on:** RabbitMQ event streams, PostgreSQL projection store, object storage, M0 authorization context

**Publishes / consumes:** Publishes report execution/projection events; consumes versioned domain events

**Before this module:** A `module-10-analytics` template bundle and an empty `backend/internal/analytics` package existed; no design content.

## Files

| File | Holds |
|---|---|
| `specification.md` | proposed purpose, data model, API shapes, rules, events and acceptance tests (from `docs/ARCHITECTURE.md` section 5) plus the proposed entities |
| `model/db-model.md` | proposed tables, relationships, dependencies, reliability tables, indexes (owner bundle, verbatim) |
| `model/erd.mmd` | Mermaid ER diagram |
| `model/architecture.mmd` | Mermaid service interaction diagram |

When the module enters design, `design.md`, `testing.md`, `plan.md` and `decisions.md` are added and the
specification is frozen (see `docs/ENGINEERING.md`, documentation standard).
