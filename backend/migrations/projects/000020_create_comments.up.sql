-- Task comments (CM-01). A delete leaves a tombstone: the row stays, the body is cleared and deleted_at is set.
CREATE TABLE comments (
    id UUID PRIMARY KEY,
    tenant_id UUID NOT NULL,
    task_id UUID NOT NULL REFERENCES tasks (id) ON DELETE CASCADE,
    author_user_id UUID NOT NULL,
    body TEXT NOT NULL DEFAULT '' CHECK (char_length(body) <= 10000),
    edited_at TIMESTAMPTZ,
    deleted_at TIMESTAMPTZ,
    deleted_by UUID,
    created_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP
);
CREATE INDEX idx_comments_task ON comments (tenant_id, task_id, created_at, id);
