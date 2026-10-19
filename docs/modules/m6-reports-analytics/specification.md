# M6 Reports & Analytics — Specification (PROPOSED)

> Not frozen. This is the target-state contract from `docs/ARCHITECTURE.md` section 5, combined with the logical data
> model from `model/db-model.md`. API paths are proposed shapes. Universal rules (tenancy, idempotency, outbox/inbox,
> envelopes, security, latency) in `docs/ARCHITECTURE.md` section 3 apply in full.

**Purpose:** provide dashboards, scheduled reports, operational metrics,
and cross-module insights without becoming the source of transactional
truth.

**Data model:** ReportDefinition, ReportVersion, Schedule, Execution,
ResultArtifactReference, DataSourceGrant, ReadModelVersion,
RefreshCheckpoint.

**Representative APIs:**
- `GET/POST /api/v1/reports/definitions`
- `POST /api/v1/reports/definitions/{id}/run`
- `POST /api/v1/reports/definitions/{id}/schedule`
- `GET /api/v1/reports/executions/{id}`
- `GET /api/v1/analytics/dashboard`
- `GET /api/v1/analytics/metrics/{metric}`

**Rules:** explicit allowed data sources and field-level permissions;
tenant-scoped read models; report runs are async for expensive queries;
signed, short-lived artifact links; query budgets and export row limits;
distinguish freshness from correctness. Use event-fed projections or
scheduled extracts instead of live synchronous fan-out for every chart.

**Events:** report requested/completed/failed; projection refreshed;
schedule due. Notifications sends completion notices.

**Acceptance tests:** tenant-isolated aggregates; stale projection
indicator; export permission check; report job retry idempotency;
expensive query cancellation; dashboard p95 budget under realistic
fan-out.

## Proposed entities

| Entity/table | Key fields (illustrative) |
|---|---|
| `ReportDefinition` | report_id, tenant_id, owner_id, name, definition_version, status |
| `ReportVersion` | report_id, version, query_spec_ref, created_at |
| `ReportSchedule` | schedule_id, report_id, cron, timezone, enabled |
| `ReportExecution` | execution_id, report_id, idempotency_key, status, started_at, finished_at |
| `ResultArtifactReference` | execution_id, object_key_ref, expires_at, checksum |
| `ProjectionCheckpoint` | projection_name, partition_key, last_event_id, updated_at |
| `ReadModelVersion` | model_name, tenant_id, source_watermark, refreshed_at |
| `DataSourceGrant` | tenant_id, role_id, dataset, allowed_fields |
| `ProjectionRecord` | tenant_id, projection_key, payload, source_version, refreshed_at |

## Relationships and invariants

ReportDefinition 1:N ReportVersion, ReportSchedule and ReportExecution. ReportExecution 0:1 ResultArtifactReference. ProjectionCheckpoint tracks consumer progress. ProjectionRecord is rebuildable and tenant-scoped; access control is rechecked at query/export time.

## Service dependencies

Consumes versioned events or bounded APIs; uses M0 authorization context; object storage holds exports. Must not write into source-domain databases or expose stale projections as live truth.
