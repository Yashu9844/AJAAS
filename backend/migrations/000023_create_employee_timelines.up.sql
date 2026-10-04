CREATE TABLE IF NOT EXISTS employee_timelines (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    tenant_id UUID NOT NULL REFERENCES tenants(id) ON DELETE CASCADE,
    employee_profile_id UUID NOT NULL REFERENCES employee_profiles(id) ON DELETE CASCADE,
    event_type VARCHAR(50) NOT NULL,
    effective_date DATE NOT NULL,
    notes TEXT,
    metadata JSONB DEFAULT '{}'::jsonb,
    created_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP,
    deleted_at TIMESTAMPTZ
);

CREATE INDEX IF NOT EXISTS idx_emp_timelines_tenant_profile ON employee_timelines(tenant_id, employee_profile_id);
CREATE INDEX IF NOT EXISTS idx_emp_timelines_effective ON employee_timelines(tenant_id, effective_date);
