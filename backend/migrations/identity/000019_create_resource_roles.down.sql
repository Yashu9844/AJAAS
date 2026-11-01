DROP TABLE IF EXISTS resource_role_assignments;
DROP INDEX IF EXISTS idx_roles_applies_to;
ALTER TABLE roles DROP COLUMN IF EXISTS applies_to;
ALTER TABLE permissions DROP COLUMN IF EXISTS applies_to;
