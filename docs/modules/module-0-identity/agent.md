# Module 0 — Agent Law

ROLE: Autonomous implementer for Module 0 (Tenant + Identity + RBAC). You own delivery of current-goal.md inside this module only. Module 0 is the root of the JAAS dependency graph — zero internal deps, consumed by Modules 1–12.

READ ORDER (mandatory): 1.README.md (this folder) 2.current-goal.md 3.plan.md 4.current-status.md 5.implementation-checklist.md (task list) 6.api-contracts.md 7.business-rules.md (rule IDs TN/AU/AZ/RB/PW/SS/RT/MF/SD/AD) 8.database-schema.dbml 9.events.md 10.security.md (+ docs/SECURITY.md) 11.coding-guidelines.md 12.testing-strategy.md 13.files map below (no files.md in Module 0 — use docs/modules/module-0-identity file list + backend tree).

ALLOWED: docs/modules/module-0-identity/**, backend/internal/identity/**, backend/internal/shared/** (cache/queue/middleware/utils need decisions.md entry + dependent tests), backend/{cmd,migrations,api,tests}/**, frontend/src/modules/identity/**, docs/{SECURITY,PEDING_WORK,CURRENT_STATUS,SERVICE_LOOP}.md, loop.md, VISION.md.

FORBIDDEN: editing Modules 1–12 implementation; editing shared/ contracts (config/database/errors/logger/constants interfaces) without decisions.md + full test run; any prod secret; editing golden tests to make red pass (§15 procedure only); claiming an unrun test passed.

CODING (docs/SECURITY.md §13 + coding-guidelines.md): self-defining names; one-line `// Name does X.` on exports; WHY-comments with rule IDs (e.g. `// PW-008: same response prevents enumeration`); func ≤50 lines, file ≤300, depth ≤4, params ≤4; no panic(), no TODO, no eslint-disable/nolint without approval; AppError flow (repository raw → service wraps → controller maps to HTTP); DTOs in dto/ only, Request ≠ Response, snake_case JSON + validate tags, PATCH pointers; models singular snake_case with GORM tags matching database-schema.dbml exactly; routes lowercase-hyphen, plural, no trailing slash.

ARCHITECTURE: controller thin (parse → validate → service → map → status, never business logic, never *gorm.DB); service owns logic (takes DTOs + context.Context, NEVER *gin.Context, never HTTP codes, one use case per method, tx for multi-table writes, publish events AFTER commit, no I/O inside tx, audit every state change); repository models-only (tenant_id filter on every scoped query, *gorm.DB tx injection, Preload capped, not-found returns nil,nil); constructor DI via module.go, interfaces not concretes, no cycles, no globals, no init() wiring.

DEPENDENCY RULES: consume only documented contracts; JWT tid must equal subdomain tenant; IDs resolved within JWT tenant (cross-tenant body IDs ignored → 404); prefer local adapter over touching shared/.

SECURITY: deny by default — only POST /auth/login, /auth/forgot-password, /auth/reset-password are public. Generic `invalid credentials` for bad tenant/email/password/soft-deleted. Forgot always 200. bcrypt 12, SHA-256 token hashes, AES-256-GCM MFA secrets. Rotation + reuse-revokes-all. TTLs: JWT 15m, refresh 7d, reset 1h, session 24h, perms cache 5m, MFA lock 5 fails/15m. Redis sliding-window limits (login 10/15m per email + 100/15m per IP, forgot 5/h, reset 10/h per IP, refresh 30/15m). Audit immutable, no passwords. Parameterized queries only. Secure headers + CORS whitelist on every response.

TESTING: RED first. Unit (services ≥90%, mocks), integration (Docker PG: CRUD, uniques incl. soft-deleted, pagination, tx rollback), contract (24 endpoints, envelopes, status map), isolation (A↔B tenants, JWT cross-subdomain → 403), security (enumeration-identical, tamper/expired → 401, reuse → revoke-all, SQLi, 11th login → 429), E2E login → access → refresh → logout. Commands in testing-strategy.md + docs/SECURITY.md §11.

VALIDATION: sensors green or fix (max 3 same-error attempts → BLOCKED + exit). Critic re-read vs docs/SECURITY.md §14 before done.

DOCUMENTATION: update current-status.md, todo (implementation-checklist.md), plan.md map, handoff.md, changelog.md in the same task. No doc may claim unimplemented behavior.

HANDOFF: phase transition (e.g. G0-1 → DONE, G0-2 → CURRENT), files changed, tests run with counts, sensors, cross-module impact, remaining risks.
