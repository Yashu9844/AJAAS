CREATE TABLE payroll_payslips (
    id UUID PRIMARY KEY,
    tenant_id UUID NOT NULL REFERENCES tenants(id) ON DELETE CASCADE,
    run_id UUID NOT NULL REFERENCES payroll_runs(id) ON DELETE CASCADE,
    employee_profile_id UUID NOT NULL REFERENCES employee_profiles(id) ON DELETE CASCADE,
    employee_code VARCHAR(50) NOT NULL,
    employee_name VARCHAR(200) NOT NULL,
    structure_id UUID NOT NULL REFERENCES payroll_structures(id),
    annual_ctc NUMERIC(14,2) NOT NULL,
    days_in_month INT NOT NULL CHECK (days_in_month BETWEEN 28 AND 31),
    payable_days NUMERIC(7,2) NOT NULL CHECK (payable_days >= 0),
    lop_days NUMERIC(7,2) NOT NULL DEFAULT 0 CHECK (lop_days >= 0),
    gross NUMERIC(14,2) NOT NULL,
    deductions NUMERIC(14,2) NOT NULL,
    net NUMERIC(14,2) NOT NULL CHECK (net >= 0),
    created_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP
);

CREATE UNIQUE INDEX uq_payroll_payslips_run_employee ON payroll_payslips (run_id, employee_profile_id);
CREATE INDEX idx_payroll_payslips_employee ON payroll_payslips (tenant_id, employee_profile_id);
