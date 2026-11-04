# M3 Meetings - security: threat to evidence

The evidence column names the test that proves each mitigation. Each test is written in the build step that adds its
feature (`plan.md`), and this table is updated with its path then. Threats in the style of `m2-projects-tasks/security.md`.

| # | Threat | Mitigation | Evidence (planned) |
|---|---|---|---|
| T1 | Cross-tenant access | `tenant_id` on every table and query; tenant from the token only; ids from another tenant answer 404 | cross-tenant table test on every route |
| T2 | Reading a private or confidential meeting, or learning that it exists | Authorizer plus 404; `busy_only` shows start and end only; confidential titles never in events, feeds or notifications | role matrix test; event payload test; feed test |
| T3 | Private talking points leak | Owner-only rows, no override (admin, `read_all`, context pack, search, export, events) | isolation test per surface (J7) |
| T4 | Privilege escalation through meeting roles | M0 resource roles; assigning needs `participant:manage`; organizer only by an organizer or `manage_all`; last-organizer guard; group rules never grant organizer | role assignment tests |
| T5 | Object-level bypass through child ids (agenda, notes, decision, action, attendance) | every child is resolved through its occurrence and series before the check | child-id tests with a foreign series |
| T6 | Action item writes into a project the caller cannot write | the M2 port runs as the caller; M2 checks `task:create`; linking needs `task:read` | port tests plus integration with real M2 |
| T7 | Lost updates | `version` and If-Match; notes versioned per section; append-only triggers on notes and minutes | concurrency and trigger tests |
| T8 | Recurrence denial of service | whitelist, COUNT ≤ 500, UNTIL ≤ 5 years, 12-month window, tenant cap, 62-day list window | rule fuzz and bounds tests |
| T9 | Malicious iCalendar | phase 1 only writes iCalendar; parsing (phase 3) is fuzzed with a 1 MiB limit; URL helpers banned | lint grep for `ParseCalendarFromUrl` |
| T10 | Feed token abuse | 32-byte random token, hash stored, revocable, rate limit, owner must be active | feed tests |
| T11 | SSRF through the video URL | https only, stored, never fetched (SECURITY.md section 9) | validation tests; no HTTP client in the package (import test) |
| T12 | Stored XSS in markdown | JSON API only; the client renders as text or sanitised | UI rule (as M2 notes-ui) |
| T13 | Recording without consent | `recording_allowed` plus per-participant consent, which M9 must check | consent API tests |
| T14 | Personal data in events or audit | ids and codes only; the attendance report is permission-gated | event payload test |
| T15 | Departed users keep access | the deactivation hook removes participation, roles and tokens | deactivation test |
| T16 | Auditor overreach | `read_all` excludes drafts and private points; confidential needs a second permission; reads audited | auditor tests plus audit row assertion |
