CREATE TABLE recruitment_interviews (
    id UUID PRIMARY KEY,
    tenant_id UUID NOT NULL REFERENCES tenants(id) ON DELETE CASCADE,
    candidate_id UUID NOT NULL REFERENCES recruitment_candidates(id) ON DELETE CASCADE,
    round_name VARCHAR(100) NOT NULL,
    interviewer_employee_id UUID NOT NULL REFERENCES employee_profiles(id),
    scheduled_at TIMESTAMPTZ NOT NULL,
    duration_mins INT NOT NULL CHECK (duration_mins BETWEEN 15 AND 480),
    meeting_link VARCHAR(500),
    status VARCHAR(20) NOT NULL DEFAULT 'scheduled' CHECK (status IN ('scheduled', 'completed', 'cancelled')),
    rating INT CHECK (rating IS NULL OR rating BETWEEN 1 AND 5),
    recommendation VARCHAR(20) CHECK (recommendation IS NULL OR recommendation IN ('strong_hire', 'hire', 'hold', 'reject')),
    feedback TEXT,
    feedback_at TIMESTAMPTZ,
    created_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP
);

CREATE INDEX idx_recruitment_interviews_interviewer ON recruitment_interviews (tenant_id, interviewer_employee_id);
CREATE INDEX idx_recruitment_interviews_candidate ON recruitment_interviews (candidate_id);
