# M9 AI Assistant

**Status:** NOT STARTED — design pending. Proposed contract in `specification.md`; logical data model, ER diagram and
service diagram in `model/`.

**Purpose:** Governed AI interface; no direct database access or authority over source facts.

**Depends on:** M0, allowlisted domain APIs, model provider, PostgreSQL, secrets/KMS, audit

**Publishes / consumes:** Publishes run/tool audit events; consumes only approved retrieval/event feeds

**Before this module:** A `module-11-ai` template bundle and an empty `backend/internal/ai` package existed; no design content.

## Files

| File | Holds |
|---|---|
| `specification.md` | proposed purpose, data model, API shapes, rules, events and acceptance tests (from `docs/ARCHITECTURE.md` section 5) plus the proposed entities |
| `model/db-model.md` | proposed tables, relationships, dependencies, reliability tables, indexes (owner bundle, verbatim) |
| `model/erd.mmd` | Mermaid ER diagram |
| `model/architecture.mmd` | Mermaid service interaction diagram |

When the module enters design, `design.md`, `testing.md`, `plan.md` and `decisions.md` are added and the
specification is frozen (see `docs/ENGINEERING.md`, documentation standard).
