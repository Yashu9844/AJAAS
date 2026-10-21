-- HR/Core positions (CO-010..CO-014): current-state attributes (DC-05); occupancy comes from assignments.
CREATE TABLE core_positions (
    id UUID PRIMARY KEY,
    tenant_id UUID NOT NULL,
    code VARCHAR(32) NOT NULL,
    title VARCHAR(150) NOT NULL,
    job_profile_id UUID NOT NULL REFERENCES core_job_profiles(id),
    department_id UUID NOT NULL REFERENCES departments(id),
    location_id UUID REFERENCES locations(id),
    budgeted_fte NUMERIC(5,2) NOT NULL DEFAULT 1 CHECK (budgeted_fte > 0 AND budgeted_fte < 100),
    shared BOOLEAN NOT NULL DEFAULT FALSE,
    status VARCHAR(20) NOT NULL DEFAULT 'active' CHECK (status IN ('active', 'frozen', 'closed')),
    created_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP
);
CREATE UNIQUE INDEX uq_core_positions_code ON core_positions (tenant_id, code);
CREATE INDEX idx_core_positions_department ON core_positions (tenant_id, department_id, status);
