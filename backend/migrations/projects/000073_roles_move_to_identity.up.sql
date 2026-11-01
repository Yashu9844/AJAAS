-- Project roles now live in M0 identity (docs/modules/m0-identity-access/resource-roles.md, decision DPJ-49). Membership stays here;
-- the role of a member is an M0 assignment on the project. Module 2 was never released, so no data is carried over.
ALTER TABLE project_members DROP CONSTRAINT IF EXISTS project_members_group_not_owner;
ALTER TABLE project_members DROP CONSTRAINT IF EXISTS project_members_one_role;
ALTER TABLE project_members DROP COLUMN role_key;
ALTER TABLE project_members DROP COLUMN custom_role_id;
DROP TABLE project_roles;
