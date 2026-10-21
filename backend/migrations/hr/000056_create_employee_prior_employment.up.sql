-- HR/Employee prior employment (FR-ER003): periods never overlap per worker.
CREATE TABLE employee_prior_employment (
    id UUID PRIMARY KEY,
    tenant_id UUID NOT NULL,
    employee_profile_id UUID NOT NULL REFERENCES employee_profiles(id) ON DELETE CASCADE,
    employer VARCHAR(200) NOT NULL,
    title VARCHAR(150) NOT NULL,
    start_date DATE NOT NULL,
    end_date DATE NOT NULL,
    reason_for_leaving VARCHAR(500),
    created_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP,
    CHECK (end_date >= start_date),
    CONSTRAINT ex_employee_prior_employment_overlap EXCLUDE USING gist (
        employee_profile_id WITH =, daterange(start_date, end_date, '[]') WITH &&
    )
);
CREATE INDEX idx_employee_prior_employment_profile ON employee_prior_employment (tenant_id, employee_profile_id);
