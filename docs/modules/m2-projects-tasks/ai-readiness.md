# M2 Projects & Tasks - AI readiness

M2 contains **no AI**. It contains the seams a separate AI module (M9) needs, so that everything a person can read or do in M2 can later be
answered or performed by an assistant acting for that person - with exactly that person's permissions. The AI side of the design is in
`docs/modules/m9-ai-assistant/ai-integration.md`; this file is the M2 half.

## What exists (built 2026-10-29)

| Seam | Where | What it guarantees |
|---|---|---|
| **REST API with RBAC** | all M2 routes | One authorization path. An assistant uses the user token; the API refuses what the user may not do. |
| **Agent attribution** | `X-JAAS-Agent` header -> `kernel.WithAgent`; middleware `internal/projects/agent_mw.go` | Activity rows written during the request get `source = ai` and `data.agent`; outbox events get a top-level `agent`. A label never widens permissions; an invalid label is ignored. |
| **Tool catalogue** | `GET /ai-hooks/tools[?project_id=]`, `internal/projects/aihooks/tools.go` | 17 tools mirroring existing routes, each with schema, route, required permission, risk (`read`/`write`/`destructive`), confirmation flag and idempotency flag. With `project_id` only the tools the caller may use there are listed. `TestEveryAIToolRouteExists` keeps it in step with the router. |
| **Context packs** | `GET /ai-hooks/context/projects/{id}`, `GET /ai-hooks/context/tasks/{id or KEY-12}` | Compact, capped, permission-filtered summaries with `approx_tokens` and a `truncated` list; relations into projects the caller cannot read are omitted. |
| **Document feed** | `GET /ai-hooks/documents?project_id=&kind=task|comment&cursor=` | Incremental text for indexing: stable ids, content hash, `updated_at`, tombstones for deletions (30 days), text capped at 4000 characters. Needs `task.read`. |
| **Readable scope** | `GET /ai-hooks/readable-projects` | Ids the caller can read, to post-filter retrieval hits per user. |
| **Events** | outbox `jaas.projects.events`, webhooks | Ids and versions only (PG12), `agent` label when an AI acted. |
| **MCP server and CLI** | `backend/cmd/jaas-pm-mcp`, `backend/cmd/jaas-pm`, libraries `pkg/pmclient`, `pkg/pmmcp` | Any MCP client can drive M2 as the signed-in user. Results are compact (projection, truncation, caps, cursors, `detail` flag) for token economy. |
| **Idempotency and versions** | `Idempotency-Key`, `version` / `If-Match` | Retried tool calls are safe; concurrent edits by a person and an agent cannot overwrite each other silently. |

## Rules these seams follow

1. Hooks are read-only; there is no AI-only write path.
2. Unreadable means 404. The catalogue, packs and feed never confirm that something exists.
3. Everything is bounded: no unbounded lists, no whole histories, no unlimited text.
4. No model, prompt, provider or key lives in M2. M2 never calls a model.
5. Text from users (titles, descriptions, comments) is data. The MCP server states this in its `instructions`; M9 must treat it as untrusted.

## How M9 will use them (summary; full design in the M9 note)

`tools` -> offered to the model, filtered by project; `context` -> grounding for the object in view; `documents` -> retrieval index kept current by cursor;
`readable-projects` -> per-user filter on retrieval hits; REST routes with the user token and `X-JAAS-Agent` -> execution; confirmation for `destructive`/`confirm` tools.

## Why this shape (decision DPJ-37)

Plane keeps AI out of its core: a thin proxy endpoint behind the normal role checks, per-user API tokens for agents, and a separate MCP/assistant service on the public
API. JAAS adopts the same separation and adds what Plane lacks inside the modules: a self-describing tool catalogue, bounded context packs, a permission-aware
document feed and agent attribution. Alternatives rejected: (a) an `/ai` endpoint inside M2 that calls a model (couples M2 to providers and secrets, duplicates M9);
(b) giving M9 direct database reads (bypasses RBAC); (c) a service account with broad rights (turns prompt injection into privilege escalation).

## Gaps and follow-ups

- **Long-lived agent credentials.** CLI/MCP currently sign in with e-mail and password and refresh the JWT. Per-user, scoped, revocable API tokens (Plane-style) need an M0 change; deferred as a permission-model decision (see the M9 note, section 5).
- The MCP server's tool list is hand-written in `pkg/pmmcp/catalogue.go`; the server catalogue (`aihooks.Catalogue`) is the intended single source. Converging them (the MCP server fetching `/ai-hooks/tools`) is a small follow-up.
- Packs cover projects and tasks only (sprints, saved views and time reports are reachable through normal tools).
- Other modules (HR, Payroll) need the same four hooks plus per-field redaction rules before any assistant touches them.
