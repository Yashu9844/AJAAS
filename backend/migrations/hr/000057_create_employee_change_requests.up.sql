-- HR/Employee personal-data change requests (FR-CR001..FR-CR005).
CREATE TABLE employee_change_requests (
    id UUID PRIMARY KEY,
    tenant_id UUID NOT NULL,
    employee_profile_id UUID NOT NULL REFERENCES employee_profiles(id) ON DELETE CASCADE,
    type VARCHAR(20) NOT NULL CHECK (type IN ('name', 'address', 'bank')),
    payload JSONB NOT NULL,
    reason VARCHAR(500),
    status VARCHAR(20) NOT NULL DEFAULT 'submitted' CHECK (status IN ('submitted', 'approved', 'rejected', 'cancelled')),
    requested_by_user_id UUID NOT NULL,
    decided_by_user_id UUID,
    decision_comment VARCHAR(1000),
    decided_at TIMESTAMPTZ,
    created_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP
);
CREATE UNIQUE INDEX uq_employee_change_requests_open ON employee_change_requests (employee_profile_id, type) WHERE status = 'submitted';
CREATE INDEX idx_employee_change_requests_status ON employee_change_requests (tenant_id, status, created_at DESC);
