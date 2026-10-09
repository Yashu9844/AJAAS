# Module 3 — Security (law = docs/SECURITY.md)

## Data classification
- Attendance times, coordinates, device ids: personal data (Medium). IP address: Medium, stored for audit only, never returned or published.
- No credentials, tokens or statutory data in Module 3 tables.

## Permissions (seeded at boot, idempotent — connections C7)
| Permission | Grants |
|---|---|
| `attendance:read` | admin record list/detail, summary, all regularizations list, shifts list/get, assignment list |
| `attendance:manage` | shift create/update/deactivate, shift assignments |
| `attendance:approve` | approve / reject regularizations |
| (none — authenticated employee) | punch, own today/records, own regularization create/list/cancel |

`tenant_admin` bypass from Module 0 RequirePermission applies unchanged.

## Threats & mitigations
| ID | Threat | Mitigation |
|---|---|---|
| AS-T1 | Buddy punching / backdating via client clock | AT-001 server clock only; no time field in PunchRequest |
| AS-T2 | Punching for another employee | NFR-SEC002: employee resolved from JWT user only |
| AS-T3 | Cross-tenant reads (IDOR) | every repo call tenant-scoped; foreign id → 404 (G1) |
| AS-T4 | Self-approval of corrections | AT-016 → 403 SELF_APPROVAL_FORBIDDEN (G7) |
| AS-T5 | Punch flooding | AT-004 60 s debounce + Redis IP rate limit 30/min on A1 |
| AS-T6 | Race producing duplicate sessions | AT-022 advisory lock (EC-15) |
| AS-T7 | Location/IP leakage via events | FR-EV003 payload whitelist |
| AS-T8 | Repudiation of corrections | supersede not delete (AT-019) + audit rows for every state change |
| AS-T9 | Mass assignment | DTOs whitelist fields; `is_night_shift`, `status`, totals never client-set |

## Audit actions (Module 0 AuditService, after commit)
`attendance.punch_in`, `attendance.punch_out`, `attendance.regularization_requested`, `attendance.regularization_cancelled`, `attendance.regularization_approved`, `attendance.regularization_rejected`, `shift.created`, `shift.updated`, `shift.deactivated`, `shift.assigned`. Metadata: ids, dates, statuses — no coordinates, no IP in metadata (IP/UA go in the audit columns).

## Error hygiene
Bind/validation errors return `VALIDATION_ERROR` with field details (no raw Go error text). Unexpected errors → `INTERNAL_ERROR` only.

## Threat → evidence (verified 2026-10-09, P9)
| Threat | Evidence |
|---|---|
| AS-T1 | unit G3 (services/punch_golden_test.go); live attendance_race_test.go (client punch_time ignored); controllers SelfEndpointsIgnoreClientIDs |
| AS-T2 | controllers SelfEndpointsIgnoreClientIDs; routes self endpoints carry no id params; live G9 member /me sees only own rows |
| AS-T3 | live G1 (attendance_rules_test.go): 5 foreign ids → 404, foreign JWT → 403, list → 0 rows |
| AS-T4 | unit + live G7 |
| AS-T5 | live AT-004 debounce (attendance_flow_test.go); IP limiter wired in module.go (30/min, shared RateLimiter) |
| AS-T6 | live attendance_race_test.go: 10 concurrent INs → 1×201, 9×409, 1 punch row, 1 record |
| AS-T7 | unit G12 + live (outbox payload has no latitude/device/ip) |
| AS-T8 | unit G6 (supersede) + live G6 (regularization punches, superseded kept) + G11 audit rows |
| AS-T9 | DTO whitelist (dto.go); controllers ignore unknown fields; status/totals/is_night_shift server-computed |

Pre-production checklist: docs/SECURITY.md §12 — Module 3 items (tenant-scoped queries, DTO validation, parameterized queries, no panic, structured logs) verified at P9; JWT/bcrypt/MFA/CORS/TLS items are Module 0 / deploy scope.
