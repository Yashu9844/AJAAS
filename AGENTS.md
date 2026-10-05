# AGENTS.md — How to Operate in JAAS

Repo is the source of truth. Never guess what you can read.

## Entry workflow (every task, in order)

```text
1. Read this file (AGENTS.md)
2. Read VISION.md (north star + SC-001..SC-010)
3. Identify the requested module (0–12, see DEPENDENCY-GRAPH.md)
4. Read docs/modules/<slug>/README.md
5. Read docs/modules/<slug>/agent.md (BEHAVIOR LAW — obey it)
6. Read docs/modules/<slug>/current-goal.md (single objective)
7. Read docs/modules/<slug>/plan.md (phase map — confirm one CURRENT, one NEXT)
8. Read docs/modules/<slug>/current-status.md (reality check)
9. Read specification.md + architecture.md + connections.md
10. Read security.md + docs/SECURITY.md (law)
11. Read testing.md + files.md
12. Inspect relevant src + tests (expand context only as needed)
13. Plan the smallest safe change (BLUEPRINT; human gate if schema/API/cross-stack)
14. Implement (one checklist item per tick — never batch)
15. Run unit tests → integration → golden → security (RED first for new code)
16. Run affected dependent-module tests
17. Update docs (status, todo/checklist, plan map, handoff, changelog, files)
18. Review git diff (no unrelated changes, no secrets)
19. Produce handoff (MASTER_PROMPT §38 block)
```

## Loop

Per-task loop: `loop.md` (tick prompt). System: `docs/SERVICE_LOOP.md`. Master contract: `docs/MASTER_PROMPT.md`.
On failure: Detect → Diagnose (implementation vs test vs env vs dependency vs config vs external) → Fix → Retest. Same error 3× → mark BLOCKED, commit nothing, exit.

## Ownership

- Stay inside your module: `backend/internal/<mod>/**`, `frontend/src/modules/<mod>/**`, `docs/modules/<mod>/**`.
- `backend/internal/shared/**` changes need a `decisions.md` entry + dependent test run.
- One module per commit: `feat(module-0): ...`, `test(module-0): ...`, `docs(module-0): ...`, `fix(module-0): ...`.
- Branch for this system: `loop_engineering`.

## Gates that stop you

Schema migrations, public API changes, backend↔frontend contract changes, multi-module blast radius, prod secrets, anything irreversible → STOP and ask the human. Everything else: proceed on defaults, report at handoff.

## Sensors (must pass before done)

Backend: `go build ./...`, `go vet ./...`, `gofmt -l`, `go test -race ./...`, coverage ≥80% (≥90% services), `grep -r TODO` empty, migrations up/down/up clean.
Frontend: `tsc --noEmit`, `eslint`, `next build`.
Perf p95: single GET <200ms, list <500ms, login <300ms, query <50ms.
Never claim an unrun test passed. Docs must never claim unimplemented behavior.
