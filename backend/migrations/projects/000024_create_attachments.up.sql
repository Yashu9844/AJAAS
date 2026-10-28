-- File metadata (AT-01..AT-05); the bytes live in object storage under storage_key. task_id has no foreign key on purpose:
-- the retention purge deletes tasks, and the attachment sweep then removes the orphaned objects before the rows.
CREATE TABLE attachments (
    id UUID PRIMARY KEY,
    tenant_id UUID NOT NULL,
    project_id UUID NOT NULL,
    task_id UUID NOT NULL,
    storage_key TEXT NOT NULL,
    filename VARCHAR(255) NOT NULL,
    content_type VARCHAR(127) NOT NULL,
    size BIGINT NOT NULL CHECK (size > 0),
    sha256 VARCHAR(64) NOT NULL DEFAULT '',
    status VARCHAR(8) NOT NULL DEFAULT 'pending' CHECK (status IN ('pending', 'ready', 'deleted')),
    uploaded_by UUID NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP,
    ready_at TIMESTAMPTZ,
    deleted_at TIMESTAMPTZ
);
CREATE INDEX idx_attachments_task ON attachments (tenant_id, task_id, created_at, id) WHERE status = 'ready';
CREATE INDEX idx_attachments_quota ON attachments (tenant_id) WHERE status <> 'deleted';
CREATE INDEX idx_attachments_sweep ON attachments (status, created_at) WHERE status <> 'ready';
