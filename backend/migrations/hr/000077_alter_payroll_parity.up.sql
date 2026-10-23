-- HR/Payroll parity (PP-01, PP-03): employer lines, employer cost and the priced segments of a payslip.
ALTER TABLE payroll_payslip_lines DROP CONSTRAINT IF EXISTS payroll_payslip_lines_kind_check;
ALTER TABLE payroll_payslip_lines ADD CONSTRAINT payroll_payslip_lines_kind_check CHECK (kind IN ('earning', 'deduction', 'employer'));
ALTER TABLE payroll_payslips ADD COLUMN employer_cost NUMERIC(14,2) NOT NULL DEFAULT 0;
ALTER TABLE payroll_payslips ADD COLUMN segments JSONB NOT NULL DEFAULT '[]'::jsonb;
ALTER TABLE payroll_runs ADD COLUMN employer_total NUMERIC(14,2) NOT NULL DEFAULT 0;
