# Module 7 — Testing

RED first. Pure score + lifecycle packages 100% (G2, G3); services ≥ 95% (G4–G11, G15 on fakes + failure injection); controllers/routes ~100%; integration G1, G4–G6, G8–G14; migration cycle + AutoMigrate↔SQL parity.

```bash
go test -count=1 -cover ./internal/performance/...
JAAS_REQUIRE_INTEGRATION=1 go test -count=1 -run Performance ./tests/integration/
```
