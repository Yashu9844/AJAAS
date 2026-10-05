CREATE TABLE designations (
    id UUID PRIMARY KEY,
    tenant_id UUID NOT NULL REFERENCES tenants(id) ON DELETE CASCADE,
    title VARCHAR(100) NOT NULL,
    code VARCHAR(32),
    level INT,
    description VARCHAR(500),
    status VARCHAR(20) NOT NULL DEFAULT 'active',
    created_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP,
    deleted_at TIMESTAMPTZ
);

CREATE UNIQUE INDEX uq_designations_tenant_title ON designations (tenant_id, lower(title)) WHERE deleted_at IS NULL;
CREATE UNIQUE INDEX uq_designations_tenant_code ON designations (tenant_id, code) WHERE deleted_at IS NULL AND code IS NOT NULL;
CREATE INDEX idx_designations_tenant_id ON designations (tenant_id);
CREATE INDEX idx_designations_level ON designations (level);
CREATE INDEX idx_designations_status ON designations (status);
CREATE INDEX idx_designations_deleted_at ON designations (deleted_at);
