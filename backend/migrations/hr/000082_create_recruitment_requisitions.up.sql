-- HR/Recruitment position-linked requisitions (RP-02, RP-03).
CREATE TABLE recruitment_requisitions (
    id UUID PRIMARY KEY,
    tenant_id UUID NOT NULL,
    position_id UUID NOT NULL REFERENCES core_positions(id),
    title VARCHAR(150) NOT NULL,
    department_id UUID NOT NULL,
    headcount INT NOT NULL CHECK (headcount BETWEEN 1 AND 100),
    justification VARCHAR(1000) NOT NULL,
    status VARCHAR(20) NOT NULL CHECK (status IN ('draft', 'submitted', 'approved', 'rejected', 'cancelled')),
    requested_by_user_id UUID NOT NULL,
    decided_by_user_id UUID,
    decision_comment VARCHAR(500),
    decided_at TIMESTAMPTZ,
    job_id UUID REFERENCES recruitment_jobs(id),
    created_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP
);
CREATE INDEX idx_recruitment_requisitions_tenant ON recruitment_requisitions (tenant_id, status, created_at DESC);
CREATE INDEX idx_recruitment_requisitions_position ON recruitment_requisitions (tenant_id, position_id);
