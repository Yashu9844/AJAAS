"""M2 platform routes (group membership rules). Merged into the swagger by scripts/gen_projects_swagger.py."""

STR = dict(type='string')
UUID = dict(type='string', format='uuid')

ROUTES = [
    ('get', '/projects/{id}/group-rules', 'List the membership rules of a project (project.manage_members)', 'Project groups', [], None, '200', ['403', '404']),
    ('post', '/projects/{id}/group-rules', 'Add a department, team or reports-of rule; members are expanded at once (project.manage_members)', 'Project groups', [], 'GroupRuleInput', '201', ['400', '403', '404', '409']),
    ('delete', '/projects/{id}/group-rules/{rule_id}', 'Remove a rule; members only it granted leave the project (project.manage_members)', 'Project groups', [], None, '200', ['403', '404']),
    ('post', '/projects/{id}/group-rules/refresh', 'Re-resolve the rules against current HR data (project.manage_members)', 'Project groups', [], None, '200', ['403', '404']),
]

SCHEMAS = {
    'GroupRuleInput': dict(required=['kind', 'ref_id', 'role'], props={'kind': dict(type='string', enum=['department', 'team', 'reports_of']),
        'ref_id': UUID, 'role': dict(type='string', enum=['manager', 'member', 'viewer'])}),
}
