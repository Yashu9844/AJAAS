-- Re-baselined into schema hr (DHR-09): no foreign keys into identity tables; tenant and user ids are
-- checked through identityport.
CREATE TABLE recruitment_stage_events (
    id UUID PRIMARY KEY,
    tenant_id UUID NOT NULL,
    candidate_id UUID NOT NULL REFERENCES recruitment_candidates(id) ON DELETE CASCADE,
    from_stage VARCHAR(20),
    to_stage VARCHAR(20) NOT NULL,
    actor_user_id UUID NOT NULL,
    note VARCHAR(500),
    created_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP
);

-- RC-003: append-only pipeline history.
CREATE INDEX idx_recruitment_stage_events_candidate ON recruitment_stage_events (candidate_id);
