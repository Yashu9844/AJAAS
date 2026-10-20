DROP INDEX IF EXISTS idx_role_permissions_tenant_id;
DROP INDEX IF EXISTS idx_role_permissions_permission_id;
DROP INDEX IF EXISTS idx_role_permissions_role_id;
DROP INDEX IF EXISTS uq_role_permissions_role_perm_tenant;

DROP TABLE IF EXISTS role_permissions;
