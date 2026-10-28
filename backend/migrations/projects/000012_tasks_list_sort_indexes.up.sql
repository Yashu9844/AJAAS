-- Keyset-paged task lists (TK-13): a stored priority weight and one index per sort order, so a 50 000-task project pages
-- without sorting the whole project (PG7).
ALTER TABLE tasks ADD COLUMN priority_rank SMALLINT GENERATED ALWAYS AS (
    CASE priority WHEN 'urgent' THEN 4 WHEN 'high' THEN 3 WHEN 'medium' THEN 2 WHEN 'low' THEN 1 ELSE 0 END) STORED;
CREATE INDEX idx_tasks_list_rank ON tasks (tenant_id, project_id, rank, id) WHERE deleted_at IS NULL;
CREATE INDEX idx_tasks_list_created ON tasks (tenant_id, project_id, created_at, id) WHERE deleted_at IS NULL;
CREATE INDEX idx_tasks_list_updated ON tasks (tenant_id, project_id, updated_at, id) WHERE deleted_at IS NULL;
CREATE INDEX idx_tasks_list_due ON tasks (tenant_id, project_id, (COALESCE(due_at, 'infinity'::timestamptz)), id) WHERE deleted_at IS NULL;
CREATE INDEX idx_tasks_list_priority ON tasks (tenant_id, project_id, priority_rank, id) WHERE deleted_at IS NULL;
CREATE INDEX idx_tasks_list_number ON tasks (tenant_id, project_id, number) WHERE deleted_at IS NULL;
