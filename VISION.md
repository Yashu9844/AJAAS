# VISION.md — JAAS North Star

Read by every agent loop tick. Anchors intent so ticks never re-derive direction.

## What we are building

JAAS (Just Another Admin SaaS / Company Operating System). One multi-tenant platform
replacing Jira, Monday, ClickUp, GreytHR, Zoho One, Odoo, ERPNext for running a company:
HR, Assets, Projects, Meetings, Payroll, Reporting, AI, IoT.

Model: one tenant per customer (`acme.jaas.com`). Isolated users, roles, data.
JAAS Super Admin owns the platform. Tenant Admin owns their tenant.

## Where we are

Module 0 (Tenant + Identity + RBAC) is the current build. Design is complete on paper
(`docs/modules/module-0-identity/`). Implementation is at shared-foundation only.
Everything in `docs/PENDING_WORK.md` is the backlog. Build it in the order given there.

## What "done" means (Module 0)

- SC-001: all 12 tables migrated. SC-002: all 24 endpoints correct.
- SC-003: login → access → refresh → logout works end-to-end.
- SC-004: no cross-tenant access. SC-005: RBAC blocks unauthorized.
- SC-006: all domain events published. SC-007: audit on every state change.
- SC-008: password reset end-to-end. SC-009: p95 latency budgets met.
- SC-010: coverage ≥80% overall, ≥90% services.

## Non-negotiables

1. `docs/SECURITY.md` is law. Secure by default, tenant isolation as a boundary,
   generic auth errors, rotation + reuse detection, immutable audit.
2. Readable, self-defining, modular service code. Concise proper comments only.
3. Latency budgets: p95 <200ms single GET, <500ms list, <300ms login, <50ms query.
4. One service fully finished (code → tests → fix → integrate) before the next.
5. Backend first, then frontend against the live API, then E2E. Never mock the seam.
