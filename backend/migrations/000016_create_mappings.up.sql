CREATE TABLE mappings (
    id UUID PRIMARY KEY,
    tenant_id UUID NOT NULL REFERENCES tenants(id) ON DELETE CASCADE,
    user_id UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    department_id UUID REFERENCES departments(id) ON DELETE RESTRICT,
    team_id UUID REFERENCES teams(id) ON DELETE RESTRICT,
    designation_id UUID REFERENCES designations(id) ON DELETE RESTRICT,
    is_primary BOOLEAN NOT NULL DEFAULT FALSE,
    manager_user_id UUID REFERENCES users(id) ON DELETE SET NULL,
    status VARCHAR(20) NOT NULL DEFAULT 'active',
    reason VARCHAR(500),
    created_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP,
    deleted_at TIMESTAMPTZ
);

CREATE UNIQUE INDEX uq_mappings_primary ON mappings (tenant_id, user_id) WHERE is_primary = TRUE AND deleted_at IS NULL;
CREATE INDEX idx_mappings_tenant_id ON mappings (tenant_id);
CREATE INDEX idx_mappings_tenant_user ON mappings (tenant_id, user_id);
CREATE INDEX idx_mappings_user_id ON mappings (user_id);
CREATE INDEX idx_mappings_department_id ON mappings (department_id);
CREATE INDEX idx_mappings_team_id ON mappings (team_id);
CREATE INDEX idx_mappings_designation_id ON mappings (designation_id);
CREATE INDEX idx_mappings_manager_user_id ON mappings (manager_user_id);
CREATE INDEX idx_mappings_primary ON mappings (is_primary) WHERE is_primary = TRUE;
CREATE INDEX idx_mappings_status ON mappings (status);
