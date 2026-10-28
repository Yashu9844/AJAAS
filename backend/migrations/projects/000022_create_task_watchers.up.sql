-- Watchers receive assignment, status and comment events (CM-03); delivery belongs to M10.
CREATE TABLE task_watchers (
    task_id UUID NOT NULL REFERENCES tasks (id) ON DELETE CASCADE,
    user_id UUID NOT NULL,
    tenant_id UUID NOT NULL,
    reason VARCHAR(12) NOT NULL DEFAULT 'explicit' CHECK (reason IN ('explicit', 'reporter', 'assignee', 'commenter', 'mentioned')),
    created_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP,
    PRIMARY KEY (task_id, user_id)
);
CREATE INDEX idx_task_watchers_user ON task_watchers (tenant_id, user_id);
