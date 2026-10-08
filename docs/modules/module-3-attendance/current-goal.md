# Module 3 — Current Goal

Goal: G3-5 services + events — ports (employeeDirectory over Module 2, auditLogger over Module 0, txRunner, clock), event envelope (FR-EV001), ShiftService, AssignmentService, PunchService, QueryService, RegularizationService, OutboxRelay.
Why: every AT rule lives here; HTTP (P6) only adapts.
Scope: backend/internal/attendance/{services,events}/**.
Non-goals: HTTP, wiring, live DB.
Success Criteria:
- [ ] unit goldens G2, G3, G6, G7, G8, G10, G11, G12 green (in-memory fakes honoring repo semantics)
- [ ] services coverage ≥ 90%
- [ ] build/vet/gofmt clean; no TODO
- [ ] plan/status/todo/handoff/changelog updated
