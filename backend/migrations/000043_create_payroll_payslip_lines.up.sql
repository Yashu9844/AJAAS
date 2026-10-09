CREATE TABLE payroll_payslip_lines (
    id UUID PRIMARY KEY,
    payslip_id UUID NOT NULL REFERENCES payroll_payslips(id) ON DELETE CASCADE,
    code VARCHAR(20) NOT NULL,
    name VARCHAR(100) NOT NULL,
    kind VARCHAR(10) NOT NULL CHECK (kind IN ('earning', 'deduction')),
    amount NUMERIC(14,2) NOT NULL CHECK (amount >= 0),
    position INT NOT NULL
);

CREATE INDEX idx_payroll_payslip_lines_payslip ON payroll_payslip_lines (payslip_id);
