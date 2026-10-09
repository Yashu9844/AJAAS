CREATE TABLE leave_requests (
    id UUID PRIMARY KEY,
    tenant_id UUID NOT NULL REFERENCES tenants(id) ON DELETE CASCADE,
    employee_profile_id UUID NOT NULL REFERENCES employee_profiles(id) ON DELETE CASCADE,
    leave_type_id UUID NOT NULL REFERENCES leave_types(id),
    start_date DATE NOT NULL,
    end_date DATE NOT NULL,
    half_day VARCHAR(12) CHECK (half_day IN ('first_half', 'second_half')),
    total_days NUMERIC(7,2) NOT NULL CHECK (total_days > 0),
    reason TEXT NOT NULL,
    status VARCHAR(20) NOT NULL DEFAULT 'pending' CHECK (status IN ('pending', 'approved', 'rejected', 'cancelled')),
    requested_by_user_id UUID NOT NULL,
    reviewer_user_id UUID,
    reviewed_at TIMESTAMPTZ,
    review_comment VARCHAR(500),
    cancelled_at TIMESTAMPTZ,
    created_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP,
    CONSTRAINT chk_leave_requests_range CHECK (start_date <= end_date),
    CONSTRAINT chk_leave_requests_half_single CHECK (half_day IS NULL OR start_date = end_date)
);

CREATE INDEX idx_leave_requests_employee_start ON leave_requests (tenant_id, employee_profile_id, start_date);
CREATE INDEX idx_leave_requests_tenant_status ON leave_requests (tenant_id, status);
