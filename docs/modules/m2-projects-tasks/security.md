# M2 Projects & Tasks - security: threat to evidence

Law: `docs/SECURITY.md` (sections 1, 5, 9, 13, 14). This table is the Definition-of-Done security row for M2.

| # | Threat | Mitigation | Evidence |
|---|---|---|---|
| S1 | Cross-tenant access (T6) | `tenant_id` on every M2 table and every query; tenant from Host + verified token only; foreign ids answer 404 | `TestProjectsContext` (foreign tenant 404), `TestAccessRules`, per-context isolation tests, PG4 |
| S2 | Reading a private project or its tasks as a non-member | One `Authorizer` resolves tenant permission + effective membership + tenant visibility; unreadable -> 404 (no existence oracle) | `project/access.go`, `TestAccessRules`, `TestTenantVisibilityAndArchivedFreeze` |
| S3 | Privilege escalation through roles | Only owners/tenant admins grant `owner`; custom roles need `project_roles:manage`; group rules cannot grant `owner` (DB check); last owner cannot be removed | `TestMembersLifecycle`, `TestLastOwnerGuard`, `TestCustomRoles`, `project_members_group_not_owner` |
| S4 | Object-level bypass (task, comment, attachment, time id) | Every id is resolved through its project (`api.TaskResolver`); the attachment id alone never authorises a download | PG4, PG8 collab tests |
| S5 | Lost updates / races | `version` optimistic concurrency, row locks, advisory locks (columns, dependency graph), partial unique indexes (one active sprint, one running timer), gapless ticket counter | PG2, PG3, PG5, PG6 |
| S6 | Injection (SQLi) | GORM parameters and `?` placeholders only; sort and filter keywords from fixed allow-lists; ids passed as bind parameters | code review, planning/task list tests |
| S7 | Stored XSS | JSON-only API (no HTML rendering); the console renders text nodes; markdown is data | SECURITY section 9 |
| S8 | Unsafe file upload | 50 MB cap, per-tenant quota, content-type allow-list, executable extensions refused, sanitised filename, size/type/checksum verified on `complete`, scan hook before `ready`, private bucket, presigned URLs (5 min up, 2 min down) | collab attachment tests, real S3 round trip |
| S9 | SSRF | M2 never fetches user-supplied URLs; storage endpoints come from configuration only | no outbound HTTP in `internal/projects` |
| S10 | Secrets in code or logs | Storage credentials come from config/environment (dev values only in `development.yaml`); calendar tokens are random 32 bytes, only the SHA-256 hash is stored and shown once | `configs/*.yaml`, planning token tests |
| S11 | Public endpoint abuse | The only token-authenticated route is the iCalendar feed: tenant is derived from the token row, only the owner assigned tasks in readable projects are listed, owner must still be active, revocable, rate limited (30/min/IP, shared Redis limiter when Redis is configured, per process otherwise) | planning feed tests |
| S12 | Personal data in events or audit | Events and audit hold ids, versions and codes only; field values only in activity (old/new) and never free text in events | PG12 tests per context |
| S13 | Repudiation | Every mutation writes an audit entry (M0 trail) and an append-only activity row (DB trigger blocks `UPDATE`) | `faults_test.go`, activity tests |
| S14 | Replay / duplicate creation | Global `Idempotency-Key` middleware on writes; inbox ledger for consumed events | PG1, PG10 |
| S15 | Departed users keep access | Deactivation hook marks memberships and assignments inactive in the same transaction; group rules refresh from HR | `TestDeactivationAndInbox`, `TestProjectGroupRulesFollowHR` |
| S16 | Silent database failure treated as "not found" | `countRows` helper; statement-level fault injection fails every SQL statement of the main flows | `faults_test.go` (DPJ-24) |
| S17 | Resource exhaustion | Page size <= 100, bulk <= 100, assignees <= 20, mentions and graph walks bounded (10 000 nodes), calendar window <= 62 days, Gantt <= 500 tasks, body limit 1 MiB | validation tests per context |
| S18 | Permission model change | Four tenant-level entries registered in the M0 catalogue (`projects:create`, `projects:read_all`, `projects:manage_all`, `project_roles:manage`) with predefined-role defaults; project-level permissions live inside M2 | `projects.Catalogue()`, `docs/modules/m2-projects-tasks/decisions.md` |

| S19 | Stored URLs or user ids in custom fields | URL values must be https without credentials and are never fetched; user values must be active members of the project; text is length-bounded; activity records text and URL fields by name only | `fields/validate.go`, `TestValuesByTypeAndVersion`, `TestOptionRemovalClearsValuesAndActivityHidesText` |

Open security items: no malware scanner behind the scan hook (no scanner exists in the platform), platform-wide gaps in
`docs/modules/m1-hr-management/acceptance-checklist.md` (secret manager, tamper-resistant audit, backups) apply to M2 as well.
