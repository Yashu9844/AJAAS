-- Re-baselined into schema hr (DHR-09): no foreign keys into identity tables; tenant and user ids are
-- checked through identityport.
CREATE TABLE IF NOT EXISTS employee_contacts (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    tenant_id UUID NOT NULL,
    employee_profile_id UUID NOT NULL REFERENCES employee_profiles(id) ON DELETE CASCADE,
    personal_email VARCHAR(255),
    work_phone VARCHAR(50),
    personal_phone VARCHAR(50),
    current_address TEXT,
    permanent_address TEXT,
    emergency_contacts JSONB DEFAULT '[]'::jsonb,
    created_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP,
    deleted_at TIMESTAMPTZ
);

CREATE UNIQUE INDEX IF NOT EXISTS idx_emp_contact_tenant_profile ON employee_contacts(tenant_id, employee_profile_id);
CREATE INDEX IF NOT EXISTS idx_emp_contact_deleted_at ON employee_contacts(deleted_at);
