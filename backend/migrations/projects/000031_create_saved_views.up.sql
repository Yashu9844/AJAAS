-- Saved views (VW-06): personal, or shared with the readers of one project. Filters and sort are validated against
-- fixed allow-lists by the service before they are stored; they are never executed as SQL.
CREATE TABLE saved_views (
    id UUID PRIMARY KEY,
    tenant_id UUID NOT NULL,
    project_id UUID REFERENCES projects (id) ON DELETE CASCADE,
    owner_user_id UUID NOT NULL,
    name VARCHAR(80) NOT NULL,
    layout VARCHAR(12) NOT NULL CHECK (layout IN ('board', 'list', 'calendar', 'gantt')),
    filters JSONB NOT NULL DEFAULT '{}',
    sort JSONB NOT NULL DEFAULT '[]',
    grouping VARCHAR(24) NOT NULL DEFAULT '',
    shared BOOLEAN NOT NULL DEFAULT FALSE,
    version INT NOT NULL DEFAULT 1,
    created_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP,
    CONSTRAINT saved_views_shared_needs_project CHECK (NOT shared OR project_id IS NOT NULL)
);
CREATE UNIQUE INDEX uq_saved_views_name ON saved_views
    (tenant_id, owner_user_id, COALESCE(project_id, '00000000-0000-0000-0000-000000000000'::uuid), lower(name));
CREATE INDEX idx_saved_views_owner ON saved_views (tenant_id, owner_user_id, created_at DESC, id DESC);
CREATE INDEX idx_saved_views_shared ON saved_views (tenant_id, project_id) WHERE shared;
