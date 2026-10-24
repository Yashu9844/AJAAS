# HR/Benefits — design

One package, `backend/internal/hr/benefits`, in the style of compensation and learning: `models.go`, `store.go` (gorm behind
a `Store` interface so the service is unit-tested with an in-memory fake), `service.go` (settings, windows, plans, versions),
`elections.go` (options, elections, deductions, termination hook, report, dependants, life events) and `http.go`.

Ports: `Employees`, `Core` (`core/api.Directory.PlacementOn`) and `Audit`. Wiring in `internal/hr/module.go`:
`payroll.SetBenefits(benefits.Service())` supplies payroll's deductions, and `core.OnActionCompleted(CoreHook)` ends and
reinstates elections on terminate and rescind. Tables are `ben_*`; the outbox is `ben_events_outbox`.

```
plan ──< version (effective-dated costs, coverage levels)
worker ──< election >── plan      election: active | waived | ended, [effective_from, effective_to)
worker ──< dependant, life event  (sensitive)
window = open enrolment (tenant dates) | new hire | life event
payroll period end ──> Deductions(): elections covering the last day × level employee cost
```
