# Module 4 — Security

Law: docs/SECURITY.md + Module 0 security.md (STRIDE T1–T10). Module specifics below.

## Permissions
`leave:read` (lists, balances of others, ledger), `leave:manage` (types, holidays, adjustments), `leave:approve` (approve/reject). Self endpoints and type/holiday reads need authentication only. Seeded at boot (C7).

## PII
`reason` and `review_comment` are free text (may contain health data, e.g. sick leave) → never in events (FR-EV003) or audit metadata; returned only to the owner and holders of `leave:read`.

## Threats
| ID | Threat | Control |
|---|---|---|
| LS-T1 | Applying for another employee | employee from JWT only; no employee_id on self endpoints |
| LS-T2 | Cross-tenant IDOR | tenant-scoped repos; foreign ids → 404 |
| LS-T3 | Self-approval | LV-010 |
| LS-T4 | Balance race / overdraw | LV-014 advisory lock + available check in the same tx |
| LS-T5 | Silent balance tampering | append-only ledger with actor + kind (LV-013, LV-016); adjustments need `leave:manage` + reason |
| LS-T6 | Sensitive reason leakage | FR-EV003 whitelist payloads; audit metadata ids/dates/days only |
| LS-T7 | Mass assignment | DTO whitelists; status/total_days/balances server-computed |
| LS-T8 | Backdating abuse | LV-005 window (30 days, only types with min_notice 0) |

## Error hygiene
Validation → VALIDATION_ERROR with field details; unexpected → opaque INTERNAL_ERROR.

## Threat → evidence (verified 2026-10-09, P9)
| Threat | Evidence |
|---|---|
| LS-T1 | controllers TestEndpoints_SelfIgnoresClientIdentity (apply/balances ignore client ids); ApplyLeaveRequest has no employee field |
| LS-T2 | live TestLeaveLive_G1_Isolation: 5 foreign ids → 404, foreign type on apply → 404, foreign employee balance → 404, foreign JWT → 403 |
| LS-T3 | unit G10 + live G10 (admin self-approval → 403, stays pending) |
| LS-T4 | live G14: 5 parallel applies on balance 5 → exactly 2 succeed, available 1 (advisory lock `leave:`) |
| LS-T5 | live G12 ledger invariant (projection == ledger sums per kind); ledger repo exposes Create/List only; adjustments need leave:manage (G9) |
| LS-T6 | unit G11 + live G11: no reason/comment/note text in audit metadata or outbox payloads |
| LS-T7 | DTO whitelists; status/total_days/balances server-computed (unit SelfIgnoresClientIdentity sends status/total_days, ignored) |
| LS-T8 | unit TestApply_Rules (backdate window, notice) + validators NoticeOK |

Pre-production checklist (docs/SECURITY.md §12): Module 4 items — tenant-scoped queries, DTO validation, parameterized queries, no panic, structured logs — verified at P9; JWT/bcrypt/MFA/CORS/TLS are Module 0 / deploy scope.
