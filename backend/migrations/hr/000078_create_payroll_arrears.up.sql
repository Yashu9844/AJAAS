-- HR/Payroll arrears (PP-04): the amount a run settled for an earlier finalized payslip.
CREATE TABLE payroll_arrears (
    id UUID PRIMARY KEY,
    tenant_id UUID NOT NULL,
    run_id UUID NOT NULL REFERENCES payroll_runs(id) ON DELETE CASCADE,
    employee_profile_id UUID NOT NULL REFERENCES employee_profiles(id),
    source_payslip_id UUID NOT NULL REFERENCES payroll_payslips(id) ON DELETE CASCADE,
    amount NUMERIC(14,2) NOT NULL CHECK (amount <> 0),
    created_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP,
    UNIQUE (run_id, source_payslip_id)
);
CREATE INDEX idx_payroll_arrears_source ON payroll_arrears (tenant_id, source_payslip_id);
