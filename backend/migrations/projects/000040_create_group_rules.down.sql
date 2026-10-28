ALTER TABLE task_assignees DROP COLUMN IF EXISTS active;
ALTER TABLE project_members DROP CONSTRAINT IF EXISTS project_members_group_not_owner;
ALTER TABLE project_members DROP COLUMN IF EXISTS source;
DROP TABLE IF EXISTS project_group_rules;
