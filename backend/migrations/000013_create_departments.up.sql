CREATE TABLE departments (
    id UUID PRIMARY KEY,
    tenant_id UUID NOT NULL REFERENCES tenants(id) ON DELETE CASCADE,
    name VARCHAR(100) NOT NULL,
    code VARCHAR(32),
    description VARCHAR(500),
    parent_department_id UUID REFERENCES departments(id) ON DELETE RESTRICT,
    status VARCHAR(20) NOT NULL DEFAULT 'active',
    created_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP,
    deleted_at TIMESTAMPTZ
);

CREATE UNIQUE INDEX uq_departments_tenant_name ON departments (tenant_id, lower(name)) WHERE deleted_at IS NULL;
CREATE UNIQUE INDEX uq_departments_tenant_code ON departments (tenant_id, code) WHERE deleted_at IS NULL AND code IS NOT NULL;
CREATE INDEX idx_departments_tenant_id ON departments (tenant_id);
CREATE INDEX idx_departments_parent_id ON departments (parent_department_id);
CREATE INDEX idx_departments_status ON departments (status);
CREATE INDEX idx_departments_deleted_at ON departments (deleted_at);
