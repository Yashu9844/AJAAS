-- Re-baselined into schema hr (DHR-09): no foreign keys into identity tables; tenant and user ids are
-- checked through identityport.
CREATE TABLE performance_key_results (
    id UUID PRIMARY KEY,
    tenant_id UUID NOT NULL,
    goal_id UUID NOT NULL REFERENCES performance_goals(id) ON DELETE CASCADE,
    title VARCHAR(200) NOT NULL,
    unit VARCHAR(20),
    start_value NUMERIC(14,2) NOT NULL,
    target_value NUMERIC(14,2) NOT NULL,
    current_value NUMERIC(14,2) NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP,
    CHECK (target_value <> start_value)
);

CREATE INDEX idx_performance_key_results_goal ON performance_key_results (goal_id);
