CREATE TABLE teams (
    id UUID PRIMARY KEY,
    tenant_id UUID NOT NULL REFERENCES tenants(id) ON DELETE CASCADE,
    department_id UUID NOT NULL REFERENCES departments(id) ON DELETE RESTRICT,
    name VARCHAR(100) NOT NULL,
    code VARCHAR(32),
    description VARCHAR(500),
    lead_user_id UUID REFERENCES users(id) ON DELETE SET NULL,
    status VARCHAR(20) NOT NULL DEFAULT 'active',
    created_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP,
    deleted_at TIMESTAMPTZ
);

CREATE UNIQUE INDEX uq_teams_dept_name ON teams (department_id, lower(name)) WHERE deleted_at IS NULL;
CREATE UNIQUE INDEX uq_teams_tenant_code ON teams (tenant_id, code) WHERE deleted_at IS NULL AND code IS NOT NULL;
CREATE INDEX idx_teams_tenant_id ON teams (tenant_id);
CREATE INDEX idx_teams_department_id ON teams (department_id);
CREATE INDEX idx_teams_lead_user_id ON teams (lead_user_id);
CREATE INDEX idx_teams_status ON teams (status);
CREATE INDEX idx_teams_deleted_at ON teams (deleted_at);
