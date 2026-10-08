# Module 3 — Current Goal

Goal: G3-7 live ring — Docker Postgres/Redis/RabbitMQ up; backend booted on :8080 (AutoMigrate + attendance permission seed); SQL migrations 000025–000030 up/down/up against a scratch database; live goldens (build tag `integration`) G1 isolation, G6/G7 regularization lifecycle + self-approval, G9 RBAC, G11 audit rows, G12 outbox rows, G13 pagination clamp, plus a full punch flow — exercising every repository method on real Postgres.
Why: repositories have only met fakes; this is the first real-SQL proof of the module.
Scope: backend/tests/api/attendance_*_test.go; fixes in backend/internal/attendance/** only for bugs the live run exposes.
Non-goals: new features, frontend.
Success Criteria:
- [ ] backend boots with Module 3 wired; seed rows present in `permissions`
- [ ] migrations 000025–000030 up → down → up clean on a scratch DB
- [ ] live goldens G1, G6, G7, G9, G11, G12, G13 + punch flow PASS
- [ ] unit ring still green; docs updated
