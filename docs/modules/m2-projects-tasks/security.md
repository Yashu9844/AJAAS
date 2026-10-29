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

| S20 | An AI agent exceeding its user rights | Agents use the user token; the API is the only authorization path; hooks are read-only; the catalogue lists only tools the caller may use; an invalid or valid `X-JAAS-Agent` label never changes permissions | `TestAIHooksFollowTheCallerPermissions`, `TestToolsAreFilteredByWhatTheUserMayDo` |
| S21 | Data leaking to a model through context packs or the feed | Packs and feed are authorised per project, omit relations into unreadable projects, cap text, and need `project.read` / `task.read`; retrieval hits must be filtered with `readable-projects` | `TestTaskContextByKeyHidesUnreadableRelationsAndCapsText`, `TestDocumentsFeedIsIncrementalWithTombstones` |
| S22 | Prompt injection through task text | Text is data: the MCP server says so in `instructions`, results are projected and capped, write/destructive tools are flagged for confirmation in the catalogue; M9 enforces the gate | `pkg/pmmcp` tests, `aihooks.Catalogue` risk/confirm fields |
| S23 | Stolen CLI/MCP credentials | Config file is owner-only (0600), passwords are never stored, tokens are the normal short-lived JWT plus refresh | `pkg/pmclient/config.go` |
| S24 | A rule used to escalate: a member sets up a rule that does more than they may | The rule runs as its creator through the task use cases; permissions are re-checked per run; no new permission; a removed or demoted creator is skipped and flagged | `TestNotAllowedCreatorIsSkippedAndFlagged`, `TestRemovedCreatorIsNotAllowed`, `TestRuleHonoursWipLimitsLikeAPerson` |
| S25 | Rule loops and floods | Agent label `automation.*` ignored by the engine; 100 actions per rule per minute; 20 consecutive failures disable; max 50 rules and 10 actions | `TestRuleMovesUrgentTaskAndDoesNotCascade`, `TestSelfTriggeringRuleStopsAfterOneRun`, `TestFloodCapSkipsRun`, `TestRuleDisablesAfterTwentyFailures` |
| S26 | Event redelivery repeats side effects | Unique `(rule_id, event_id)` claimed before the actions | `TestRedeliveredEventIsAppliedOnce` |
| S27 | Rule definitions that reach other projects or run code | Closed vocabulary; column, label and member ids checked against the project; extra members on an action refused | `TestRuleValidation` |
| S28 | A dry run with side effects | The test endpoint reads only; no run, no event, no task call | `TestDryRunChangesNothing` |
| S29 | Recurrence creating tasks without access, in archived projects or twice | Creator re-check, archived flag, `FOR UPDATE SKIP LOCKED` claim | `TestArchivedProjectAndLostAccessAreFlagged`, `TestConcurrentTicksNeverDuplicate`, `TestMissedRunsCollapseIntoOneCatchUp` |
| S30 | Public link leaks people or private data | Strict allow-list structs, mention scrubbing, description opt-in, tenant and project from the token row | `TestPublicViewIsAStrictAllowList`, `TestOnlyTheTokenProjectIsReadable`, integration `TestAutomationRecurrenceAndPublicLinksThroughTheRouter` |
| S31 | Token guessing, reuse after revocation, existence oracle | 256-bit token, hash only stored, constant-time compare, revoke is immediate, identical 404 bodies, per-IP rate limit, no-store/noindex/no-referrer | `TestUnknownRevokedMalformedAndHiddenProjectsAnswerIdentically`, `TestPublicRouteIsRateLimited`, `TestLinkLifecycleAndPermissions` |
| S32 | Silent database failures | Statement-level fault injection over every flow | `TestEveryStatementFailureIsReported` in each package |
| S33 | SSRF through a webhook URL (loopback, private, metadata, numeric forms, IPv4-mapped IPv6) | https only, ports 443/8443, no userinfo, host rules at save and send, every resolved and connected address checked, no redirects, no proxy | `guard_test.go` (URL and IP tables), `client_test.go` (`TestSenderRefusesBadURLsAndAddresses`, `TestSocketControlChecksTheConnectedAddress`), integration `TestWebhooksThroughTheRouter` |
| S34 | DNS rebinding between check and use | the connection goes to the validated IP literal; control re-checks the connected IP | `TestSenderStopsDNSRebinding` (fake resolver, one connection only) |
| S35 | Redirect to an internal address | redirects are never followed, a 3xx is a failure | `TestSenderRefusesRedirects` |
| S36 | Slow or huge receivers exhaust the worker | 5 s total timeout, 64 KB request, 4 KB response read, 5 parallel sends | `TestSenderTimeout`, `TestSenderSizeLimits` |
| S37 | Secret disclosure | AES-256-GCM at rest, shown once, hint only, never in logs, audit, events, errors or list responses | `TestSealerRoundTripAndTamper`, `TestWebhookLifecycle`, `TestWebhookRoutes` |
| S38 | Forged or replayed provider call | HMAC over raw body in constant time before parsing; signature header formats strict | `TestSignatureMatches`, `TestIncomingSignatureAndNoOracle`, integration `TestGitIntegrationThroughTheRouter` |
| S39 | Integration id or secret oracle | identical 404 body for every rejection, dummy HMAC for unknown ids | `TestIncomingSignatureAndNoOracle` |
| S40 | Cross-project or cross-tenant linking | tenant and project from the integration row; task lookup scoped by tenant and project | `TestGitHubPullRequestFlow` (OTH-1 in the title does not link) |
| S41 | Personal data in events, activity, links or chat | ids and state only; chat text from keys only; titles and messages dropped | `TestGitHubPullRequestFlow`, `TestChatFormatsCarryOnlyKeys`, `TestSendSignedGenericBody` |
| S42 | Public endpoint abuse | 1 MiB body limit, per-IP limiter, deny by default for inactive integrations | `TestIncomingSignatureAndNoOracle` (413), `TestIncomingRateLimitPerIP` |
| S43 | Event loss or duplicate delivery | own cursor with lookback, unique (webhook, event), lease claim with SKIP LOCKED | `TestDispatchMatchesProjectAndEventTypes`, `TestDispatchLookbackCatchesLateCommits`, `TestParallelWorkersSendEachDeliveryOnce` |
| S44 | Silent database failure | every statement of the main flows failed in turn must return an error | `faults_test.go` (DPJ-24) |
| S45 | Runaway failing receiver | backoff, dead after 6, webhook disabled after 20 consecutive failures, audit entry | `TestRetriesBackOffThenDie`, `TestTwentyFailuresDisableTheWebhook` |
| S46 | Using test pings as a request cannon | 10 pings per minute per webhook, `project.update` required | `TestPingIsRecordedAndNeverRetried` |

Open security items: no malware scanner behind the scan hook (no scanner exists in the platform), platform-wide gaps in
`docs/modules/m1-hr-management/acceptance-checklist.md` (secret manager, tamper-resistant audit, backups) apply to M2 as well.
