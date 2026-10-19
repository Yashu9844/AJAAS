# M10 Notifications

**Status:** NOT STARTED — design pending. Proposed contract in `specification.md`; logical data model, ER diagram and
service diagram in `model/`.

**Purpose:** Reliable multi-channel dispatch and delivery tracking.

**Depends on:** RabbitMQ, PostgreSQL, Redis (rate limits/cache), email/SMS/push providers, secrets manager

**Publishes / consumes:** Consumes domain events; publishes delivery outcome events

**Before this module:** An unnumbered `module-x-notifications` template bundle and an empty `backend/internal/notifications` package existed; no design content. M0 already publishes `identity.*` events that M10 will consume.

## Files

| File | Holds |
|---|---|
| `specification.md` | proposed purpose, data model, API shapes, rules, events and acceptance tests (from `docs/ARCHITECTURE.md` section 5) plus the proposed entities |
| `model/db-model.md` | proposed tables, relationships, dependencies, reliability tables, indexes (owner bundle, verbatim) |
| `model/erd.mmd` | Mermaid ER diagram |
| `model/architecture.mmd` | Mermaid service interaction diagram |

When the module enters design, `design.md`, `testing.md`, `plan.md` and `decisions.md` are added and the
specification is frozen (see `docs/ENGINEERING.md`, documentation standard).
