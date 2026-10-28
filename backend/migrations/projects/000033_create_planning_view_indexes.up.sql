-- Hot paths of the read models (design section 3): backlog and sprint paging, gantt windows, my-work joins.
CREATE INDEX idx_plan_tasks_backlog ON tasks (tenant_id, project_id, rank, id)
    WHERE sprint_id IS NULL AND deleted_at IS NULL AND archived_at IS NULL;
CREATE INDEX idx_plan_tasks_sprint ON tasks (tenant_id, sprint_id, rank, id)
    WHERE sprint_id IS NOT NULL AND deleted_at IS NULL;
CREATE INDEX idx_plan_tasks_gantt ON tasks (tenant_id, project_id, (COALESCE(start_on, (due_at AT TIME ZONE 'UTC')::date)), id)
    WHERE deleted_at IS NULL AND archived_at IS NULL AND (start_on IS NOT NULL OR due_at IS NOT NULL);
CREATE INDEX idx_plan_task_assignees_user ON task_assignees (tenant_id, user_id, task_id);
