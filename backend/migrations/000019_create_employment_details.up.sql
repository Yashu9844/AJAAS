CREATE TABLE IF NOT EXISTS employment_details (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    tenant_id UUID NOT NULL REFERENCES tenants(id) ON DELETE CASCADE,
    employee_profile_id UUID NOT NULL REFERENCES employee_profiles(id) ON DELETE CASCADE,
    employment_type VARCHAR(30) NOT NULL DEFAULT 'full_time',
    joining_date DATE NOT NULL,
    probation_end_date DATE,
    confirmation_date DATE,
    notice_period_days INT NOT NULL DEFAULT 30,
    resignation_date DATE,
    exit_date DATE,
    exit_reason TEXT,
    created_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP,
    deleted_at TIMESTAMPTZ
);

CREATE UNIQUE INDEX IF NOT EXISTS idx_emp_detail_tenant_profile ON employment_details(tenant_id, employee_profile_id);
CREATE INDEX IF NOT EXISTS idx_emp_detail_deleted_at ON employment_details(deleted_at);
