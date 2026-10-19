# M8 Documents

**Status:** NOT STARTED — design pending. Proposed contract in `specification.md`; logical data model, ER diagram and
service diagram in `model/`.

**Purpose:** Secure file lifecycle; binaries stored in object storage.

**Depends on:** M0, PostgreSQL, object storage, malware scanner, KMS, RabbitMQ

**Publishes / consumes:** Publishes upload/scan/version/access lifecycle events; consumes document requirement/reference events

**Before this module:** No prior bundle. HR/Employee stores employee document metadata today; file lifecycle moves to M8 when it is built (section 8 phase 4).

## Files

| File | Holds |
|---|---|
| `specification.md` | proposed purpose, data model, API shapes, rules, events and acceptance tests (from `docs/ARCHITECTURE.md` section 5) plus the proposed entities |
| `model/db-model.md` | proposed tables, relationships, dependencies, reliability tables, indexes (owner bundle, verbatim) |
| `model/erd.mmd` | Mermaid ER diagram |
| `model/architecture.mmd` | Mermaid service interaction diagram |

When the module enters design, `design.md`, `testing.md`, `plan.md` and `decisions.md` are added and the
specification is frozen (see `docs/ENGINEERING.md`, documentation standard).
