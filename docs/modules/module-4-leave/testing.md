# Module 4 — Testing

Strategy: RED first per item. Unit (calc 100%, services ≥ 90%, others ≥ 80%) + unit goldens (G2–G8, G10, G11, G13) + live goldens (G1, G5, G7–G15) + migration up/down/up + AutoMigrate↔SQL parity diff.

```bash
# unit ring
go test -count=1 -cover ./internal/leave/...
# live ring (Docker PG/Redis/RabbitMQ + backend on :8080; boot steps in module-3 handoff.md)
go test -count=1 -tags integration ./tests/api/ -run Leave -v
```

## Integration onto Module 0–2 hardening (2026-10-09)
Merged onto `fix/module-0-2-hardening` as branch `integrate/m3-m4-on-hardening`. Migrations renumbered 000025–000036 → 000026–000037 (SQL runner is the schema authority, D-HARDEN-3); module wired in `internal/app` (`time_modules.go`); controllers normalize persistence errors (unique violation → 409); live tests ported from the removed `tests/api` suite to `tests/integration` (in-process, real Postgres + Redis) — run with `JAAS_REQUIRE_INTEGRATION=1 go test -count=1 ./tests/integration/`. Full suite 59/59 PASS; migration cycle 37 up / 12 down / 12 up clean.
