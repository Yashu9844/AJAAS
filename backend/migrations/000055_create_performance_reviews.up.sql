CREATE TABLE performance_reviews (
    id UUID PRIMARY KEY,
    tenant_id UUID NOT NULL REFERENCES tenants(id) ON DELETE CASCADE,
    cycle_id UUID NOT NULL REFERENCES performance_cycles(id) ON DELETE CASCADE,
    employee_id UUID NOT NULL REFERENCES employee_profiles(id),
    reviewer_employee_id UUID NOT NULL REFERENCES employee_profiles(id),
    type VARCHAR(10) NOT NULL CHECK (type IN ('self', 'manager', 'peer')),
    status VARCHAR(20) NOT NULL DEFAULT 'pending' CHECK (status IN ('pending', 'submitted', 'expired')),
    ratings JSONB NOT NULL DEFAULT '[]'::jsonb CHECK (jsonb_typeof(ratings) = 'array'),
    strengths TEXT,
    improvements TEXT,
    overall_score NUMERIC(3,2) CHECK (overall_score IS NULL OR (overall_score >= 1 AND overall_score <= 5)),
    potential VARCHAR(10) CHECK (potential IS NULL OR potential IN ('low', 'medium', 'high')),
    submitted_at TIMESTAMPTZ,
    created_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP,
    CHECK ((type = 'self') = (reviewer_employee_id = employee_id)),
    CHECK (status <> 'submitted' OR (overall_score IS NOT NULL AND submitted_at IS NOT NULL))
);

-- PM-010/PM-011: one review per (cycle, subject, reviewer, type).
CREATE UNIQUE INDEX uq_performance_reviews_assignment ON performance_reviews (cycle_id, employee_id, reviewer_employee_id, type);
CREATE INDEX idx_performance_reviews_reviewer ON performance_reviews (tenant_id, reviewer_employee_id);
