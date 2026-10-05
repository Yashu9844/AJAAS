# Module 6 — Plan (updated: 2026-09-30, owner: scaffold)

## Phase Map

| Phase | Goal ID | Objective | Depends On | Status | Proof |
|---|---|---|---|---|---|
| P0 | G6-0 | Bundle scaffold + dependency metadata | — | CURRENT | this scaffold |
| P1 | G6-1 | Design: spec, architecture, contracts, goldens | Module 0 (Identity), Module 2 (Employee) stable | NEXT | — |
| P2 | G6-2 | Implement against frozen contracts | P1 | LATER | — |
| P3 | G6-3 | Integrate + E2E + harden | P2 | LATER | — |

Status vocabulary: DONE / CURRENT (one) / NEXT (one) / LATER / BLOCKED.
Promotion needs MASTER_PROMPT section 24 Definition of Done + handoff. No phase skipping.

## Current Phase

- Goal: P0 scaffold — 17-file bundle with real dependency metadata, no placeholders for known facts.
- Why now: loop engineering must be able to start; fresh agents need module context.
- Scope in: docs/modules/module-6-approvals/**.md only.
- Scope out: production code, schema, API.
- Entry criteria: MASTER_PROMPT registry row known — true.
- Exit criteria: all 17 files present, validated by Step 5 checklist.
- Risks: design drift before deps stable (mitigate: P1 blocked until Modules 0 + 2 contracts).

## Next Phase

- Goal: P1 design.
- Why next: contracts must freeze before any implementation.
- Pre-reqs: Modules 0 + 2 contracts.
- Est. unblocks: consumer modules (Modules 7, 10, 11).

## Later Phases

- P2 implementation, P3 integration — per MASTER_PROMPT section 28 ordering.

## Gate Log

| Date | Phase | Gate | Decision | By | Reason |
|---|---|---|---|---|---|
| 2026-09-30 | P0 | Scaffold scope (docs only) | GO | scaffold | zero production impact |

## Progress Journal

| Date | Phase | Did | Sensors | Result | Handoff |
|---|---|---|---|---|---|
| 2026-09-30 | P0 | bundle scaffolded | structure check | GREEN | see handoff.md |
