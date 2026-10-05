CREATE TABLE IF NOT EXISTS employee_profiles (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    tenant_id UUID NOT NULL REFERENCES tenants(id) ON DELETE CASCADE,
    user_id UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    employee_code VARCHAR(32) NOT NULL,
    first_name VARCHAR(100) NOT NULL,
    last_name VARCHAR(100) NOT NULL,
    display_name VARCHAR(200),
    gender VARCHAR(20),
    date_of_birth DATE,
    marital_status VARCHAR(30),
    blood_group VARCHAR(10),
    avatar_url TEXT,
    status VARCHAR(30) NOT NULL DEFAULT 'active',
    created_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP,
    deleted_at TIMESTAMPTZ
);

CREATE UNIQUE INDEX IF NOT EXISTS idx_emp_profile_tenant_user ON employee_profiles(tenant_id, user_id);
CREATE UNIQUE INDEX IF NOT EXISTS idx_emp_profile_tenant_code ON employee_profiles(tenant_id, LOWER(employee_code));
CREATE INDEX IF NOT EXISTS idx_emp_profile_status ON employee_profiles(tenant_id, status);
CREATE INDEX IF NOT EXISTS idx_emp_profile_deleted_at ON employee_profiles(deleted_at);
