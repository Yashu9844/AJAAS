CREATE TABLE recruitment_offers (
    id UUID PRIMARY KEY,
    tenant_id UUID NOT NULL REFERENCES tenants(id) ON DELETE CASCADE,
    candidate_id UUID NOT NULL REFERENCES recruitment_candidates(id) ON DELETE CASCADE,
    offered_ctc NUMERIC(14,2) NOT NULL CHECK (offered_ctc > 0),
    joining_date DATE NOT NULL,
    expires_on DATE,
    status VARCHAR(20) NOT NULL DEFAULT 'offered' CHECK (status IN ('offered', 'accepted', 'declined', 'withdrawn')),
    decided_at TIMESTAMPTZ,
    created_by_user_id UUID NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP
);

-- RC-006: at most one open offer per candidate.
CREATE UNIQUE INDEX uq_recruitment_offers_open ON recruitment_offers (candidate_id) WHERE status = 'offered';
CREATE INDEX idx_recruitment_offers_candidate ON recruitment_offers (candidate_id);
