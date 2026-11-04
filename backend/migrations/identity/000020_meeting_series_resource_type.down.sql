DELETE FROM resource_role_assignments WHERE resource_type = 'meeting_series';
DELETE FROM role_permissions WHERE role_id IN (SELECT id FROM roles WHERE applies_to = 'meeting_series');
DELETE FROM user_roles WHERE role_id IN (SELECT id FROM roles WHERE applies_to = 'meeting_series');
DELETE FROM roles WHERE applies_to = 'meeting_series';
DELETE FROM role_permissions WHERE permission_id IN (SELECT id FROM permissions WHERE applies_to = 'meeting_series');
DELETE FROM permissions WHERE applies_to = 'meeting_series';
ALTER TABLE resource_role_assignments DROP CONSTRAINT resource_role_assignments_resource_type_check,
    ADD CONSTRAINT resource_role_assignments_resource_type_check CHECK (resource_type IN ('project'));
ALTER TABLE roles DROP CONSTRAINT chk_roles_applies_to,
    ADD CONSTRAINT chk_roles_applies_to CHECK (applies_to IN ('tenant', 'project'));
ALTER TABLE permissions DROP CONSTRAINT chk_permissions_applies_to,
    ADD CONSTRAINT chk_permissions_applies_to CHECK (applies_to IN ('tenant', 'project'));
