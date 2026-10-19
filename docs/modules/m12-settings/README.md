# M12 Settings

**Status:** NOT STARTED — design pending. Proposed contract in `specification.md`; logical data model, ER diagram and
service diagram in `model/`.

**Purpose:** Tenant configuration; secret values remain in a secrets manager.

**Depends on:** M0, PostgreSQL, secrets manager, RabbitMQ

**Publishes / consumes:** Publishes versioned configuration/integration events; domain services consume only relevant config changes

**Before this module:** No prior bundle. M0 has a `tenant_settings` table today; ownership moves to M12 when it is built (M0 `plan.md`).

## Files

| File | Holds |
|---|---|
| `specification.md` | proposed purpose, data model, API shapes, rules, events and acceptance tests (from `docs/ARCHITECTURE.md` section 5) plus the proposed entities |
| `model/db-model.md` | proposed tables, relationships, dependencies, reliability tables, indexes (owner bundle, verbatim) |
| `model/erd.mmd` | Mermaid ER diagram |
| `model/architecture.mmd` | Mermaid service interaction diagram |

When the module enters design, `design.md`, `testing.md`, `plan.md` and `decisions.md` are added and the
specification is frozen (see `docs/ENGINEERING.md`, documentation standard).
