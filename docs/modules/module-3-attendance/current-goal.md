# Module 3 — Current Goal

Goal: G3-3 calc engine + DTOs + validators — pure time functions (ParseHHMM/FormatHHMM, ExpectedMinutes, AttendanceDate per AT-005, ComputeTotals per AT-006..AT-010), request/response DTOs with pagination clamp, field validators (org-style code, HH:MM, IANA timezone, date ranges).
Why: services and controllers are thin wrappers over this logic; time-zone/night-shift math is the highest-risk code in the module.
Scope: backend/internal/attendance/{calc,dto,validators}/**.
Non-goals: repositories, services, HTTP.
Success Criteria:
- [ ] golden G4 (totals table) and G5 (night shift + time zones) green
- [ ] calc 100% statement coverage; dto/validators tested
- [ ] build/vet/gofmt clean
- [ ] plan/status/todo/handoff/changelog updated
