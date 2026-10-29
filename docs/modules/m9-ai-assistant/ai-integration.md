# M9 AI Assistant - how AI plugs into JAAS modules (integration design)

**Status:** design note, written 2026-10-29 while building M2 Projects & Tasks. M9 itself is NOT STARTED; this file records the
seams that already exist in M2 and the rules M9 must follow, so AI can be added later without changing any module's contract.
It refines, and does not replace, `specification.md` (the proposed M9 contract).

## 1. Principle: the AI is a user, never an authority

1. **One authorization path.** The assistant calls module REST APIs with the **signed-in user's own token**. The API checks that
   user's permissions on every call, exactly as it does for the web console, the CLI or the MCP server. The assistant has no
   service account with extra rights, no database access and no side door. A prompt-injected or malicious model can therefore never
   do more than the user may do.
2. **Modules stay separate.** Each module (M2 Projects, M1 HR, ...) keeps its own data, RBAC and API. AI is its own module (M9).
   Modules expose *hooks* (below); M9 composes them. Nothing in a module imports M9 and no module knows which model is used.
3. **Everything an AI does is traceable.** Calls made for an AI carry `X-JAAS-Agent: <label>`; M2 stamps it on activity (`source = ai`,
   `data.agent`) and on outbox events (top-level `agent`). Idempotency keys make retried tool calls safe.
