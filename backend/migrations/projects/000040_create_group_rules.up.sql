-- Group membership rules (PR-04): all members of a department or team, or a manager direct reports, get a role in the
-- project. Expansions are materialised as project_members rows with source = 'group' and refreshed from HR facts.
CREATE TABLE project_group_rules (
    id UUID PRIMARY KEY,
    tenant_id UUID NOT NULL,
    project_id UUID NOT NULL REFERENCES projects (id) ON DELETE CASCADE,
    kind VARCHAR(16) NOT NULL CHECK (kind IN ('department', 'team', 'reports_of')),
    ref_id UUID NOT NULL,
    role_key VARCHAR(20) NOT NULL CHECK (role_key IN ('manager', 'member', 'viewer')),
    created_by UUID NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP,
    refreshed_at TIMESTAMPTZ
);
CREATE UNIQUE INDEX uq_project_group_rules ON project_group_rules (project_id, kind, ref_id);
CREATE INDEX idx_project_group_rules_tenant ON project_group_rules (tenant_id, kind, ref_id);

ALTER TABLE project_members ADD COLUMN source VARCHAR(10) NOT NULL DEFAULT 'direct' CHECK (source IN ('direct', 'group'));
ALTER TABLE project_members ADD CONSTRAINT project_members_group_not_owner CHECK (source = 'direct' OR role_key IS DISTINCT FROM 'owner');

-- A deactivated login keeps its assignments for history but no longer counts as an active assignee.
ALTER TABLE task_assignees ADD COLUMN active BOOLEAN NOT NULL DEFAULT TRUE;
