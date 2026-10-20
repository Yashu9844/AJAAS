-- HR/Performance continuous feedback (FB-001..FB-003): immutable once given.
CREATE TABLE performance_feedback (
    id UUID PRIMARY KEY,
    tenant_id UUID NOT NULL,
    from_employee_id UUID NOT NULL REFERENCES employee_profiles(id),
    to_employee_id UUID NOT NULL REFERENCES employee_profiles(id),
    kind VARCHAR(20) NOT NULL CHECK (kind IN ('praise', 'constructive')),
    visibility VARCHAR(10) NOT NULL CHECK (visibility IN ('private', 'public')),
    message VARCHAR(2000) NOT NULL,
    request_id UUID REFERENCES performance_feedback_requests(id),
    created_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP,
    CHECK (from_employee_id <> to_employee_id),
    CHECK (visibility = 'private' OR kind = 'praise')
);

CREATE INDEX idx_performance_feedback_to ON performance_feedback (tenant_id, to_employee_id, created_at DESC);
CREATE INDEX idx_performance_feedback_from ON performance_feedback (tenant_id, from_employee_id, created_at DESC);
CREATE INDEX idx_performance_feedback_public ON performance_feedback (tenant_id, created_at DESC) WHERE visibility = 'public';
