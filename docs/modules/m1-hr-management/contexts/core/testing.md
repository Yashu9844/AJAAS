# HR / Core — Testing

## Golden tests (protected)

| # | Behaviour | Expected | Location |
|---|---|---|---|
| CG1 | Isolation | foreign ids 404; foreign token 403 | integration |
| CG2 | Action state graph | every allowed and forbidden transition; illegal → INVALID_STATE, nothing changes | service |
| CG3 | No overlapping assignments | sequential actions close and open periods exactly; the DB exclusion constraint rejects a forced overlap | service + integration |
| CG4 | Future-dated change | visible "as of" its date only; current views unchanged before it | service + integration |
| CG5 | Self-approval | the initiator cannot approve (403) | service + integration |
| CG6 | Grade and transfer rules | promote needs a higher rank or an override reason; transfer needs a new department or location | service |
| CG7 | Position never double-filled | non-shared: second incumbent 409 on overlapping dates, allowed after the first leaves; shared: Σ FTE ≤ budget | service + integration |
| CG8 | Termination effects | assignment closes after the last working day; employee terminated, login deactivated once effective; position freed | service + integration |
| CG9 | Rescinded termination | restores the assignment, position occupancy, employee status and login | service + integration |
| CG10 | Rehire | same worker id, history kept, new assignment, login reactivated | service + integration |
| CG11 | History "as of" | the worker view and the as-of org view reproduce any past date | service + integration |
| CG12 | Back-fill and projections | one initial assignment per worker, idempotent; actions project department, location and manager onto the primary mapping | service + integration |

## Commands

```bash
go test -count=1 -cover ./internal/hr/core/...
JAAS_REQUIRE_INTEGRATION=1 go test -count=1 -run Core ./tests/integration/
node scripts/console-headless.mjs <slug> <admin-email> <password> hr.core
```

## Delivery evidence

- Unit: lifecycle 100%; services 93.7% (golden suites for hire/effects, state graph and self-approval, change
  rules, positions, terminate/rescind/rehire, architecture, workers/headcount/back-fill, failure injection);
  controllers + routes 99.8% over 32 operations.
- Integration: `TestCoreLifecycle` (CG1, CG3–CG12 against Postgres, including the exclusion constraint and the
  employee, organization and login projections) is green, and the full suite is green.
  `TestSwaggerMatchesRoutes` covers the 32 operations.
- Migrations hr/000048–000053: up/down/up verified.
- Console: `/dev-test/hr/core`, with scenarios `hr.core.flow` and `hr.core.rbac`. tsc and eslint are clean. The live
  headless run is pending: the API process was stopped for low memory.
