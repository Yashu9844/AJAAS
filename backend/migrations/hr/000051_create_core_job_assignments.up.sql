-- HR/Core effective-dated job assignments (CO-020..CO-022): [effective_from, effective_to), never overlapping (DC-03).
CREATE TABLE core_job_assignments (
    id UUID PRIMARY KEY,
    tenant_id UUID NOT NULL,
    employee_id UUID NOT NULL REFERENCES employee_profiles(id),
    action_id UUID NOT NULL REFERENCES core_hr_actions(id),
    job_profile_id UUID REFERENCES core_job_profiles(id),
    position_id UUID REFERENCES core_positions(id),
    department_id UUID REFERENCES departments(id),
    location_id UUID REFERENCES locations(id),
    manager_employee_id UUID REFERENCES employee_profiles(id),
    grade_id UUID REFERENCES core_grades(id),
    worker_type VARCHAR(20) NOT NULL CHECK (worker_type IN ('employee', 'contractor', 'intern')),
    employment_type VARCHAR(20) NOT NULL CHECK (employment_type IN ('full_time', 'part_time', 'contract', 'intern')),
    fte NUMERIC(5,2) NOT NULL DEFAULT 1 CHECK (fte > 0 AND fte <= 1),
    probation_end_date DATE,
    effective_from DATE NOT NULL,
    effective_to DATE,
    status VARCHAR(20) NOT NULL DEFAULT 'active' CHECK (status IN ('active', 'rescinded')),
    created_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP,
    CHECK (effective_to IS NULL OR effective_to > effective_from),
    CHECK (manager_employee_id IS NULL OR manager_employee_id <> employee_id),
    CONSTRAINT ex_core_job_assignments_overlap EXCLUDE USING gist (
        tenant_id WITH =, employee_id WITH =, daterange(effective_from, effective_to) WITH &&
    ) WHERE (status = 'active')
);
CREATE INDEX idx_core_job_assignments_employee ON core_job_assignments (tenant_id, employee_id, effective_from);
CREATE INDEX idx_core_job_assignments_position ON core_job_assignments (tenant_id, position_id) WHERE status = 'active';
CREATE INDEX idx_core_job_assignments_department ON core_job_assignments (tenant_id, department_id) WHERE status = 'active';
