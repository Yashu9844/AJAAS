-- Append-only history per task and project (CM-04). Rows are never updated; they disappear only with their project.
CREATE TABLE activity (
    seq BIGSERIAL PRIMARY KEY,
    id UUID NOT NULL UNIQUE,
    tenant_id UUID NOT NULL,
    project_id UUID NOT NULL REFERENCES projects (id) ON DELETE CASCADE,
    task_id UUID REFERENCES tasks (id) ON DELETE CASCADE,
    actor_user_id UUID,
    type VARCHAR(60) NOT NULL,
    data JSONB NOT NULL DEFAULT '{}',
    source VARCHAR(20) NOT NULL DEFAULT 'user',
    occurred_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP
);
CREATE INDEX idx_activity_task ON activity (tenant_id, task_id, seq) WHERE task_id IS NOT NULL;
CREATE INDEX idx_activity_project ON activity (tenant_id, project_id, seq);

CREATE FUNCTION projects_activity_immutable() RETURNS trigger AS $$
BEGIN
    RAISE EXCEPTION 'activity rows are append-only';
END
$$ LANGUAGE plpgsql;
CREATE TRIGGER activity_no_update BEFORE UPDATE ON activity FOR EACH ROW EXECUTE FUNCTION projects_activity_immutable();
