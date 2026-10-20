-- Re-baselined into schema hr (DHR-09): no foreign keys into identity tables; tenant and user ids are
-- checked through identityport.
CREATE TABLE performance_cycles (
    id UUID PRIMARY KEY,
    tenant_id UUID NOT NULL,
    title VARCHAR(150) NOT NULL,
    start_date DATE NOT NULL,
    end_date DATE NOT NULL,
    criteria JSONB NOT NULL DEFAULT '[]'::jsonb CHECK (jsonb_typeof(criteria) = 'array'),
    status VARCHAR(20) NOT NULL DEFAULT 'draft' CHECK (status IN ('draft', 'active', 'review', 'completed')),
    activated_at TIMESTAMPTZ,
    review_started_at TIMESTAMPTZ,
    completed_at TIMESTAMPTZ,
    created_by_user_id UUID NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP,
    CHECK (end_date >= start_date)
);

CREATE INDEX idx_performance_cycles_tenant_status ON performance_cycles (tenant_id, status);
