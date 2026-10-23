# HR/Learning — testing

## Gates

| Gate | Where |
|---|---|
| LG1 a rule enrols a new joiner exactly once however often it runs | `TestCoreHook`, `TestRecurrence`, `TestLearning` (a real core hire then a promotion) |
| LG2 a retired course keeps its completions | `TestRetiredCourseKeepsCompletions`, `TestLearning` |
| LG3 overdue follows the tenant time zone | `TestOverdueFollowsTheTenantZone`, `TestSettingsAndZones`, `TestLearning` (Pago Pago vs Kiritimati) |
| LG4 unsafe URLs are refused | `TestCourseValidation`, `TestLearning`, console scenario |
| LG5 a completion needing verification cannot finish the enrolment until another person verifies it | `TestVerification`, `TestLearning` |
| LG6 events and audit hold ids only | `TestLearning` (scores, evidence, comments, reasons never appear) |

Also covered: workforce statuses (`TestWorkforceStatuses`), plans and progress, self-enrolment, corrections and waivers,
the HTTP layer including permissions (`TestHTTP`), and the model helpers.

## Evidence (2026-10-23)

- Migrations hr/000085–000088 (up/down/up verified on a scratch database).
- `learning` 89% unit coverage (services 88–100% per function; the gorm store is exercised by `TestLearning`).
- Contract: 35 operations in `backend/api/swagger.yaml` (tag Learning).
- Console: `/dev-test/hr/learning` lists 35 operations; scenario `hr.learning.flow`.
- The manager-verifies path is unit-tested with a fake report line; the integration test verifies as HR because the test
  tenant has no manager mapping for the hired worker.
