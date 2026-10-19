# VISION.md — JAAS North Star

## What we are building

JAAS — a multi-tenant Company Operating System. One platform to run a company: HR, projects, meetings, assets,
finance, reporting, approvals, documents, AI assistance, notifications, company knowledge and settings — replacing a
stack of tools such as Workday/GreytHR, Jira, Monday, ClickUp, Zoho One, Odoo and ERPNext.

Model: one tenant per customer (`acme.jaas.com`) with isolated users, roles, data and settings. The JAAS Super Admin
owns the platform; each Tenant Admin owns their tenant.

## Shape

M0 Identity & Access plus 12 product modules (M1 HR Management … M12 Settings), each an independently deployable
service with its own data, communicating through versioned APIs and events. Dashboard is a role-aware frontend
composition. Architecture authority: `docs/ARCHITECTURE.md`.

## Where we are (2026-10-20)

M0 is built and hardened. M1 HR Management has six contexts built and `performance` in progress; it is being
restructured into one HR service and completed to enterprise-HCM depth before any other module starts
(`docs/modules/m1-hr-management/plan.md`).

## Non-negotiables

1. `docs/SECURITY.md` is law: secure by default, tenant isolation as a hard boundary, least privilege, sensitive data
   masked by default, immutable audit.
2. Production quality per module: a smaller feature set done completely beats a large half-built one.
3. Exactness: no floating point in money or quantities; history is never silently overwritten.
4. Reliability: transactional outbox, idempotent commands, retries without duplicate side effects.
5. Latency budgets: p95 < 200 ms single GET, < 500 ms list, < 300 ms login, < 50 ms query.
6. Every feature is verifiable by hand in the test console before it is called done.
7. Readable, modular code with concise comments; docs never claim what the code does not do.
