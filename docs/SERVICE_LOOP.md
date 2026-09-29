# JAAS Service Loop — Loop-Engineered Delivery

How every service gets built start-to-finish: autonomous loops with verification,
not manual prompting. Combines the Ralph loop (Huntley), harness Guides + Sensors
(Thoughtworks), and PEV gates (Plan → Execute → Verify).

Backlog: `docs/PENDING_WORK.md`. Law: `docs/SECURITY.md`. Intent: `VISION.md`.
Tick prompt: `loop.md`.

## Outer loop — one service at a time

Work through `PENDING_WORK.md` "Suggested build order" strictly in sequence.
One service is fully finished — code → tests → fix → integrate — before the next starts:

1. Models (11 missing; `tenant.go` must compile)
2. Migrations (12 pairs) + `cmd/main.go` + compose infra (PG/Redis/RabbitMQ)
3. DTOs + validator
4. Repositories (12)
5. Services: Tenant → Auth → User → Role → Permission/Session/Token/Audit
6. Controllers + routes (24 endpoints)
7. Middleware stack (CORS → RateLimit → TenantResolver → Auth → RBAC → Audit)
8. Events / queue (12 event types)
9. Frontend identity against the live API
10. E2E + hardening, then Module 1 design

Never start step N+1 while step N has failing sensors or unticked checklist items.

## Inner loop — per task (8 phases)

| Phase | Does | Gate |
|---|---|---|
| ANALYZE | Read checklist item + only the spec pages it needs | — |
| BLUEPRINT | 5-line file plan | Human gate if schema / public API / cross-stack contract |
| RED | Failing test first, confirm right failure reason | Test must fail before code |
| GREEN | Minimal impl until RED passes, one item only | — |
| REFACTOR | Names, one-line docs, size limits, no bans violated | SECURITY.md §13 |
| REVIEW | Fresh-eyes critic pass vs §14 checklist | Builder never approves own work blind — re-read the diff |
| INTEGRATE | Boot backend, hit endpoint; update frontend vs LIVE api; E2E path | Real services, never mocked seam |
| NEXT | Tick checklist, commit, report, exit | One item per tick, never batch |

## Loop contract

```text
TRIGGER -> next unchecked item in implementation-checklist.md
SCOPE   -> one service, one item, files in BLUEPRINT only
ACTION  -> RED -> GREEN -> REFACTOR -> REVIEW -> INTEGRATE
BUDGET  -> max 20 iterations per item; same error 3x -> BLOCKED + exit; $ cap set before sleep
STOP    -> all sensors green + critic pass + checklist ticked
REPORT  -> what changed, sensors run, next item up
```

## Sensors (say NO for us)

Silent success, verbose failure. From SECURITY.md §11.

- Backend: `go build ./...`, `go vet ./...`, `gofmt -l`, `go test -race ./...`,
  coverage ≥80% (≥90% services), `grep -r TODO` empty, migrations up/down/up clean,
  24 endpoint contract tests, isolation + reuse-detection + enumeration tests.
- Frontend: `tsc --noEmit`, `eslint`, `next build`, 401→refresh→retry + 403-state tests.
- Perf (p95): single GET <200ms, list <500ms, login <300ms, query <50ms.
- Any repeated prose correction gets promoted to a mechanical check (lint/arch/CI).

## Human gates (only these interrupt the loop)

Schema migrations, public API changes, backend↔frontend contract changes,
multi-service blast radius, prod secrets, anything irreversible.
Everything else proceeds on defaults and gets reviewed at REPORT.

## Memory (loop never re-derives state)

| Anchor | Role |
|---|---|
| `VISION.md` | North star + done criteria (SC-001..SC-010) |
| `docs/SECURITY.md` | Enforceable standard, sensors, checklists |
| `loop.md` | Per-tick prompt |
| `implementation-checklist.md` | Progress state — the loop's disk memory |
| Journal (BLOCKED entries) | Failure memory — tunes future ticks |
| Skills below | Compounding recipes, not one-off prompts |

## Skills to build (each: prompt + tool policy + verification)

- `/implement-service` — RED→GREEN→REFACTOR for one service file set
- `/verify-service` — run that service's full sensor suite
- `/fix-ci` — failing check → reproduce → minimal fix → re-run
- `/code-review` — critic pass vs SECURITY.md §14
- `/e2e-feature` — backend boot → frontend vs live API → E2E path

## Guardrails

- Worktree/branch isolation per loop run; never destructive ops on main unasked.
- Least-privilege tools per phase (review needs read-only).
- Cost: iterations capped, no-progress detection (same diff/error 3x → stop),
  dollar budget set before any unattended run.
- Verification is never the builder's self-report — independent critic or CI.

## Failure modes we refuse

- Unverifiable goal → every item maps to checklist + acceptance criteria first.
- Wide action surface → SCOPE limits files; gates limit blast radius.
- No memory → checklist + journal + anchors carry state across ticks.
- Self-verification → REVIEW phase + sensors + (later) CI are separate from builder.
