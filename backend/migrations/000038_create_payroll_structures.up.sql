CREATE TABLE payroll_structures (
    id UUID PRIMARY KEY,
    tenant_id UUID NOT NULL REFERENCES tenants(id) ON DELETE CASCADE,
    name VARCHAR(100) NOT NULL,
    description VARCHAR(500),
    pf_enabled BOOLEAN NOT NULL DEFAULT TRUE,
    esi_enabled BOOLEAN NOT NULL DEFAULT TRUE,
    pt_enabled BOOLEAN NOT NULL DEFAULT TRUE,
    tds_enabled BOOLEAN NOT NULL DEFAULT TRUE,
    status VARCHAR(20) NOT NULL DEFAULT 'active' CHECK (status IN ('active', 'inactive')),
    created_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP,
    deleted_at TIMESTAMPTZ
);

CREATE UNIQUE INDEX uq_payroll_structures_tenant_name ON payroll_structures (tenant_id, lower(name)) WHERE deleted_at IS NULL;
CREATE INDEX idx_payroll_structures_tenant_id ON payroll_structures (tenant_id);
CREATE INDEX idx_payroll_structures_deleted_at ON payroll_structures (deleted_at);
