# Module 1 — Current Goal

Goal: G1-2 Implementation — build Module 1 against the frozen P1 contracts: models, migrations 000013+, DTOs + validators, repositories, services (with cycle/depth guards), controllers + routes, events + consumers, module.go DI, swagger append, unit suites — RED-first, sensors green on a tooled machine.
Why: P1 design DONE 2026-10-01 (spec/architecture/connections/goldens frozen); implementation is the next topological step and unblocks P3 integration + Modules 2–5.
Scope: files.md P2 file map exactly. Cross-module touch allowed once: `organization:read/update` seed via Module 0 (decisions entry both sides).
Non-goals: P3 integration/E2E/goldens execution, frontend pages, contract changes (re-gate via decisions.md if needed).
Success Criteria:
- [ ] `go build ./internal/organization/...` + `go vet` + `gofmt -l` clean
- [ ] `go test ./internal/organization/...` + `-race` green; coverage ≥90% services, ≥80% module
- [ ] migrations 000013.. up/down/up clean; AC-style index/constraint check
- [ ] unit suites cover FR-D/T/DG/H/M/O/E guards (cycle, depth, primary-swap, deactivation blocks, event convergence)
- [ ] no TODOs, no placeholders; plan.md P2 → DONE with proof; handoff written
