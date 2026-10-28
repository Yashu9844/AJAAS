-- Time logged on tasks (TM-01..TM-04). A running timer has ended_at NULL; one per user is a database constraint (PG6).
CREATE TABLE time_entries (
    id UUID PRIMARY KEY,
    tenant_id UUID NOT NULL,
    task_id UUID NOT NULL REFERENCES tasks (id) ON DELETE CASCADE,
    user_id UUID NOT NULL,
    started_at TIMESTAMPTZ NOT NULL,
    ended_at TIMESTAMPTZ,
    minutes INT NOT NULL DEFAULT 0 CHECK (minutes BETWEEN 0 AND 1440),
    note VARCHAR(500) NOT NULL DEFAULT '',
    source VARCHAR(8) NOT NULL DEFAULT 'manual' CHECK (source IN ('manual', 'timer')),
    auto_stopped BOOLEAN NOT NULL DEFAULT FALSE,
    created_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP,
    CONSTRAINT time_entries_span CHECK (ended_at IS NULL OR ended_at >= started_at)
);
CREATE UNIQUE INDEX uq_time_entries_running ON time_entries (tenant_id, user_id) WHERE ended_at IS NULL;
CREATE INDEX idx_time_entries_task ON time_entries (tenant_id, task_id, started_at DESC);
CREATE INDEX idx_time_entries_user ON time_entries (tenant_id, user_id, started_at);
CREATE INDEX idx_time_entries_stale ON time_entries (started_at) WHERE ended_at IS NULL;
