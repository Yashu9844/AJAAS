# Module 6 — Testing

RED first. Pure pipeline graphs 100% (G2, G3); services ≥ 90% (G4–G11 on fakes); controllers/routes 100%; integration G1, G5, G6, G8, G10, G12–G14; migration cycle + parity.

```bash
go test -count=1 -cover ./internal/recruitment/...
JAAS_REQUIRE_INTEGRATION=1 go test -count=1 -run Recruitment ./tests/integration/
```
