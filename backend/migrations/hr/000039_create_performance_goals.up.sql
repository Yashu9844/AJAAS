-- Re-baselined into schema hr (DHR-09): no foreign keys into identity tables; tenant and user ids are
-- checked through identityport.
CREATE TABLE performance_goals (
    id UUID PRIMARY KEY,
    tenant_id UUID NOT NULL,
    cycle_id UUID NOT NULL REFERENCES performance_cycles(id) ON DELETE CASCADE,
    employee_id UUID NOT NULL REFERENCES employee_profiles(id),
    parent_goal_id UUID REFERENCES performance_goals(id),
    title VARCHAR(200) NOT NULL,
    description TEXT,
    weight NUMERIC(5,2) NOT NULL CHECK (weight > 0 AND weight <= 100),
    progress NUMERIC(5,2) NOT NULL DEFAULT 0 CHECK (progress >= 0 AND progress <= 100),
    status VARCHAR(20) NOT NULL DEFAULT 'active' CHECK (status IN ('active', 'completed', 'cancelled')),
    created_by_user_id UUID NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP,
    CHECK (parent_goal_id IS NULL OR parent_goal_id <> id)
);

CREATE INDEX idx_performance_goals_cycle_employee ON performance_goals (tenant_id, cycle_id, employee_id);
CREATE INDEX idx_performance_goals_parent ON performance_goals (parent_goal_id);
