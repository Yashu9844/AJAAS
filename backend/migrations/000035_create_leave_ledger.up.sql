CREATE TABLE leave_ledger (
    id UUID PRIMARY KEY,
    tenant_id UUID NOT NULL REFERENCES tenants(id) ON DELETE CASCADE,
    employee_profile_id UUID NOT NULL REFERENCES employee_profiles(id) ON DELETE CASCADE,
    leave_type_id UUID NOT NULL REFERENCES leave_types(id) ON DELETE CASCADE,
    year INT NOT NULL,
    kind VARCHAR(20) NOT NULL CHECK (kind IN ('carry_forward', 'accrual', 'adjustment', 'reserve', 'release', 'consume', 'reversal')),
    days NUMERIC(7,2) NOT NULL CHECK (days <> 0),
    leave_request_id UUID REFERENCES leave_requests(id),
    actor_user_id UUID,
    note VARCHAR(500),
    created_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP
);

-- LV-013/LV-016: append-only; balances are its projection.
CREATE INDEX idx_leave_ledger_balance ON leave_ledger (tenant_id, employee_profile_id, leave_type_id, year);
