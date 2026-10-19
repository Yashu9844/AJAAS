# M4 Assets & Inventory

**Status:** NOT STARTED — design pending. Proposed contract in `specification.md`; logical data model, ER diagram and
service diagram in `model/`.

**Purpose:** Physical and digital asset lifecycle and inventory accounting.

**Depends on:** M0, HR employee references, PostgreSQL, RabbitMQ, Approvals, Documents, Notifications

**Publishes / consumes:** Publishes asset/stock events; consumes approval outcomes and identity lifecycle

**Before this module:** A `module-8-assets` template bundle (old masterplan number) and an empty `backend/internal/assets` package existed; neither held design content. The old masterplan also sketched QR asset tags; consider it at design.

## Files

| File | Holds |
|---|---|
| `specification.md` | proposed purpose, data model, API shapes, rules, events and acceptance tests (from `docs/ARCHITECTURE.md` section 5) plus the proposed entities |
| `model/db-model.md` | proposed tables, relationships, dependencies, reliability tables, indexes (owner bundle, verbatim) |
| `model/erd.mmd` | Mermaid ER diagram |
| `model/architecture.mmd` | Mermaid service interaction diagram |

When the module enters design, `design.md`, `testing.md`, `plan.md` and `decisions.md` are added and the
specification is frozen (see `docs/ENGINEERING.md`, documentation standard).
