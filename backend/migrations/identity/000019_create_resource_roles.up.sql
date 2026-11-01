-- Resource-scoped roles (docs/modules/m0-identity-access/resource-roles.md): a role can be assigned to a user for one resource
-- (today: a project). Permissions and roles say what they apply to.
ALTER TABLE permissions ADD COLUMN applies_to VARCHAR(20) NOT NULL DEFAULT 'tenant'
    CONSTRAINT chk_permissions_applies_to CHECK (applies_to IN ('tenant', 'project'));
ALTER TABLE roles ADD COLUMN applies_to VARCHAR(20) NOT NULL DEFAULT 'tenant'
    CONSTRAINT chk_roles_applies_to CHECK (applies_to IN ('tenant', 'project'));
CREATE INDEX idx_roles_applies_to ON roles (tenant_id, applies_to);

CREATE TABLE resource_role_assignments (
    id UUID PRIMARY KEY,
    tenant_id UUID NOT NULL REFERENCES tenants(id) ON DELETE CASCADE,
    user_id UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    role_id UUID NOT NULL REFERENCES roles(id) ON DELETE RESTRICT,
    resource_type VARCHAR(20) NOT NULL CHECK (resource_type IN ('project')),
    -- no foreign key: the resource belongs to another service schema
    resource_id UUID NOT NULL,
    source VARCHAR(20) NOT NULL DEFAULT 'manual' CHECK (source IN ('manual', 'group', 'automatic')),
    assigned_by UUID,
    created_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP
);
CREATE UNIQUE INDEX uq_resource_role_user ON resource_role_assignments (tenant_id, user_id, resource_type, resource_id);
CREATE INDEX idx_resource_role_resource ON resource_role_assignments (tenant_id, resource_type, resource_id);
CREATE INDEX idx_resource_role_role ON resource_role_assignments (role_id);
