# HR / Compensation — Testing

## Golden tests (protected)

| # | Behaviour | Expected | Location |
|---|---|---|---|
| CO-G1 | Isolation | foreign ids 404 | integration |
| CO-G2 | Compa-ratio and penetration exact | 1.00 / 50.00 at midpoint; bands below < 0.90 ≤ within ≤ 1.10 < above | service |
| CO-G3 | No overlapping records or ranges | the open record or range closes at the new start; an earlier start is `BACKDATED` | service + integration |
| CO-G4 | Payroll CTC from the effective date only | the payroll assignment closes and reopens with the new CTC on that date | integration |
| CO-G5 | Self-approval | the requester or cycle opener cannot approve (403) | service + integration |
| CO-G6 | Budget never exceeded without override | `BUDGET_EXCEEDED`; override needs a justification | service + integration |
| CO-G7 | Apply twice | 409 `CYCLE_STATE`, no extra records | service + integration |
| CO-G8 | Manager visibility | managers see only proposals they manage, only while open or in review | service + integration |
| CO-G9 | Privacy | events carry no amounts or comments | integration |
| CO-G10 | Rollback | a payroll or store failure leaves no change, record or event | service |

## Evidence (2026-10-21)

- Package `internal/hr/compensation` (migrations hr/000058–000062, up/down/up verified).
- Unit: `compensation_test.go`, 90% of the package. The service and HTTP edge are covered, including failure
  injection; the gorm store is covered by integration.
- Integration: `TestCompensationLifecycle` covers ranges, an auto change synced to payroll, an approved promotion, the
  worker and self views, RBAC, a merit cycle through apply, the privacy scan and isolation.
- Contract: 24 operations in `backend/api/swagger.yaml` (tag Compensation); `TestSwaggerMatchesRoutes` passes.
- Console: `/dev-test/hr/compensation` with scenario `hr.comp`.
