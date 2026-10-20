-- M0 access model (docs/ARCHITECTURE.md §17): scoped grants, role inheritance, predefined and automatic roles,
-- per-user grants and blocks, and a per-tenant access version used to invalidate cached effective access.

ALTER TABLE permissions ADD COLUMN module VARCHAR(50) NOT NULL DEFAULT '';
ALTER TABLE permissions ADD COLUMN is_sensitive BOOLEAN NOT NULL DEFAULT false;

ALTER TABLE roles ADD COLUMN parent_role_id UUID REFERENCES roles(id) ON DELETE SET NULL;
ALTER TABLE roles ADD COLUMN template_key VARCHAR(50);
ALTER TABLE roles ADD COLUMN is_automatic BOOLEAN NOT NULL DEFAULT false;
ALTER TABLE roles ADD COLUMN template_managed BOOLEAN NOT NULL DEFAULT false;
CREATE INDEX idx_roles_parent_role_id ON roles (parent_role_id);
CREATE UNIQUE INDEX uq_roles_tenant_template_key ON roles (tenant_id, template_key)
    WHERE template_key IS NOT NULL AND deleted_at IS NULL;

ALTER TABLE role_permissions ADD COLUMN scope VARCHAR(20) NOT NULL DEFAULT 'tenant'
    CONSTRAINT chk_role_permissions_scope CHECK (scope IN ('own', 'direct_reports', 'all_reports', 'department', 'tenant'));

ALTER TABLE user_roles ADD COLUMN source VARCHAR(20) NOT NULL DEFAULT 'manual'
    CONSTRAINT chk_user_roles_source CHECK (source IN ('manual', 'automatic'));

ALTER TABLE tenants ADD COLUMN access_version BIGINT NOT NULL DEFAULT 0;

CREATE TABLE user_permission_grants (
    id UUID PRIMARY KEY,
    tenant_id UUID NOT NULL REFERENCES tenants(id) ON DELETE CASCADE,
    user_id UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    permission_id UUID NOT NULL REFERENCES permissions(id) ON DELETE CASCADE,
    scope VARCHAR(20) NOT NULL
        CONSTRAINT chk_user_permission_grants_scope CHECK (scope IN ('own', 'direct_reports', 'all_reports', 'department', 'tenant')),
    expires_at TIMESTAMPTZ,
    reason VARCHAR(500),
    granted_by UUID,
    created_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP
);
CREATE UNIQUE INDEX uq_user_permission_grants ON user_permission_grants (tenant_id, user_id, permission_id);

CREATE TABLE user_permission_blocks (
    id UUID PRIMARY KEY,
    tenant_id UUID NOT NULL REFERENCES tenants(id) ON DELETE CASCADE,
    user_id UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    permission_id UUID NOT NULL REFERENCES permissions(id) ON DELETE CASCADE,
    reason VARCHAR(500),
    created_by UUID,
    created_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP
);
CREATE UNIQUE INDEX uq_user_permission_blocks ON user_permission_blocks (tenant_id, user_id, permission_id);
