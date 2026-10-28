CREATE TABLE labels (
    id UUID PRIMARY KEY,
    tenant_id UUID NOT NULL,
    project_id UUID REFERENCES projects (id) ON DELETE CASCADE,
    name VARCHAR(60) NOT NULL,
    color VARCHAR(16) NOT NULL DEFAULT '',
    created_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP
);
-- Names are unique per scope (one project, or the tenant when project_id is NULL), ignoring case.
CREATE UNIQUE INDEX uq_labels_project_name ON labels (project_id, lower(name)) WHERE project_id IS NOT NULL;
CREATE UNIQUE INDEX uq_labels_tenant_name ON labels (tenant_id, lower(name)) WHERE project_id IS NULL;

CREATE TABLE milestones (
    id UUID PRIMARY KEY,
    tenant_id UUID NOT NULL,
    project_id UUID NOT NULL REFERENCES projects (id) ON DELETE CASCADE,
    name VARCHAR(120) NOT NULL,
    due_on DATE,
    status VARCHAR(16) NOT NULL DEFAULT 'open' CHECK (status IN ('open', 'reached')),
    created_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP
);
CREATE UNIQUE INDEX uq_milestones_name ON milestones (project_id, lower(name));

CREATE TABLE project_favorites (
    project_id UUID NOT NULL REFERENCES projects (id) ON DELETE CASCADE,
    user_id UUID NOT NULL,
    tenant_id UUID NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP,
    PRIMARY KEY (project_id, user_id)
);
CREATE INDEX idx_project_favorites_user ON project_favorites (tenant_id, user_id);
