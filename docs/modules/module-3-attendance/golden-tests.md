# Module 3 — Golden Tests (FROZEN at P1 2026-10-08 — protected from P3 on)

Rules: MASTER_PROMPT §15. Never edit a golden to make red pass. Change = reason → spec update → contract version → dependent update → full suite.
Two rings: **unit goldens** (no infra, always run by `go test ./internal/...`) and **live goldens** (`//go:build integration`, real backend + Docker PG).

| # | Golden behavior | Input | Expected | Why | Location |
|---|---|---|---|---|---|
| G1 | Tenant isolation | Tenant B admin reads tenant A record / shift / regularization ids; tenant A JWT on B subdomain | 404 each; 403 on subdomain mismatch | Isolation is the boundary | tests/api/attendance_isolation_test.go |
| G2 | Punch alternation | in, in, out, out | 201, 409 ALREADY_PUNCHED_IN, 201, 409 NOT_PUNCHED_IN | Session integrity | services/punch_golden_test.go (+ live in attendance_flow_test.go) |
| G3 | Server clock only | punch with extra `punch_time` field in body | ignored; punch_time = server now | Anti-backdating | services/punch_golden_test.go |
| G4 | Totals engine | fixed shifts × punch sets (day, late, break, overtime, half-day, absent, no shift) | exact work/break/late/overtime/status table | Pay depends on it | calc/calc_golden_test.go |
| G5 | Night shift attribution | 22:00–06:00 shift, IN 21:55 D / 01:00 D+1, OUT 06:10 D+1, Asia/Kolkata tz | record date D, work 495, late 0 | Cross-midnight correctness | calc/calc_golden_test.go |
| G6 | Regularization lifecycle | request → approve | record regularized, old punches superseded, totals from requested window, event + audit | Correction flow | services/regularization_golden_test.go + live |
| G7 | Self-approval forbidden | approver approves own request | 403 SELF_APPROVAL_FORBIDDEN, request still pending | Segregation of duties | services/regularization_golden_test.go + live |
| G8 | One pending per date | second request same date | 409 REGULARIZATION_PENDING | No conflicting corrections | services/regularization_golden_test.go |
| G9 | RBAC | member (no attendance perms) on records/summary/shifts; same member on self endpoints | 403; 200/201 | Least privilege | tests/api/attendance_rbac_test.go |
| G10 | Assignment overlap | open-ended A from Jan 1, new B from Mar 1; then C overlapping B | A closed Feb 28, B created; C 409 ASSIGNMENT_OVERLAP | Single active shift | services/assignment_golden_test.go |
| G11 | Audit on every state change | punch, regularize, approve, shift create/assign | audit row per action, no IP/coords in metadata | Repudiation defense | services (spy) + live |
| G12 | Outbox atomic with state | punch | outbox row in same tx; relay publishes; payload has no ip/device/coords | No lost/ghost events, privacy | services/outbox_golden_test.go + live |
| G13 | Pagination never crashes | per_page=0, page=-1, per_page=abc | 200 with defaults | Fixes Module 0 crash class | controllers + live |
