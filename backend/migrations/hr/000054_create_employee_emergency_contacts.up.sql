-- HR/Employee emergency contacts (FR-ER001).
CREATE TABLE employee_emergency_contacts (
    id UUID PRIMARY KEY,
    tenant_id UUID NOT NULL,
    employee_profile_id UUID NOT NULL REFERENCES employee_profiles(id) ON DELETE CASCADE,
    name VARCHAR(150) NOT NULL,
    relationship VARCHAR(50) NOT NULL,
    phone VARCHAR(30) NOT NULL,
    email VARCHAR(255),
    is_primary BOOLEAN NOT NULL DEFAULT FALSE,
    created_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP
);
CREATE INDEX idx_employee_emergency_contacts_profile ON employee_emergency_contacts (tenant_id, employee_profile_id);
CREATE UNIQUE INDEX uq_employee_emergency_contacts_primary ON employee_emergency_contacts (employee_profile_id) WHERE is_primary;
