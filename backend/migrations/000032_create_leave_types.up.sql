CREATE TABLE leave_types (
    id UUID PRIMARY KEY,
    tenant_id UUID NOT NULL REFERENCES tenants(id) ON DELETE CASCADE,
    name VARCHAR(100) NOT NULL,
    code VARCHAR(10) NOT NULL CHECK (code ~ '^[A-Z0-9_]{2,10}$'),
    is_paid BOOLEAN NOT NULL DEFAULT TRUE,
    annual_allowance NUMERIC(7,2) NOT NULL CHECK (annual_allowance BETWEEN 0 AND 365),
    accrual VARCHAR(10) NOT NULL DEFAULT 'annual' CHECK (accrual IN ('annual', 'monthly')),
    carry_forward_limit NUMERIC(7,2) NOT NULL DEFAULT 0 CHECK (carry_forward_limit BETWEEN 0 AND 365),
    max_consecutive_days INT CHECK (max_consecutive_days BETWEEN 1 AND 365),
    min_notice_days INT NOT NULL DEFAULT 0 CHECK (min_notice_days BETWEEN 0 AND 90),
    allow_half_day BOOLEAN NOT NULL DEFAULT TRUE,
    sandwich_rule BOOLEAN NOT NULL DEFAULT FALSE,
    applicable_gender VARCHAR(10) NOT NULL DEFAULT 'all' CHECK (applicable_gender IN ('all', 'male', 'female')),
    status VARCHAR(20) NOT NULL DEFAULT 'active',
    created_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP,
    deleted_at TIMESTAMPTZ
);

CREATE UNIQUE INDEX uq_leave_types_tenant_name ON leave_types (tenant_id, lower(name)) WHERE deleted_at IS NULL;
CREATE UNIQUE INDEX uq_leave_types_tenant_code ON leave_types (tenant_id, code) WHERE deleted_at IS NULL;
CREATE INDEX idx_leave_types_tenant_id ON leave_types (tenant_id);
CREATE INDEX idx_leave_types_status ON leave_types (status);
CREATE INDEX idx_leave_types_deleted_at ON leave_types (deleted_at);
