-- Sprints (SP-01): at most one active sprint per project is enforced by the partial unique index (PG6).
CREATE TABLE sprints (
    id UUID PRIMARY KEY,
    tenant_id UUID NOT NULL,
    project_id UUID NOT NULL REFERENCES projects (id) ON DELETE CASCADE,
    name VARCHAR(120) NOT NULL,
    goal VARCHAR(1000) NOT NULL DEFAULT '',
    start_on DATE,
    end_on DATE,
    status VARCHAR(16) NOT NULL DEFAULT 'planned' CHECK (status IN ('planned', 'active', 'completed')),
    started_at TIMESTAMPTZ,
    completed_at TIMESTAMPTZ,
    report JSONB,
    version INT NOT NULL DEFAULT 1,
    created_by UUID NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP,
    CONSTRAINT sprints_dates CHECK (start_on IS NULL OR end_on IS NULL OR end_on >= start_on)
);
CREATE UNIQUE INDEX uq_sprints_one_active ON sprints (project_id) WHERE status = 'active';
CREATE UNIQUE INDEX uq_sprints_name ON sprints (project_id, lower(name));
CREATE INDEX idx_sprints_project ON sprints (tenant_id, project_id, created_at DESC, id DESC);

-- tasks.sprint_id was created without a target; no sprint existed before this migration, so any value is dangling.
UPDATE tasks SET sprint_id = NULL WHERE sprint_id IS NOT NULL;
ALTER TABLE tasks ADD CONSTRAINT fk_tasks_sprint FOREIGN KEY (sprint_id) REFERENCES sprints (id) ON DELETE SET NULL;
