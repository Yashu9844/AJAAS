CREATE TABLE recruitment_candidates (
    id UUID PRIMARY KEY,
    tenant_id UUID NOT NULL REFERENCES tenants(id) ON DELETE CASCADE,
    job_id UUID NOT NULL REFERENCES recruitment_jobs(id) ON DELETE CASCADE,
    first_name VARCHAR(100) NOT NULL,
    last_name VARCHAR(100) NOT NULL,
    email VARCHAR(255) NOT NULL,
    phone VARCHAR(30),
    source VARCHAR(20) NOT NULL CHECK (source IN ('career_site', 'referral', 'agency', 'linkedin', 'other')),
    resume_url VARCHAR(500),
    expected_ctc NUMERIC(14,2) CHECK (expected_ctc IS NULL OR expected_ctc >= 0),
    notice_period_days INT CHECK (notice_period_days IS NULL OR notice_period_days BETWEEN 0 AND 365),
    stage VARCHAR(20) NOT NULL DEFAULT 'applied' CHECK (stage IN ('applied', 'screening', 'interview', 'offer', 'hired', 'rejected', 'withdrawn')),
    hired_user_id UUID,
    hired_employee_id UUID,
    created_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP
);

-- FR-CD001: one application per email per job, case-insensitive.
CREATE UNIQUE INDEX uq_recruitment_candidates_job_email ON recruitment_candidates (job_id, lower(email));
CREATE INDEX idx_recruitment_candidates_job_stage ON recruitment_candidates (tenant_id, job_id, stage);
