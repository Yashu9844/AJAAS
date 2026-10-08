# Module 4 — Testing

Strategy: RED first per item. Unit (calc 100%, services ≥ 90%, others ≥ 80%) + unit goldens (G2–G8, G10, G11, G13) + live goldens (G1, G5, G7–G15) + migration up/down/up + AutoMigrate↔SQL parity diff.

```bash
# unit ring
go test -count=1 -cover ./internal/leave/...
# live ring (Docker PG/Redis/RabbitMQ + backend on :8080; boot steps in module-3 handoff.md)
go test -count=1 -tags integration ./tests/api/ -run Leave -v
```
