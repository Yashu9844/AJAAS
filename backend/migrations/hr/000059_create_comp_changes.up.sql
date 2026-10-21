-- HR/Compensation off-cycle changes (CO-C10, CO-C13).
CREATE TABLE comp_changes (
    id UUID PRIMARY KEY,
    tenant_id UUID NOT NULL,
    employee_profile_id UUID NOT NULL REFERENCES employee_profiles(id),
    annual_base NUMERIC(14,2) NOT NULL CHECK (annual_base > 0),
    effective_from DATE NOT NULL,
    reason VARCHAR(20) NOT NULL CHECK (reason IN ('hire', 'promotion', 'market', 'correction')),
    comment VARCHAR(1000),
    status VARCHAR(20) NOT NULL CHECK (status IN ('pending', 'approved', 'rejected', 'cancelled')),
    requested_by_user_id UUID NOT NULL,
    decided_by_user_id UUID,
    decision_comment VARCHAR(1000),
    decided_at TIMESTAMPTZ,
    created_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP
);
CREATE UNIQUE INDEX uq_comp_changes_pending ON comp_changes (employee_profile_id) WHERE status = 'pending';
CREATE INDEX idx_comp_changes_tenant ON comp_changes (tenant_id, status, created_at DESC);
