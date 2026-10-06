# Module 2 — Changelog

- **2026-10-05:** Frozen P1 design bundle (specification FRs, Clean architecture, connections edge table, candidates G1..G10). Kicked off P2 backend implementation.

## 2026-10-06 — hardening pass (branch `fix/module-0-2-hardening`)
Verification-console findings fixed and pinned by tests; full list in `docs/dev-test/HARDENING_REPORT.md`. Highlights: PII unmask requires employee:read_sensitive (audited), status state machine + exit/confirmation effects, synchronous user-deactivation convergence (FR-EV001), emergency-contact validation, document verify ownership, statutory upsert fix.
Sensors: build/vet/gofmt clean; unit + integration + golden + security tests pass; coverage 86.5 % (services 93-97 %); migrations up/down/up clean.
