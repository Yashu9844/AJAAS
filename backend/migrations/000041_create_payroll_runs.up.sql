CREATE TABLE payroll_runs (
    id UUID PRIMARY KEY,
    tenant_id UUID NOT NULL REFERENCES tenants(id) ON DELETE CASCADE,
    year INT NOT NULL CHECK (year BETWEEN 2000 AND 2200),
    month INT NOT NULL CHECK (month BETWEEN 1 AND 12),
    status VARCHAR(20) NOT NULL DEFAULT 'draft' CHECK (status IN ('draft', 'calculated', 'approved', 'finalized')),
    employee_count INT NOT NULL DEFAULT 0,
    gross_total NUMERIC(14,2) NOT NULL DEFAULT 0,
    deduction_total NUMERIC(14,2) NOT NULL DEFAULT 0,
    net_total NUMERIC(14,2) NOT NULL DEFAULT 0,
    warnings JSONB NOT NULL DEFAULT '[]',
    created_by_user_id UUID NOT NULL,
    calculated_by_user_id UUID,
    approved_by_user_id UUID,
    calculated_at TIMESTAMPTZ,
    approved_at TIMESTAMPTZ,
    finalized_at TIMESTAMPTZ,
    created_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP
);

-- PY-010: one run per tenant period.
CREATE UNIQUE INDEX uq_payroll_runs_tenant_period ON payroll_runs (tenant_id, year, month);
