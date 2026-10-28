-- Old ticket ids keep resolving after a task moves to another project (TK-08, DPJ-10).
CREATE TABLE task_ticket_alias (
    tenant_id UUID NOT NULL,
    old_key VARCHAR(10) NOT NULL,
    old_number INT NOT NULL,
    task_id UUID NOT NULL REFERENCES tasks (id) ON DELETE CASCADE,
    created_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP,
    PRIMARY KEY (tenant_id, old_key, old_number)
);
CREATE INDEX idx_task_ticket_alias_task ON task_ticket_alias (task_id);
