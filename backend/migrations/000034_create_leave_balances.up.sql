CREATE TABLE leave_balances (
    id UUID PRIMARY KEY,
    tenant_id UUID NOT NULL REFERENCES tenants(id) ON DELETE CASCADE,
    employee_profile_id UUID NOT NULL REFERENCES employee_profiles(id) ON DELETE CASCADE,
    leave_type_id UUID NOT NULL REFERENCES leave_types(id) ON DELETE CASCADE,
    year INT NOT NULL CHECK (year BETWEEN 2000 AND 2200),
    opening NUMERIC(7,2) NOT NULL DEFAULT 0,
    accrued NUMERIC(7,2) NOT NULL DEFAULT 0,
    adjusted NUMERIC(7,2) NOT NULL DEFAULT 0,
    used NUMERIC(7,2) NOT NULL DEFAULT 0,
    reserved NUMERIC(7,2) NOT NULL DEFAULT 0 CHECK (reserved >= 0),
    created_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP
);

-- D4-02: projection of leave_ledger; available = opening + accrued + adjusted - used - reserved (computed in code).
CREATE UNIQUE INDEX uq_leave_balances_employee_type_year ON leave_balances (tenant_id, employee_profile_id, leave_type_id, year);
