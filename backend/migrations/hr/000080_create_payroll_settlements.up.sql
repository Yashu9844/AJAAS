-- HR/Payroll final settlements (PP-06..PP-08).
CREATE TABLE payroll_settlements (
    id UUID PRIMARY KEY,
    tenant_id UUID NOT NULL,
    employee_profile_id UUID NOT NULL REFERENCES employee_profiles(id),
    status VARCHAR(20) NOT NULL CHECK (status IN ('draft', 'approved', 'paid', 'cancelled')),
    last_working_day DATE NOT NULL,
    service_years INT NOT NULL,
    monthly_basic NUMERIC(14,2) NOT NULL,
    gratuity NUMERIC(14,2) NOT NULL DEFAULT 0,
    leave_encashment NUMERIC(14,2) NOT NULL DEFAULT 0,
    encashments NUMERIC(14,2) NOT NULL DEFAULT 0,
    notice_recovery NUMERIC(14,2) NOT NULL DEFAULT 0,
    recovery_shortfall NUMERIC(14,2) NOT NULL DEFAULT 0,
    net NUMERIC(14,2) NOT NULL CHECK (net >= 0),
    encashment_ids JSONB NOT NULL DEFAULT '[]'::jsonb,
    calculated_by_user_id UUID NOT NULL,
    approved_by_user_id UUID,
    calculated_at TIMESTAMPTZ NOT NULL,
    approved_at TIMESTAMPTZ,
    paid_at TIMESTAMPTZ,
    created_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP
);
CREATE UNIQUE INDEX uq_payroll_settlements_open ON payroll_settlements (employee_profile_id) WHERE status IN ('draft', 'approved');
CREATE INDEX idx_payroll_settlements_tenant ON payroll_settlements (tenant_id, status, created_at DESC);

CREATE TABLE payroll_settlement_lines (
    id UUID PRIMARY KEY,
    settlement_id UUID NOT NULL REFERENCES payroll_settlements(id) ON DELETE CASCADE,
    code VARCHAR(30) NOT NULL,
    name VARCHAR(100) NOT NULL,
    kind VARCHAR(10) NOT NULL CHECK (kind IN ('earning', 'deduction')),
    amount NUMERIC(14,2) NOT NULL CHECK (amount >= 0),
    position INT NOT NULL
);
CREATE INDEX idx_payroll_settlement_lines ON payroll_settlement_lines (settlement_id);
