CREATE TABLE project_roles (
    id UUID PRIMARY KEY,
    tenant_id UUID NOT NULL,
    name VARCHAR(60) NOT NULL,
    description VARCHAR(300) NOT NULL DEFAULT '',
    permissions JSONB NOT NULL DEFAULT '[]',
    created_by UUID NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP
);
CREATE UNIQUE INDEX uq_project_roles_name ON project_roles (tenant_id, lower(name));

CREATE TABLE project_members (
    project_id UUID NOT NULL REFERENCES projects (id) ON DELETE CASCADE,
    user_id UUID NOT NULL,
    tenant_id UUID NOT NULL,
    role_key VARCHAR(20) CHECK (role_key IN ('owner', 'manager', 'member', 'viewer')),
    custom_role_id UUID REFERENCES project_roles (id),
    active BOOLEAN NOT NULL DEFAULT TRUE,
    added_by UUID NOT NULL,
    added_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP,
    PRIMARY KEY (project_id, user_id),
    CONSTRAINT project_members_one_role CHECK ((role_key IS NULL) <> (custom_role_id IS NULL))
);
CREATE INDEX idx_project_members_user ON project_members (tenant_id, user_id) WHERE active;
