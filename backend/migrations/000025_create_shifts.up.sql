CREATE TABLE shifts (
    id UUID PRIMARY KEY,
    tenant_id UUID NOT NULL REFERENCES tenants(id) ON DELETE CASCADE,
    name VARCHAR(100) NOT NULL,
    code VARCHAR(32),
    start_minute INT NOT NULL CHECK (start_minute BETWEEN 0 AND 1439),
    end_minute INT NOT NULL CHECK (end_minute BETWEEN 0 AND 1439),
    grace_period_mins INT NOT NULL DEFAULT 15 CHECK (grace_period_mins BETWEEN 0 AND 240),
    break_duration_mins INT NOT NULL DEFAULT 60 CHECK (break_duration_mins BETWEEN 0 AND 480),
    full_day_minutes INT CHECK (full_day_minutes BETWEEN 1 AND 1440),
    half_day_minutes INT CHECK (half_day_minutes BETWEEN 1 AND 1440),
    timezone VARCHAR(64) NOT NULL DEFAULT 'UTC',
    is_night_shift BOOLEAN NOT NULL DEFAULT FALSE,
    status VARCHAR(20) NOT NULL DEFAULT 'active',
    created_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP,
    deleted_at TIMESTAMPTZ,
    CONSTRAINT chk_shifts_start_ne_end CHECK (start_minute <> end_minute)
);

CREATE UNIQUE INDEX uq_shifts_tenant_name ON shifts (tenant_id, lower(name)) WHERE deleted_at IS NULL;
CREATE UNIQUE INDEX uq_shifts_tenant_code ON shifts (tenant_id, code) WHERE deleted_at IS NULL AND code IS NOT NULL;
CREATE INDEX idx_shifts_tenant_id ON shifts (tenant_id);
CREATE INDEX idx_shifts_status ON shifts (status);
CREATE INDEX idx_shifts_deleted_at ON shifts (deleted_at);
