-- HR/Recruitment competency scorecards and interview scores (RP-04, RP-05).
CREATE TABLE recruitment_scorecards (
    id UUID PRIMARY KEY,
    tenant_id UUID NOT NULL,
    name VARCHAR(150) NOT NULL,
    competencies JSONB NOT NULL,
    status VARCHAR(20) NOT NULL CHECK (status IN ('active', 'archived')),
    created_by_user_id UUID NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP
);
CREATE INDEX idx_recruitment_scorecards_tenant ON recruitment_scorecards (tenant_id, status);

CREATE TABLE recruitment_job_scorecards (
    tenant_id UUID NOT NULL,
    job_id UUID PRIMARY KEY REFERENCES recruitment_jobs(id) ON DELETE CASCADE,
    scorecard_id UUID NOT NULL REFERENCES recruitment_scorecards(id)
);

CREATE TABLE recruitment_interview_scores (
    tenant_id UUID NOT NULL,
    interview_id UUID NOT NULL REFERENCES recruitment_interviews(id) ON DELETE CASCADE,
    competency VARCHAR(100) NOT NULL,
    score INT NOT NULL CHECK (score BETWEEN 1 AND 5),
    note VARCHAR(500),
    created_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP,
    PRIMARY KEY (interview_id, competency)
);
