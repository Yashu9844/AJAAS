CREATE TABLE recruitment_jobs (
    id UUID PRIMARY KEY,
    tenant_id UUID NOT NULL REFERENCES tenants(id) ON DELETE CASCADE,
    title VARCHAR(150) NOT NULL,
    department_id UUID,
    designation_id UUID,
    headcount INT NOT NULL CHECK (headcount BETWEEN 1 AND 1000),
    hired_count INT NOT NULL DEFAULT 0 CHECK (hired_count >= 0 AND hired_count <= headcount),
    location VARCHAR(100),
    employment_type VARCHAR(20) NOT NULL CHECK (employment_type IN ('full_time', 'part_time', 'contract', 'intern')),
    min_experience_years INT NOT NULL DEFAULT 0 CHECK (min_experience_years BETWEEN 0 AND 50),
    description TEXT NOT NULL,
    status VARCHAR(20) NOT NULL DEFAULT 'draft' CHECK (status IN ('draft', 'open', 'on_hold', 'closed', 'filled')),
    opened_at TIMESTAMPTZ,
    closed_at TIMESTAMPTZ,
    created_by_user_id UUID NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP
);

CREATE INDEX idx_recruitment_jobs_tenant_status ON recruitment_jobs (tenant_id, status);
