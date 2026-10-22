-- HR/Leave encashment rules and requests (LP-12..LP-15).
CREATE TABLE leave_encashment_rules (
    tenant_id UUID NOT NULL,
    leave_type_id UUID NOT NULL REFERENCES leave_types(id),
    max_days_per_year NUMERIC(7,2) NOT NULL CHECK (max_days_per_year BETWEEN 0 AND 365),
    min_balance_after NUMERIC(7,2) NOT NULL DEFAULT 0 CHECK (min_balance_after >= 0),
    updated_by_user_id UUID NOT NULL,
    updated_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP,
    PRIMARY KEY (tenant_id, leave_type_id)
);

CREATE TABLE leave_encashments (
    id UUID PRIMARY KEY,
    tenant_id UUID NOT NULL,
    employee_profile_id UUID NOT NULL REFERENCES employee_profiles(id),
    leave_type_id UUID NOT NULL REFERENCES leave_types(id),
    year INT NOT NULL,
    days NUMERIC(7,2) NOT NULL CHECK (days > 0),
    reason VARCHAR(500),
    status VARCHAR(20) NOT NULL CHECK (status IN ('pending', 'approved', 'rejected', 'cancelled')),
    requested_by_user_id UUID NOT NULL,
    decided_by_user_id UUID,
    decision_comment VARCHAR(500),
    decided_at TIMESTAMPTZ,
    paid_out_at TIMESTAMPTZ,
    created_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP
);
CREATE UNIQUE INDEX uq_leave_encashments_pending ON leave_encashments (employee_profile_id, leave_type_id) WHERE status = 'pending';
CREATE INDEX idx_leave_encashments_tenant ON leave_encashments (tenant_id, status, created_at DESC);
CREATE INDEX idx_leave_encashments_payout ON leave_encashments (tenant_id, employee_profile_id) WHERE status = 'approved' AND paid_out_at IS NULL;
