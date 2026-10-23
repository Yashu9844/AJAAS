ALTER TABLE payroll_runs DROP COLUMN IF EXISTS employer_total;
ALTER TABLE payroll_payslips DROP COLUMN IF EXISTS segments;
ALTER TABLE payroll_payslips DROP COLUMN IF EXISTS employer_cost;
DELETE FROM payroll_payslip_lines WHERE kind = 'employer';
ALTER TABLE payroll_payslip_lines DROP CONSTRAINT IF EXISTS payroll_payslip_lines_kind_check;
ALTER TABLE payroll_payslip_lines ADD CONSTRAINT payroll_payslip_lines_kind_check CHECK (kind IN ('earning', 'deduction'));
