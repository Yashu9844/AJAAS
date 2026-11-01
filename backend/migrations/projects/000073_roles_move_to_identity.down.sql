CREATE TABLE project_roles (
    id UUID PRIMARY KEY,
    tenant_id UUID NOT NULL,
    name VARCHAR(60) NOT NULL,
    description VARCHAR(300) NOT NULL DEFAULT '',
    permissions JSONB NOT NULL DEFAULT '[]',
    created_by UUID NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE UNIQUE INDEX uq_project_roles_name ON project_roles (tenant_id, lower(name));
ALTER TABLE project_members ADD COLUMN role_key VARCHAR(20) NOT NULL DEFAULT 'viewer';
ALTER TABLE project_members ADD COLUMN custom_role_id UUID REFERENCES project_roles (id);
