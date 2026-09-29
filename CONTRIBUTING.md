# Contributing to JAAS (loop_engineering branch)

## Setup

```bash
# infra (PG/Redis/RabbitMQ land in G0-3; until then unit-only validation)
docker compose up -d
# backend
cd backend && go run cmd/main.go   # cmd/main.go lands in G0-3; until then: go build ./... + go test ./...
# frontend
cd frontend && npm install && npm run dev
```

Until G0-3, `go build ./...`, `go vet ./...`, `go test -race ./...` are the boot-equivalent gates.

## Workflow

1. Pick ONE checklist item from your module's `todo.md` / `implementation-checklist.md` (mirror of plan.md CURRENT).
2. Follow `AGENTS.md` entry workflow + `loop.md` tick. One item per tick — never batch.
3. Human gates (STOP + ask): schema, public API, cross-stack contracts, multi-module blast radius, secrets, irreversible ops.
4. Update in the same task: code → tests → `current-status.md` → `todo.md` → `plan.md` map → `handoff.md` → `changelog.md` → `files.md` if structure changed.

## Commits

One module per commit: `feat(module-0): ...`, `test(module-0): ...`, `docs(module-0): ...`, `fix(module-0): ...`. Small, rebase-friendly, no unrelated formatting.

## PR gates (all must pass)

Backend: build, vet, gofmt, `go test -race`, coverage ≥80% (≥90% services), no TODO, migrations up/down/up clean.
Frontend: `tsc --noEmit`, `eslint`, `next build`.
Contract: envelopes + status map; isolation + RBAC + security tests green; goldens untouched (or §15 procedure followed).
Perf p95: single <200ms, list <500ms, login <300ms, query <50ms.
Secrets: none in diff. Docs: no claims beyond implementation.
