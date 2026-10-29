# M2 webhooks, chat messages and git integrations - notes (phase 4)

> Decisions here are numbered 1-14 locally; the project-wide records are DPJ-41 to DPJ-44 in `decisions.md`.

Package `backend/internal/projects/webhooks`, migrations `000060_create_webhooks` and `000061_create_integrations`. Format of each decision: decision - reason - alternatives - trade-off. The lead merges these into `decisions.md` (suggested ids DPJ-W1..W12) and the table below into `security.md`.

## What was built

- **Outgoing webhooks** per project (max 20): generic JSON, Slack and Discord text. Events come from `projects_events_outbox`. Routes: list, create, patch, delete, rotate-secret, test ping, list deliveries, redeliver.
- **Git integrations** (GitHub, Gitea; max 10 per project): public `POST /integrations/{provider}/{integration_id}` (HMAC over the raw body), links PRs, branches and commits to tasks of THAT project by `KEY-123`, `GET /tasks/{id}/links`.
- 14 routes (13 guarded, 1 public). Permissions reuse `project.update` (manage) and `project.read` (list deliveries, task links need `task.read`); no M0 change.

## Decisions

1. **One SSRF-guarded sender is the only outgoing-HTTP exception to SECURITY section 9** - the feature needs the server to call user URLs; every call goes through `Sender.Do` (guard.go, client.go). Alternatives: a separate egress proxy (right for a hardened deployment, not available now). Trade-off: the guard is in-process, so a bug there is a bug in the app; hence the table tests and the second check at socket level.
2. **Validation at create/update AND at send; the address is checked on the connection that is actually made.** The host is resolved at dial time, every answer must be public (one blocked answer rejects the host), the socket is opened to the validated IP literal, and the dialer `Control` re-checks the connected address and the port. DNS rebinding therefore cannot change the destination between check and use. Alternative: resolve once at save time (rejected: TOCTOU). Trade-off: a host with mixed public/private answers is refused outright.
3. **Test-only relaxations are unexported** (`senderOptions`: http, loopback, any port, trusted test CA, short timeout, connect hook). Production uses `NewSender()` with the zero value; nothing reads them from configuration. `Service.SetSender` replaces the whole sender for tests (fake recorder).
4. **No redirects, 5 s total, 64 KB request, 4 KB response read and discarded, no proxy variables, https only, ports 443/8443, no userinfo, numeric-looking hosts (0x7f.1, 2130706433, 127.1) and internal names refused.** Errors stored per attempt are fixed short strings (`timeout`, `tls error`, `connection failed`, `redirect refused`, `destination address is not allowed`, `http 503`), never the raw Go error (which carries URL and IP).
5. **Secrets: AES-256-GCM with nonce prefix, key `projects.secrets_key`** (`PROJECTS_SECRETS_KEY`), 32 random bytes shown once (`whsec_` + 64 hex), only a 4-character hint readable. Development derives a key from the JWT secret; production requires an explicit key (`ValidateConfig`). An empty key does not stop M2: secret routes answer 503 `SECRETS_UNAVAILABLE` (same pattern as storage, DPJ-30). Alternative: hash the secret (impossible - HMAC signing needs the key). Rotating the encryption key needs a re-encrypt tool (open item).
6. **Dispatcher reads the outbox through its own cursor** (`projects_dispatch_cursor`, one row), independent of the relay's `published` flag. Because `created_at` is the transaction start time, a slower transaction can commit after a later one; each pass re-reads a 30 s lookback and the unique `(webhook_id, event_id)` index drops duplicates. Webhooks only see events created after the webhook (database clock, `created_at` omitted from the insert so it matches the outbox clock). Alternative: a sequence column on the outbox (touches the relay table contract). Trade-off: a transaction open longer than 30 s could be missed.
7. **One delivery row per (webhook, event); body is rebuilt at send time from the outbox envelope** (plus `delivery_id`; the optional `agent` label is forwarded unchanged). No payload copy is stored. Trade-off: if the outbox row is purged the attempt fails with `event unavailable`.
8. **Retries: 1 m, 5 m, 30 m, 2 h, 12 h with +-20 % jitter, 6 attempts then `dead`; 20 consecutive failed attempts disable the webhook** (`disabled_at`, audit `webhook.auto_disabled` as a system action). Success resets the counter. Re-enabling with `active: true` clears both. Test pings are never retried and never count. "Every attempt result" is the last result on the delivery row (status code, short error, attempt count); no per-attempt history table (smaller surface).
9. **Workers claim with `FOR UPDATE SKIP LOCKED` and a 3-minute lease** (next attempt pushed out before sending), so several instances never double-send and a crash only delays. No network I/O inside a transaction. Deliveries older than 30 days are purged hourly.
10. **Chat formats carry only event name, project KEY and ticket key** (`[JAAS] task.created in PRJ (PRJ-12)`); the event name must match `^[a-z][a-z0-9_.]*$`. No titles, names or comments can reach a chat room.
11. **Incoming route: one unauthenticated endpoint besides the calendar feed.** HMAC is checked over the RAW body before parsing (constant time; GitHub `sha256=<hex>` in `X-Hub-Signature-256`, Gitea hex in `X-Gitea-Signature`), body limit 1 MiB (413), per-IP limiter (120/min, shared Redis limiter when configured, in-process otherwise). Unknown id, malformed id, wrong provider, inactive integration, unreadable secret and bad signature all answer the identical 404, and an unknown id still runs a dummy HMAC (no timing oracle). Tenant and project come only from the integration row. Alternative: per-integration limit (rejected: an attacker would get a budget per guessed id).
12. **Links are idempotent and ids-only.** `external_links` upsert with `WHERE state IS DISTINCT FROM`: a provider retry changes nothing and emits nothing. New link: `task.external_linked`; first transition to merged for a PR: `task.pr_merged` (consumed later by the automation agent). Activity has source `integration`, data holds link id, provider, kind, state only. PR titles, branch names in activity/events, and commit messages are scanned and dropped, never stored; stored URLs are https-only and never fetched. Only tasks of the integration's project are linked (`KEY-123` of another project is ignored). Archived or deleted projects take no new links (200, ignored).
13. **Routes and permissions.** Listing webhooks and integrations needs `project.update` (a Slack/Discord URL is itself a secret); deliveries need `project.read`; redeliver and test need `project.update` (they cause an outgoing request); test pings are limited to 10 per minute per webhook.
14. **Event types** are validated by shape (`^[a-z][a-z_]*(\.[a-z][a-z_]*){1,3}$` or `*`), not against a closed list, so new M2 events work without a code change. Trade-off: a typo is accepted and simply never matches.

