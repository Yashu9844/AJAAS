# HR / Onboarding — Testing

## Golden tests (protected)

| # | Behaviour | Expected | Location |
|---|---|---|---|
| OG1 | Isolation | foreign ids 404 | integration |
| OG2 | Duplicate start | the same action, or a recruitment hire after a core hire, creates one checklist | service + integration |
| OG3 | Most specific templates win | department template beats the default; archived and non-matching are ignored | service + integration |
| OG4 | Blocking | a blocking task cannot be skipped and holds early completion | service |
| OG5 | Document task | needs a verified document; evidence id stored | service + integration |
| OG6 | Clearance | completion refused before all four sign-offs; event fires once at the last one; payroll port reports it | service + integration |
| OG7 | Exit interview privacy | only the worker and `onboarding:exit_interview`; `read` gets 404; not in events | service + integration |
| OG8 | Rescind | rescinding the core action cancels its checklist | service + integration |

## Evidence (2026-10-22)

- Package `internal/hr/onboarding` (migrations hr/000063–000068, up/down/up verified).
- Unit: `onboarding_test.go`, 89% of the package including failure injection; the gorm store is covered by integration.
- Integration: `TestOnboardingLifecycle` (core hire and terminate, document task, clearance, exit interview, rescind,
  isolation, privacy) and the recruitment golden now asserts the hire starts onboarding.
- Contract: 19 operations in `backend/api/swagger.yaml` (tag Onboarding); `TestSwaggerMatchesRoutes` passes.
- Console: `/dev-test/hr/onboarding` with scenario `hr.onb`.
