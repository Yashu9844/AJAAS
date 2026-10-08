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
