CREATE TABLE leave_holidays (
    id UUID PRIMARY KEY,
    tenant_id UUID NOT NULL REFERENCES tenants(id) ON DELETE CASCADE,
    holiday_date DATE NOT NULL,
    name VARCHAR(100) NOT NULL,
    is_optional BOOLEAN NOT NULL DEFAULT FALSE,
    created_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP,
    deleted_at TIMESTAMPTZ
);

-- FR-HD001: one holiday per tenant per date.
CREATE UNIQUE INDEX uq_leave_holidays_tenant_date ON leave_holidays (tenant_id, holiday_date) WHERE deleted_at IS NULL;
CREATE INDEX idx_leave_holidays_deleted_at ON leave_holidays (deleted_at);
