CREATE TABLE payroll_assignments (
    id UUID PRIMARY KEY,
    tenant_id UUID NOT NULL REFERENCES tenants(id) ON DELETE CASCADE,
    employee_profile_id UUID NOT NULL REFERENCES employee_profiles(id) ON DELETE CASCADE,
    structure_id UUID NOT NULL REFERENCES payroll_structures(id),
    annual_ctc NUMERIC(14,2) NOT NULL CHECK (annual_ctc > 0),
    effective_from DATE NOT NULL,
    effective_to DATE,
    created_by_user_id UUID NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP,
    CONSTRAINT chk_payroll_assignments_range CHECK (effective_to IS NULL OR effective_to >= effective_from)
);

-- PY-014: append-only history per employee.
CREATE INDEX idx_payroll_assignments_employee ON payroll_assignments (tenant_id, employee_profile_id, effective_from);
