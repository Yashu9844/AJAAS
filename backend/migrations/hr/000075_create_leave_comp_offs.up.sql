-- HR/Leave comp-off credits (LP-08..LP-11).
CREATE TABLE leave_comp_offs (
    id UUID PRIMARY KEY,
    tenant_id UUID NOT NULL,
    employee_profile_id UUID NOT NULL REFERENCES employee_profiles(id),
    worked_date DATE NOT NULL,
    days NUMERIC(7,2) NOT NULL CHECK (days IN (0.5, 1)),
    reason VARCHAR(500) NOT NULL,
    status VARCHAR(20) NOT NULL CHECK (status IN ('pending', 'approved', 'rejected', 'cancelled')),
    requested_by_user_id UUID NOT NULL,
    decided_by_user_id UUID,
    decision_comment VARCHAR(500),
    decided_at TIMESTAMPTZ,
    expires_on DATE,
    expired_days NUMERIC(7,2),
    expired_at TIMESTAMPTZ,
    created_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP
);
CREATE UNIQUE INDEX uq_leave_comp_offs_day ON leave_comp_offs (employee_profile_id, worked_date) WHERE status IN ('pending', 'approved');
CREATE INDEX idx_leave_comp_offs_tenant ON leave_comp_offs (tenant_id, status, created_at DESC);
CREATE INDEX idx_leave_comp_offs_expiry ON leave_comp_offs (expires_on) WHERE status = 'approved' AND expired_at IS NULL;
