CREATE TABLE IF NOT EXISTS employee_statutory (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    tenant_id UUID NOT NULL REFERENCES tenants(id) ON DELETE CASCADE,
    employee_profile_id UUID NOT NULL REFERENCES employee_profiles(id) ON DELETE CASCADE,
    tax_id VARCHAR(100),
    national_id VARCHAR(100),
    bank_name VARCHAR(100),
    bank_account_number VARCHAR(100),
    bank_routing_swift VARCHAR(100),
    created_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP,
    deleted_at TIMESTAMPTZ
);

CREATE UNIQUE INDEX IF NOT EXISTS idx_emp_statutory_tenant_profile ON employee_statutory(tenant_id, employee_profile_id);
CREATE INDEX IF NOT EXISTS idx_emp_statutory_deleted_at ON employee_statutory(deleted_at);
