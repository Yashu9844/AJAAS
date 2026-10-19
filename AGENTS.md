# AGENTS.md — How to Operate in JAAS

The repository is the source of truth. Never guess what you can read.

## Entry workflow (every task, in order)

```text
1. Read this file.
2. Read docs/ARCHITECTURE.md §1–2 and §12–16 (canonical modules M0–M12, repo layout, current migration phase).
3. Identify the module (M0–M12) and, for M1 HR, the bounded context. Old "Module 1–7" names are HR contexts.
4. Read docs/modules/<mNN-…>/README.md and plan.md (status, current phase, gaps).
5. For HR work also read docs/modules/m1-hr-management/{specification,design}.md and the context's
   contexts/<context>/{specification,design,testing,decisions}.md.
6. Read docs/SECURITY.md (law) and docs/ENGINEERING.md (loop, gates, sensors, Definition of Done, doc standard).
7. Inspect the relevant code and tests (docs/SYSTEM_REFERENCE.md Part 2 indexes every route, table, file, function).
8. Plan the smallest safe change; stop at a human gate for schema / public API / frontend contract / multi-module /
   secrets / irreversible changes.
9. RED test first → implement one item → sensors → integration → test-console page → docs → commit → handoff.
```

## Rules that stop you

- Same error three times → mark BLOCKED in the module's `plan.md` with evidence, commit nothing, stop.
- Never claim an unrun test passed. Docs never claim unimplemented behaviour.
- Golden tests are protected (`docs/ENGINEERING.md` §8).
- Stay inside your module/context; shared-code changes need a decision entry and dependent test runs.

## Sensors

Backend: `go build ./...`, `go vet ./...`, `gofmt -l` (strip CRLF), unit tests, coverage ≥ 80% (≥ 90% services),
no `TODO`, migrations up/down/up, integration ring with `JAAS_REQUIRE_INTEGRATION=1`.
Frontend: `tsc --noEmit`, `eslint`, `next build`. Perf p95: GET < 200 ms, list < 500 ms, login < 300 ms, query < 50 ms.

Loop tick prompt: `loop.md`.
