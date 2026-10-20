-- Re-baselined into schema hr (DHR-09): no foreign keys into identity tables; tenant and user ids are
-- checked through identityport.
CREATE TABLE performance_checkins (
    id UUID PRIMARY KEY,
    tenant_id UUID NOT NULL,
    goal_id UUID NOT NULL REFERENCES performance_goals(id) ON DELETE CASCADE,
    key_result_id UUID REFERENCES performance_key_results(id) ON DELETE CASCADE,
    from_progress NUMERIC(5,2) NOT NULL CHECK (from_progress >= 0 AND from_progress <= 100),
    to_progress NUMERIC(5,2) NOT NULL CHECK (to_progress >= 0 AND to_progress <= 100),
    value NUMERIC(14,2),
    note VARCHAR(1000),
    author_user_id UUID NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP
);

-- PM-007: append-only progress history.
CREATE INDEX idx_performance_checkins_goal ON performance_checkins (goal_id);
