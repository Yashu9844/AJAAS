CREATE TABLE tasks (
    id UUID PRIMARY KEY,
    tenant_id UUID NOT NULL,
    project_id UUID NOT NULL REFERENCES projects (id) ON DELETE CASCADE,
    number INT NOT NULL,
    parent_task_id UUID REFERENCES tasks (id),
    depth SMALLINT NOT NULL DEFAULT 0 CHECK (depth BETWEEN 0 AND 3),
    title VARCHAR(300) NOT NULL,
    description TEXT NOT NULL DEFAULT '',
    column_id UUID NOT NULL REFERENCES board_columns (id),
    priority VARCHAR(8) NOT NULL DEFAULT 'none' CHECK (priority IN ('none', 'low', 'medium', 'high', 'urgent')),
    reporter_user_id UUID NOT NULL,
    start_on DATE,
    due_at TIMESTAMPTZ,
    estimate_minutes INT CHECK (estimate_minutes IS NULL OR estimate_minutes >= 0),
    milestone_id UUID REFERENCES milestones (id),
    sprint_id UUID,
    rank TEXT COLLATE "C" NOT NULL,
    completed_at TIMESTAMPTZ,
    archived_at TIMESTAMPTZ,
    deleted_at TIMESTAMPTZ,
    version INT NOT NULL DEFAULT 1,
    search TSVECTOR,
    created_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP,
    CONSTRAINT tasks_dates CHECK (start_on IS NULL OR due_at IS NULL OR due_at::date >= start_on)
);
CREATE UNIQUE INDEX uq_tasks_project_number ON tasks (project_id, number);
CREATE INDEX idx_tasks_board ON tasks (tenant_id, project_id, column_id, rank, id) WHERE deleted_at IS NULL AND archived_at IS NULL;
CREATE INDEX idx_tasks_project_updated ON tasks (tenant_id, project_id, updated_at DESC) WHERE deleted_at IS NULL;
CREATE INDEX idx_tasks_project_due ON tasks (tenant_id, project_id, due_at) WHERE deleted_at IS NULL AND due_at IS NOT NULL;
CREATE INDEX idx_tasks_parent ON tasks (parent_task_id) WHERE parent_task_id IS NOT NULL;
CREATE INDEX idx_tasks_milestone ON tasks (milestone_id) WHERE milestone_id IS NOT NULL;
CREATE INDEX idx_tasks_sprint ON tasks (sprint_id) WHERE sprint_id IS NOT NULL;
CREATE INDEX idx_tasks_search ON tasks USING GIN (search);

CREATE FUNCTION projects_tasks_search_update() RETURNS trigger AS $$
BEGIN
    NEW.search := setweight(to_tsvector('simple', coalesce(NEW.title, '')), 'A') || setweight(to_tsvector('simple', coalesce(NEW.description, '')), 'B');
    RETURN NEW;
END
$$ LANGUAGE plpgsql;
CREATE TRIGGER tasks_search_update BEFORE INSERT OR UPDATE OF title, description ON tasks
    FOR EACH ROW EXECUTE FUNCTION projects_tasks_search_update();