## Threat to evidence (merge into security.md)

| # | Threat | Mitigation | Evidence |
|---|---|---|---|
| W1 | SSRF through a webhook URL (loopback, private, metadata, numeric forms, IPv4-mapped IPv6) | https only, ports 443/8443, no userinfo, host rules at save and send, every resolved and connected address checked, no redirects, no proxy | `guard_test.go` (URL and IP tables), `client_test.go` (`TestSenderRefusesBadURLsAndAddresses`, `TestSocketControlChecksTheConnectedAddress`), integration `TestWebhooksThroughTheRouter` |
| W2 | DNS rebinding between check and use | the connection goes to the validated IP literal; control re-checks the connected IP | `TestSenderStopsDNSRebinding` (fake resolver, one connection only) |
| W3 | Redirect to an internal address | redirects are never followed, a 3xx is a failure | `TestSenderRefusesRedirects` |
| W4 | Slow or huge receivers exhaust the worker | 5 s total timeout, 64 KB request, 4 KB response read, 5 parallel sends | `TestSenderTimeout`, `TestSenderSizeLimits` |
| W5 | Secret disclosure | AES-256-GCM at rest, shown once, hint only, never in logs, audit, events, errors or list responses | `TestSealerRoundTripAndTamper`, `TestWebhookLifecycle`, `TestWebhookRoutes` |
| W6 | Forged or replayed provider call | HMAC over raw body in constant time before parsing; signature header formats strict | `TestSignatureMatches`, `TestIncomingSignatureAndNoOracle`, integration `TestGitIntegrationThroughTheRouter` |
| W7 | Integration id or secret oracle | identical 404 body for every rejection, dummy HMAC for unknown ids | `TestIncomingSignatureAndNoOracle` |
| W8 | Cross-project or cross-tenant linking | tenant and project from the integration row; task lookup scoped by tenant and project | `TestGitHubPullRequestFlow` (OTH-1 in the title does not link) |
| W9 | Personal data in events, activity, links or chat | ids and state only; chat text from keys only; titles and messages dropped | `TestGitHubPullRequestFlow`, `TestChatFormatsCarryOnlyKeys`, `TestSendSignedGenericBody` |
| W10 | Public endpoint abuse | 1 MiB body limit, per-IP limiter, deny by default for inactive integrations | `TestIncomingSignatureAndNoOracle` (413), `TestIncomingRateLimitPerIP` |
| W11 | Event loss or duplicate delivery | own cursor with lookback, unique (webhook, event), lease claim with SKIP LOCKED | `TestDispatchMatchesProjectAndEventTypes`, `TestDispatchLookbackCatchesLateCommits`, `TestParallelWorkersSendEachDeliveryOnce` |
| W12 | Silent database failure | every statement of the main flows failed in turn must return an error | `faults_test.go` (DPJ-24) |
| W13 | Runaway failing receiver | backoff, dead after 6, webhook disabled after 20 consecutive failures, audit entry | `TestRetriesBackOffThenDie`, `TestTwentyFailuresDisableTheWebhook` |
| W14 | Using test pings as a request cannon | 10 pings per minute per webhook, `project.update` required | `TestPingIsRecordedAndNeverRetried` |

## Open items

- Re-encryption tool for `projects.secrets_key` rotation.
- No per-attempt history (last result only).
- Swagger generator documents the public provider route with a `text/calendar` response and types the `{provider}` path parameter as uuid (the generator hard-codes both); the runtime answers JSON. `gen_projects_swagger.py` knows only error codes 400, 403, 404, 409, 429, so 413 and 503 are described in the summaries.
