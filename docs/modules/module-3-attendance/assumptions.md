# Module 3 — Assumptions (P1 2026-10-08)

- A3-01 Module 2 `EmployeeService` contract (GetByUserID / GetByID / List) is stable as merged; employee statuses `active|probation|notice` mean "may work".
- A3-02 Module 0 `RequirePermission` + `tenant_admin` bypass semantics unchanged; JWT `user_id`/`tenant_id` set in Gin context by Module 0 middleware.
- A3-03 Shift timezone is the authority for dates/lateness; employees without a shift are evaluated in UTC.
- A3-04 One shift per employee per day (no split rosters in v1).
- A3-05 Module 4 will write `on_leave` only through contract C10; Module 3 never infers leave.
- A3-06 Dev schema comes from GORM AutoMigrate; SQL migrations 000025–000030 are the production path and must produce the same tables (verified at P7).
- A3-07 Live tests run on a machine with Docker Desktop + Go 1.27.1 (this machine as of 2026-10-08).
