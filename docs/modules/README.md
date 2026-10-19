# JAAS Modules

M0 Identity & Access plus the 12 product modules (`docs/ARCHITECTURE.md` §2.1). Dashboard is frontend composition,
not a module. Build order follows `docs/ARCHITECTURE.md` §8 and §16: M0 → M1 HR (complete) → M7 Approvals and
M8 Documents → M2–M6 and M10, M12 → M6 Reports, M11 Knowledge Base, M9 AI Assistant.

| Module | Folder | Status (2026-10-20) |
|---|---|---|
| M0 Identity & Access | `m0-identity-access/` | built and hardened; Phase B alignment next |
| M1 HR Management | `m1-hr-management/` | 6 contexts built, `performance` in progress, 5 contexts planned |
| M2 Projects & Tasks | `m2-projects-tasks/` | not started |
| M3 Meetings | `m3-meetings/` | not started |
| M4 Assets & Inventory | `m4-assets-inventory/` | not started |
| M5 Finance | `m5-finance/` | not started |
| M6 Reports & Analytics | `m6-reports-analytics/` | not started |
| M7 Approvals | `m7-approvals/` | not started |
| M8 Documents | `m8-documents/` | not started |
| M9 AI Assistant | `m9-ai-assistant/` | not started |
| M10 Notifications | `m10-notifications/` | not started |
| M11 Company Knowledge Base | `m11-knowledge-base/` | not started |
| M12 Settings | `m12-settings/` | not started |

Bundle layout rules: `docs/ENGINEERING.md` §8. Universal dependency matrix:
`docs/architecture/universal_db_model_and_dependencies.md`.
