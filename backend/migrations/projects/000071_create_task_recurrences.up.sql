-- Recurring tasks: a scheduler copies the template task on schedule as the recurrence creator.
CREATE TABLE task_recurrences (
    id UUID PRIMARY KEY,
    tenant_id UUID NOT NULL,
    project_id UUID NOT NULL REFERENCES projects (id) ON DELETE CASCADE,
    template_task_id UUID NOT NULL REFERENCES tasks (id) ON DELETE CASCADE,
    frequency VARCHAR(8) NOT NULL CHECK (frequency IN ('daily', 'weekly', 'monthly')),
    interval INT NOT NULL DEFAULT 1 CHECK (interval BETWEEN 1 AND 52),
    weekday INT CHECK (weekday IS NULL OR weekday BETWEEN 0 AND 6),
    day_of_month INT CHECK (day_of_month IS NULL OR day_of_month BETWEEN 1 AND 31),
    next_run_at TIMESTAMPTZ NOT NULL,
    until_on DATE,
    active BOOLEAN NOT NULL DEFAULT TRUE,
    flagged_code VARCHAR(40),
    last_run_at TIMESTAMPTZ,
    last_task_id UUID,
    created_by UUID NOT NULL,
    version INT NOT NULL DEFAULT 1,
    created_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP
);
CREATE INDEX idx_task_recurrences_project ON task_recurrences (tenant_id, project_id, created_at);
CREATE INDEX idx_task_recurrences_due ON task_recurrences (next_run_at) WHERE active;
