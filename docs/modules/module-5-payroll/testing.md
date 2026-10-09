# Module 5 — Testing

RED first. Unit: calc 100% (statutory goldens G2–G9), services ≥ 90% (G10–G12, G15 on fakes), controllers/routes 100%. Integration (in-process harness, real Postgres + Redis): G1, G10–G14, G16 + full lifecycle. Migration cycle up/down/up.

```bash
go test -count=1 -cover ./internal/payroll/...
JAAS_REQUIRE_INTEGRATION=1 go test -count=1 -run Payroll ./tests/integration/
```
