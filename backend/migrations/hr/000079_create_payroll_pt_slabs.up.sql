-- HR/Payroll tenant professional-tax slabs per state (PP-02, PP-09).
CREATE TABLE payroll_pt_slabs (
    id UUID PRIMARY KEY,
    tenant_id UUID NOT NULL,
    state_code VARCHAR(2) NOT NULL,
    min_gross NUMERIC(14,2) NOT NULL CHECK (min_gross >= 0),
    max_gross NUMERIC(14,2) CHECK (max_gross IS NULL OR max_gross >= min_gross),
    amount NUMERIC(14,2) NOT NULL CHECK (amount >= 0),
    month SMALLINT CHECK (month IS NULL OR month BETWEEN 1 AND 12),
    created_by_user_id UUID NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP
);
CREATE INDEX idx_payroll_pt_slabs_state ON payroll_pt_slabs (tenant_id, state_code);
