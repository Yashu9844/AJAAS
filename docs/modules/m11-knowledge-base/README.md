# M11 Company Knowledge Base

**Status:** NOT STARTED — design pending. Proposed contract in `specification.md`; logical data model, ER diagram and
service diagram in `model/`.

**Purpose:** Versioned company knowledge and ACL-aware search.

**Depends on:** M0, PostgreSQL, search index, Documents refs, RabbitMQ

**Publishes / consumes:** Publishes article/publication/index events; consumes document and identity/ACL lifecycle events

**Before this module:** No prior bundle.

## Files

| File | Holds |
|---|---|
| `specification.md` | proposed purpose, data model, API shapes, rules, events and acceptance tests (from `docs/ARCHITECTURE.md` section 5) plus the proposed entities |
| `model/db-model.md` | proposed tables, relationships, dependencies, reliability tables, indexes (owner bundle, verbatim) |
| `model/erd.mmd` | Mermaid ER diagram |
| `model/architecture.mmd` | Mermaid service interaction diagram |

When the module enters design, `design.md`, `testing.md`, `plan.md` and `decisions.md` are added and the
specification is frozen (see `docs/ENGINEERING.md`, documentation standard).
