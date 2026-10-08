# JAAS Dependency Graph

Build bottom-up. Independent same-level modules may run in parallel. Details per module: `docs/modules/<slug>/connections.md`.

| Module | Depends On | Consumed By | Type | State (2026-09-30) |
|---|---|---|---|---|
| 0 Identity | PG/Redis/RabbitMQ/SMTP/Kong (infra) | 1–12 (all) | Infra + Auth/API root | Spec complete; shared/ done; models 1/12 broken; G0-1 CURRENT |
| 1 Organization | 0 (Tenant, User, RBAC) | 2, 3, 4, 5 | API/Data/Auth | Design pending (ER review next) |
| 2 Employee | 0, 1 | 3, 4, 5, 6 | API/Data | Pending |
| 3 Attendance, Shifts & Time Tracking (masterplan scope, D3-01) | 0, 2 | 4 (Leave), 5, 10, 11, 12 | API/Data/Event | backend DONE 2026-10-09 (P1–P7, P9; live goldens PASS); frontend P8 LATER (needs identity shell) |
| 4 Projects | 2, 1 | 6, 7, 10, 11 | API/Data | Pending |
| 5 Meetings | 2, 1 | 6, 7, 10, 11 | API/Data | Pending |
| 6 Approvals | 0, 2 | 7, 10, 11 | API/Data | Pending |
| 7 Notifications | 0, 4, 5, 6, 8, 9 (events) | 10, 11 | Event/Data | Pending |
| 8 Assets | 0 (User, RBAC) | 7, 10, 11 | API/Auth | Pending |
| 9 Payroll | 0 (User, RBAC) | 7, 10, 11 | API/Auth | Pending |
| 10 Analytics | all (read contracts + events) | 11 | Data/Event | Pending |
| 11 AI | all business | — | Data | Pending |
| 12 IoT | 0 (User, RBAC) | 7, 10, 11 | API/Auth | Pending |

Edge-type legend: Runtime / API / Data / Configuration / Authentication / Infrastructure / Testing — labeled per edge in each module's connections.md.

Blast-radius reads:
- Changing Module 0 contracts → affects 1–12. Highest caution; backward-compatible only.
- Changing Modules 1–2 → affects 3–7, 10–11.
- Modules 3–6, 8–9, 12 → affect 7, 10–11 only.
- Modules 10–11 → affect nothing downstream (tops of graph).

"If I change X, what breaks?" → Consumed By column + provider `connections.md` failure rows + consumer golden tests.
"What may I depend on?" → provider `specification.md` + `connections.md` + `golden-tests.md` only. Never internals.
