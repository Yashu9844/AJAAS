-- Automation rules (specification section 9): when a task event happens and all conditions hold, run the actions as the
-- rule creator. Runs are unique per (rule, event) so a redelivered event is applied once. The cursor table lets the engine
-- read new outbox rows independently of the broker relay.
CREATE TABLE workflow_rules (
    id UUID PRIMARY KEY,
    tenant_id UUID NOT NULL,
    project_id UUID NOT NULL REFERENCES projects (id) ON DELETE CASCADE,
    name VARCHAR(80) NOT NULL,
    trigger VARCHAR(30) NOT NULL CHECK (trigger IN ('task.created', 'task.moved', 'task.assigned', 'task.completed', 'task.commented', 'task.due_changed', 'task.pr_merged')),
    conditions JSONB NOT NULL DEFAULT '[]',
    actions JSONB NOT NULL DEFAULT '[]',
    active BOOLEAN NOT NULL DEFAULT TRUE,
    run_count INT NOT NULL DEFAULT 0,
    last_run_at TIMESTAMPTZ,
    consecutive_failures INT NOT NULL DEFAULT 0,
    flagged_code VARCHAR(40),
    created_by UUID NOT NULL,
    version INT NOT NULL DEFAULT 1,
    created_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP
);
CREATE UNIQUE INDEX uq_workflow_rules_name ON workflow_rules (project_id, lower(name));
CREATE INDEX idx_workflow_rules_trigger ON workflow_rules (tenant_id, project_id, trigger) WHERE active;

CREATE TABLE workflow_rule_runs (
    id UUID PRIMARY KEY,
    tenant_id UUID NOT NULL,
    rule_id UUID NOT NULL REFERENCES workflow_rules (id) ON DELETE CASCADE,
    event_id UUID NOT NULL,
    task_id UUID,
    status VARCHAR(10) NOT NULL CHECK (status IN ('ok', 'skipped', 'failed')),
    error_code VARCHAR(40),
    action_count INT NOT NULL DEFAULT 0,
    created_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP
);
CREATE UNIQUE INDEX uq_workflow_rule_runs_event ON workflow_rule_runs (rule_id, event_id);
CREATE INDEX idx_workflow_rule_runs_rule ON workflow_rule_runs (rule_id, created_at DESC, id DESC);
CREATE INDEX idx_workflow_rule_runs_age ON workflow_rule_runs (created_at);

-- One row per background reader of the outbox; (last_created_at, last_id) is the position, lease_until the single-reader lease.
CREATE TABLE automation_cursor (
    name VARCHAR(40) PRIMARY KEY,
    last_created_at TIMESTAMPTZ NOT NULL,
    last_id UUID NOT NULL,
    lease_until TIMESTAMPTZ,
    updated_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP
);
INSERT INTO automation_cursor (name, last_created_at, last_id) VALUES ('rules', CURRENT_TIMESTAMP, '00000000-0000-0000-0000-000000000000');