4. **Text from the system is data, not instructions.** Task titles, comments, descriptions, documents and tool results are untrusted
   input to the model (prompt-injection defence is M9's job; modules never put instructions into their payloads).

## 2. What a module offers to AI (the hook contract)

A module that wants to be AI-ready offers four things; M2 is the reference implementation (`docs/modules/m2-projects-tasks/ai-readiness.md`).

| Hook | Purpose | M2 endpoint |
|---|---|---|
| **Tool catalogue** | Machine-readable list of the actions the module offers: name, description, JSON input schema, the REST route behind it, the permission it needs, risk class (`read` / `write` / `destructive`), whether the runtime should ask for confirmation, whether an `Idempotency-Key` applies. Filtered by what the caller may do in a given project. | `GET /ai-hooks/tools[?project_id=]` |
| **Context packs** | A compact, bounded, permission-filtered summary of an object (a project, a task) that fits a model context, with `approx_tokens` and the sections that were cut. | `GET /ai-hooks/context/projects/{id}`, `/tasks/{id|KEY-12}` |
| **Document feed** | Incremental text feed (create/update/delete tombstones, content hash, cursor) for building a retrieval index. | `GET /ai-hooks/documents?project_id=&kind=task|comment` |
| **Readable scope** | The ids the caller may read, so retrieval results from an index can be filtered to what *this user* may see before they reach the model. | `GET /ai-hooks/readable-projects` |

Plus the existing, non-AI surfaces: the REST API (swagger), events (outbox, ids and versions only), webhooks, the CLI and the MCP server.

Rules for any module adding these hooks:

- Hooks are **read-only**. Actions go through the normal REST routes so there is one write path with one set of checks.
- Hooks authorise through the module's own `Authorizer`; an unreadable object is **404**, never "forbidden" (no existence oracle).
- Packs have **caps** (counts, characters) and report what they cut; they hold ids and short excerpts, not whole histories.
- A test must prove every catalogue route exists in the router (M2: `TestEveryAIToolRouteExists`).

## 3. M9 runtime design

### 3.1 Request flow (one assistant run)

1. The user sends a message (`POST /api/v1/ai/conversations/{id}/messages`). M9 authenticates the user (same JWT) and keeps the user token
   in memory for the run only.
2. **Plan context.** M9 fetches the tool catalogues of the modules relevant to the conversation (filtered by project when one is in
   scope) and builds the model's tool list from them. Tools the user cannot use are never offered.
3. **Ground the model.** M9 fetches context packs for the objects in scope (the project or task the user is looking at) and, if
   retrieval is enabled, queries its index and **post-filters hits with `readable-projects`**.
4. **Model loop.** M9 calls the model through the provider-agnostic client (3.2), executes requested tools by calling the module REST
   API with the user token and `X-JAAS-Agent: m9.<assistant>`, feeds compact results back, and repeats until the model answers or a limit is hit.
5. **Gate risky tools.** `write` tools run directly only when the conversation is in a mode that allows it; `destructive` tools and any tool with
   `confirm: true` are turned into a **proposal** shown to the user; the call happens only after an explicit user confirmation
   (stored as a `PolicyDecision`). Bulk operations and anything affecting several people are proposals.
6. **Record.** Run, tool calls, policy decisions and usage are stored (spec entities); raw prompts and sensitive payloads are not published as events.

### 3.2 Model-agnostic client ("Open Responses")

The model must be swappable by configuration (OpenAI, Anthropic, Gemini, local servers such as Ollama or vLLM, gateways such as LiteLLM or
OpenRouter). Plane hard-codes a provider/model catalogue in its AI proxy; JAAS should not.

- **One internal interface**, `ModelClient`: `Respond(ctx, Request) -> Response` (and a streaming variant), where `Request` holds instructions, input items
  (messages, tool results), tool definitions (JSON schema), limits and a `store=false` privacy default. The shapes follow the **Responses-style** protocol
  (input items, output items, function calls) because it models tool use, reasoning items and multi-step state more cleanly than chat messages.
  The open specification of that shape ("Open Responses") should be checked and pinned when M9 enters design; its version goes into `decisions.md`.
- **Adapters**: (1) Responses-compatible HTTP (`POST {base_url}/responses`), (2) Chat Completions-compatible (`/chat/completions`) as the fallback for servers
  that only speak that, (3) provider-native adapters only if a feature needs it. A capability probe per configured model records what it supports
  (tools, streaming, JSON mode, context window) so the runtime degrades instead of failing.
- **Configuration, not code**: tenant/instance settings hold `base_url`, `model`, `api_key` (secret manager, never in the database in clear),
  context window, price per token, timeouts, allowed data classes. Several named profiles (for example `fast`, `smart`, `local`) can be bound to features.
  `base_url` is **admin-configured only**, never user-supplied, and is subject to the same outbound-call guard as webhooks (https, no private ranges, no redirects)
  unless an operator explicitly allows an internal address for a self-hosted model.
- **No vendor lock in the data model**: conversations store provider-neutral items; provider-specific ids are optional metadata.

### 3.3 Context and token management

- **Budget first.** Each run has a budget (context window minus reserved output). Everything that enters the prompt is counted by an estimator
  (tokenizer when available, `chars/4` otherwise) and checked against the budget before the call.
- **Layered context, cheapest first:** system rules (small, stable, cacheable) -> tool list (filtered, short descriptions) -> scope packs (project, task)
  -> conversation memory -> retrieval hits -> tool results of the current run.
- **Conversation memory:** keep the last N turns verbatim; summarise older turns into a rolling summary item (written by the model, stored, re-used); never
  re-send full tool results from earlier turns - keep the compact projection or a reference.
- **Tool results are compact by construction** (the M2 MCP server and packs show the pattern): projected fields only, long text truncated with an explicit
  marker, lists capped with `next_cursor`, `detail=true` to ask for more. The runtime enforces a hard cap per tool result and tells the model how to narrow.
- **Prompt caching:** keep the stable prefix (rules + tools) byte-identical across runs so provider caches apply; put volatile data last.
- **Retrieval:** chunk documents from the feed (title + body, small overlapping chunks), store embeddings with `project_id`, `tenant_id`, `content_hash`;
  update from the feed cursor, delete on tombstones, **filter by `readable-projects` at query time** (the index is shared, permissions are per user and change often).
- **Cost and abuse control:** per-user and per-tenant quotas, max tool calls and max wall time per run, cancellation, and usage records (`ModelUsage`).

### 3.4 Safety checklist for M9 design (acceptance tests)

Unauthorized tool call blocked by the module API (not only by M9); prompt injection in a task comment cannot cause a tool call the user did not intend
(confirmation gate for write/destructive; instructions in data ignored); retrieval never returns another user's projects; duplicate tool call is safe
(idempotency key); sensitive fields redacted before they reach third-party models (data classes per profile); cancellation and deadlines work;
tenant data never crosses conversations; usage quota enforced.

## 4. What exists today versus what M9 must add

| Exists (M2, 2026-10-29) | M9 must add |
|---|---|
| REST API with per-user RBAC, idempotency, optimistic versions | conversation/run/tool-call/policy/usage tables and APIs |
| `X-JAAS-Agent` attribution on activity and events | choose and set the agent label per assistant/feature |
| Tool catalogue, context packs, document feed, readable scope (`/ai-hooks/*`) | the same four hooks for M1 HR (sensitive: per-field redaction rules) and later modules |
| CLI (`jaas-pm`) and MCP server (`jaas-pm-mcp`, stdio) built on `pkg/pmclient` | an HTTP MCP/tool gateway if remote clients are needed; token strategy (see 5) |
| Compact projections and result caps (`pkg/pmclient/compact.go`) | the provider-agnostic `ModelClient`, context manager, retrieval index, confirmation UI |

## 5. Open decisions for M9 (record in `decisions.md` when it enters design)

1. **Long-lived credentials for agents.** Today agents sign in with the user's e-mail/password and use the 15-minute JWT with refresh. Plane uses per-user API tokens
   (`X-Api-Key`) with a label, expiry, last-used time and per-key rate limit. JAAS needs the equivalent in **M0** (scoped, revocable personal access tokens that resolve to the
   same user permissions, optionally narrower). This is a permission-model change and must be approved before M9 ships remote agents.
2. Pin the "Open Responses" specification version and the capability matrix of supported providers.
3. Embedding model and vector store (PostgreSQL pgvector vs. a separate store) and the per-tenant data-residency rules.
4. Redaction and data-class policy per module (HR payroll and personal data must not leave the tenant boundary to a third-party model by default).
5. Where confirmation UI lives (product UI) and how proposals are stored.

## 6. Reference: how Plane separates AI (reviewed 2026-10-29, `makeplane/plane`, read-only)

- The core application contains only a **thin stateless AI proxy** (`GPTIntegrationEndpoint` for a project, `WorkspaceGPTIntegrationEndpoint` for a workspace):
  it forwards a prompt to a provider chosen in instance settings and returns text. It is guarded by the same role decorator as every other endpoint
  (`allow_permission([ADMIN, MEMBER])`). No tools, no conversation state, no context management.
- Provider support is a small hard-coded catalogue (OpenAI, Anthropic, Gemini) with model lists - the rigidity JAAS avoids with configurable profiles.
- Programmatic and agent access goes through **per-user API tokens** (`plane_api_...`, `X-Api-Key`) bound to a user and a workspace, with expiry, last-used tracking,
  per-key rate limiting (with remaining/reset headers) and an API activity log. Their MCP server and AI assistant are separate services that call this API; RBAC is
  enforced by the API for every call.
- Outbound **webhooks** deliver events to external systems.

The shape is the same as the one chosen here (separate AI service, same RBAC for humans and agents), with three deliberate differences: a provider-agnostic model
client, first-class **tool catalogue / context packs / document feed** hooks in the modules, and **agent attribution** in activity and events.
