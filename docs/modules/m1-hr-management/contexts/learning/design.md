# HR/Learning — design

One package, `backend/internal/hr/learning`, in the style of compensation: `models.go`, `store.go` (gorm, behind a
`Store` interface so the service is unit-tested with an in-memory fake), `service.go` (settings, courses, plans),
`assign.go` (assignments, rule matching, the core hook, recurrence), `enrol.go` (enrolments, completions, views,
reports) and `http.go` (routes, permission catalogue, module).

Ports: `Employees` (directory), `Core` (`core/api.Directory.PlacementOn`), `Reports` (the organization reporting line) and
`Audit`. Learning is wired in `internal/hr/module.go`; it registers `core.OnActionCompleted(CoreHook)` and an hourly
`RunRecurrence` loop. Tables are `lrn_*`; the outbox is `lrn_events_outbox`.

```
course ──< plan_item >── plan
assignment ──(creates)──> enrolment ──< completion
rule match = core placement (department | job profile | location) | all | one employee
```

Concurrency: self-enrolment takes an advisory lock per worker and course before checking for a live enrolment, and a
partial unique index backs it; enrolment creation is `INSERT … ON CONFLICT DO NOTHING` on the dedupe key; recurrence and
completion decisions lock the assignment, enrolment and completion rows.
