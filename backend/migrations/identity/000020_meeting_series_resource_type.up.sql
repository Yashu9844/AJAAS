-- Roles can be assigned on an M3 meeting series (docs/modules/m3-meetings/decisions.md DMT-09).
ALTER TABLE permissions DROP CONSTRAINT chk_permissions_applies_to,
    ADD CONSTRAINT chk_permissions_applies_to CHECK (applies_to IN ('tenant', 'project', 'meeting_series'));
ALTER TABLE roles DROP CONSTRAINT chk_roles_applies_to,
    ADD CONSTRAINT chk_roles_applies_to CHECK (applies_to IN ('tenant', 'project', 'meeting_series'));
ALTER TABLE resource_role_assignments DROP CONSTRAINT resource_role_assignments_resource_type_check,
    ADD CONSTRAINT resource_role_assignments_resource_type_check CHECK (resource_type IN ('project', 'meeting_series'));
