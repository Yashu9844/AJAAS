-- HR/Compensation records (CO-C11): a worker's annual base over [effective_from, effective_to), never overlapping.
CREATE TABLE comp_records (
    id UUID PRIMARY KEY,
    tenant_id UUID NOT NULL,
    employee_profile_id UUID NOT NULL REFERENCES employee_profiles(id),
    annual_base NUMERIC(14,2) NOT NULL CHECK (annual_base > 0),
    effective_from DATE NOT NULL,
    effective_to DATE,
    reason VARCHAR(20) NOT NULL CHECK (reason IN ('hire', 'promotion', 'market', 'correction', 'merit')),
    change_id UUID REFERENCES comp_changes(id),
    merit_cycle_id UUID REFERENCES comp_merit_cycles(id),
    payroll_synced BOOLEAN NOT NULL DEFAULT FALSE,
    created_by_user_id UUID NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP,
    CHECK (effective_to IS NULL OR effective_to > effective_from),
    CONSTRAINT ex_comp_records_overlap EXCLUDE USING gist (tenant_id WITH =, employee_profile_id WITH =, daterange(effective_from, effective_to) WITH &&)
);
