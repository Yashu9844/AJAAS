# JAAS — Company Operating System

Multi-tenant SaaS: M0 Identity & Access plus 12 product modules (HR Management, Projects & Tasks, Meetings, Assets &
Inventory, Finance, Reports & Analytics, Approvals, Documents, AI Assistant, Notifications, Company Knowledge Base,
Settings).

- Backend: Go, Gin, GORM/pgx, PostgreSQL, Redis, RabbitMQ (`backend/`)
- Frontend: Next.js, React, TypeScript (`frontend/`); verification console at `/dev-test`

## Start here

| Read | For |
|---|---|
| `VISION.md` | what and why |
| `docs/ARCHITECTURE.md` | the architecture authority (services, rules, repo layout, migration status) |
| `docs/modules/README.md` | module index and status |
| `docs/ENGINEERING.md` | setup, workflow, gates, Definition of Done |
| `docs/SECURITY.md` | security law and code style |
| `docs/TEST_CONSOLE.md` | test every feature by hand |
| `docs/SYSTEM_REFERENCE.md` | generated inventory of routes, tables, files, functions and tests |
| `AGENTS.md`, `loop.md` | autonomous agent workflow |
