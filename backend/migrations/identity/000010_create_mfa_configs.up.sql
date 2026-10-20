CREATE TABLE mfa_configs (
    id UUID PRIMARY KEY,
    user_id UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    tenant_id UUID NOT NULL REFERENCES tenants(id) ON DELETE CASCADE,
    type VARCHAR(20) NOT NULL,
    secret VARCHAR(500) NOT NULL,
    is_enabled BOOLEAN NOT NULL DEFAULT false,
    verified_at TIMESTAMPTZ,
    created_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP
);

CREATE UNIQUE INDEX uq_mfa_configs_user_type ON mfa_configs (user_id, type);
CREATE INDEX idx_mfa_configs_user_id ON mfa_configs (user_id);
CREATE INDEX idx_mfa_configs_tenant_id ON mfa_configs (tenant_id);
