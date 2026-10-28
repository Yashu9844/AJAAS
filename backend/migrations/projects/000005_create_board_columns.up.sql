CREATE TABLE board_columns (
    id UUID PRIMARY KEY,
    tenant_id UUID NOT NULL,
    project_id UUID NOT NULL REFERENCES projects (id) ON DELETE CASCADE,
    name VARCHAR(60) NOT NULL,
    key VARCHAR(40) NOT NULL,
    position INT NOT NULL,
    color VARCHAR(16) NOT NULL DEFAULT '',
    category VARCHAR(16) NOT NULL CHECK (category IN ('backlog', 'todo', 'in_progress', 'done', 'canceled')),
    wip_limit INT CHECK (wip_limit IS NULL OR wip_limit > 0),
    wip_hard BOOLEAN NOT NULL DEFAULT FALSE,
    is_default BOOLEAN NOT NULL DEFAULT FALSE,
    created_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP
);
CREATE UNIQUE INDEX uq_board_columns_key ON board_columns (project_id, key);
CREATE UNIQUE INDEX uq_board_columns_default ON board_columns (project_id) WHERE is_default;
CREATE INDEX idx_board_columns_order ON board_columns (project_id, position);
