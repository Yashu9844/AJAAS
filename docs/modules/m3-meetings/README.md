# M3 Meetings

**Status:** DESIGN APPROVED (2026-11-04), build not started - see `plan.md`. Research in `research/` (features, journeys, market, open source, internals, architecture options, validated answers to the 12 owner questions); decisions DMT-01..20 in `decisions.md`.

**Purpose:** Scheduling, meeting records and decisions; tasks remain owned by Projects & Tasks.

**Depends on:** M0, HR identity references, PostgreSQL, RabbitMQ, Documents refs, Projects task references

**Publishes / consumes:** Publishes meeting/minutes/action-item events; consumes identity and task references

**Before this module:** An unnumbered `module-x-meetings` template bundle and an empty `backend/internal/meetings` package existed; both held no design content and were removed.

## Files

| File | Holds |
|---|---|
| `specification.md` | phase-1 contract: phase plan by feature id, rules MR-01..13, API, events, acceptance tests, extension points |
| `design.md` | packages, ports, data model, materialiser, authorizer, notes, action items, feed, performance, risks |
| `decisions.md` | DMT-01..20 (owner-approved 2026-11-04) |
| `security.md` | threats T1-T16 with mitigations and planned evidence |
| `testing.md` | test layers, golden tests, gates |
| `plan.md` | build steps P0-P7 and status |
| `research/` | research input (see `research/README.md`) |
| `model/` | original proposed tables and diagrams (pre-design; superseded by `design.md` section 2 where they differ) |
