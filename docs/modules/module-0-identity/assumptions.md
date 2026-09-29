# Module 0 — Assumptions (operational additions to spec assumptions AS-001..AS-008)

- Loop entry assumes agent reads agent.md → current-goal.md → plan.md → current-status.md in order; skipping plan.md causes phase mistakes (e.g. starting migrations before models freeze).
- G0-1 assumes database-schema.dbml is frozen for v1; any DBML change after P1 DONE requires decisions.md + migration revision, not silent tag edits.
- G0-3 assumes Docker is available for PG/Redis/RabbitMQ; without it, boot validation substitutes to build+unit only and reports the gap in handoff (never claims E2E passed).
- Frontend G0-13 assumes backend P10+P11 DONE with live contracts; no frontend work against mocked seams.
