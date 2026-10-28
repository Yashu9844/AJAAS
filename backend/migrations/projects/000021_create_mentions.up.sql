-- Mentions that survived the readability filter (CM-02, PG11): only users who can read the project are stored.
CREATE TABLE mentions (
    tenant_id UUID NOT NULL,
    source_kind VARCHAR(10) NOT NULL CHECK (source_kind IN ('task', 'comment')),
    source_id UUID NOT NULL,
    task_id UUID NOT NULL REFERENCES tasks (id) ON DELETE CASCADE,
    user_id UUID NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP,
    PRIMARY KEY (source_kind, source_id, user_id)
);
CREATE INDEX idx_mentions_user ON mentions (tenant_id, user_id, created_at DESC);
CREATE INDEX idx_mentions_task ON mentions (tenant_id, task_id);
