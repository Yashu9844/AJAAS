CREATE TABLE projects (
    id UUID PRIMARY KEY,
    tenant_id UUID NOT NULL,
    key VARCHAR(10) NOT NULL,
    name VARCHAR(120) NOT NULL,
    description TEXT NOT NULL DEFAULT '',
    status VARCHAR(16) NOT NULL DEFAULT 'active' CHECK (status IN ('active', 'archived')),
    visibility VARCHAR(16) NOT NULL DEFAULT 'private' CHECK (visibility IN ('private', 'tenant')),
    mode VARCHAR(16) NOT NULL DEFAULT 'kanban' CHECK (mode IN ('kanban', 'scrum')),
    owner_user_id UUID NOT NULL,
    start_on DATE,
    due_on DATE,
    icon VARCHAR(40) NOT NULL DEFAULT '',
    color VARCHAR(16) NOT NULL DEFAULT '',
    task_counter INT NOT NULL DEFAULT 0,
    time_tracking_enabled BOOLEAN NOT NULL DEFAULT TRUE,
    version INT NOT NULL DEFAULT 1,
    created_by UUID NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP,
    last_activity_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP,
    archived_at TIMESTAMPTZ,
    deleted_at TIMESTAMPTZ,
    CONSTRAINT projects_dates CHECK (due_on IS NULL OR start_on IS NULL OR due_on >= start_on)
);
-- A key stays reserved for the tenant even after a soft delete, so ticket ids never point at a different project.
CREATE UNIQUE INDEX uq_projects_tenant_key ON projects (tenant_id, key);
CREATE INDEX idx_projects_tenant_activity ON projects (tenant_id, last_activity_at DESC, id) WHERE deleted_at IS NULL;
