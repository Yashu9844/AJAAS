"""Generate the M2 Projects & Tasks section of backend/api/swagger.yaml from the route table below.

Usage: python scripts/gen_projects_swagger.py <repo>
The generated block sits between the BEGIN/END markers inside `paths:` and `components.schemas`; everything else in the
file is untouched. TestSwaggerMatchesRoutes keeps the table and the router in step. Re-run whenever routes change.
"""
import re
import sys

Q = lambda name, typ='string', **kw: dict(name=name, **{'in': 'query'}, schema=dict(type=typ, **kw))
CURSOR = [Q('cursor'), Q('limit', 'integer', minimum=1, maximum=100)]
UUID_ID = {'id': 'id'}

# (method, path, summary, tag, query params, body schema, success status, error codes[, 'public'])
# A 9th element 'public' documents a route without a bearer token (security: [], text/calendar response).
ROUTES = [
    # --- projects
    ('post', '/projects', 'Create a project; the caller becomes its owner (projects:create)', 'Projects', [], 'ProjectInput', '201', ['400', '403', '409']),
    ('get', '/projects', 'List the projects the caller can read, most recently active first', 'Projects',
     [Q('status', enum=['active', 'archived']), Q('q'), Q('mine', 'boolean'), Q('favorite', 'boolean')] + CURSOR, None, '200', ['400']),
    ('get', '/projects/{id}', 'One project with the caller favourite flag and role', 'Projects', [], None, '200', ['404']),
    ('patch', '/projects/{id}', 'Edit a project; send version (or If-Match) for optimistic concurrency (project.update)', 'Projects', [], 'ProjectInput', '200', ['400', '403', '404', '409']),
    ('post', '/projects/{id}/archive', 'Archive a project, freezing its tasks (project.archive)', 'Projects', [], None, '200', ['403', '404', '409']),
    ('post', '/projects/{id}/restore', 'Restore an archived project (project.archive)', 'Projects', [], None, '200', ['403', '404', '409']),
    ('delete', '/projects/{id}', 'Soft-delete an archived project (projects:manage_all)', 'Projects', [], None, '200', ['403', '404', '409']),
    ('put', '/projects/{id}/favorite', 'Star the project for the caller', 'Projects', [], None, '200', ['404']),
    ('delete', '/projects/{id}/favorite', 'Unstar the project for the caller', 'Projects', [], None, '200', ['404']),
    # --- members and roles
    ('get', '/projects/{id}/members', 'List active members with role names', 'Projects', [], None, '200', ['404']),
    ('post', '/projects/{id}/members', 'Add a member with a built-in or custom role (project.manage_members)', 'Projects', [], 'MemberInput', '201', ['400', '403', '404', '409']),
    ('patch', '/projects/{id}/members/{user_id}', 'Change a member role; the last owner stays (project.manage_members)', 'Projects', [], 'MemberInput', '200', ['400', '403', '404', '409']),
    ('delete', '/projects/{id}/members/{user_id}', 'Remove a member; anyone may remove themselves', 'Projects', [], None, '200', ['403', '404', '409']),
    ('get', '/project-permissions', 'Project permissions and the built-in roles', 'Project roles', [], None, '200', []),
    ('get', '/project-roles', 'List the tenant custom project roles', 'Project roles', [], None, '200', []),
    ('post', '/project-roles', 'Create a custom role (project_roles:manage)', 'Project roles', [], 'ProjectRoleInput', '201', ['400', '403', '409']),
    ('patch', '/project-roles/{id}', 'Edit a custom role (project_roles:manage)', 'Project roles', [], 'ProjectRoleInput', '200', ['400', '403', '404', '409']),
    ('delete', '/project-roles/{id}', 'Delete an unused custom role (project_roles:manage)', 'Project roles', [], None, '200', ['403', '404', '409']),
    # --- columns, labels, milestones
    ('get', '/projects/{id}/columns', 'List board columns with open task counts', 'Project setup', [], None, '200', ['404']),
    ('post', '/projects/{id}/columns', 'Add a column (column.manage)', 'Project setup', [], 'ColumnInput', '201', ['400', '403', '404']),
    ('put', '/projects/{id}/columns/order', 'Reorder all columns (column.manage)', 'Project setup', [], 'ColumnOrderInput', '200', ['400', '403', '404']),
    ('patch', '/projects/{id}/columns/{column_id}', 'Edit a column; keeps one todo and one done column (column.manage)', 'Project setup', [], 'ColumnInput', '200', ['400', '403', '404', '409']),
    ('delete', '/projects/{id}/columns/{column_id}', 'Delete a column, moving its tasks to move_to (column.manage)', 'Project setup', [Q('move_to', format='uuid')], None, '200', ['400', '403', '404', '409']),
    ('get', '/projects/{id}/labels', 'Project and tenant-wide labels usable in the project', 'Project setup', [], None, '200', ['404']),
    ('post', '/projects/{id}/labels', 'Create a project label (label.manage)', 'Project setup', [], 'LabelInput', '201', ['400', '403', '404', '409']),
    ('patch', '/projects/{id}/labels/{label_id}', 'Edit a project label (label.manage)', 'Project setup', [], 'LabelInput', '200', ['400', '403', '404', '409']),
    ('delete', '/projects/{id}/labels/{label_id}', 'Delete a project label from every task (label.manage)', 'Project setup', [], None, '200', ['403', '404']),
    ('get', '/project-labels', 'List tenant-wide labels', 'Project setup', [], None, '200', []),
    ('post', '/project-labels', 'Create a tenant-wide label (projects:manage_all)', 'Project setup', [], 'LabelInput', '201', ['400', '403', '409']),
    ('patch', '/project-labels/{id}', 'Edit a tenant-wide label (projects:manage_all)', 'Project setup', [], 'LabelInput', '200', ['400', '403', '404', '409']),
    ('delete', '/project-labels/{id}', 'Delete a tenant-wide label (projects:manage_all)', 'Project setup', [], None, '200', ['403', '404']),
    ('get', '/projects/{id}/milestones', 'List milestones with progress', 'Project setup', [], None, '200', ['404']),
    ('post', '/projects/{id}/milestones', 'Create a milestone (milestone.manage)', 'Project setup', [], 'MilestoneInput', '201', ['400', '403', '404', '409']),
    ('patch', '/projects/{id}/milestones/{milestone_id}', 'Edit a milestone (milestone.manage)', 'Project setup', [], 'MilestoneInput', '200', ['400', '403', '404', '409']),
    ('delete', '/projects/{id}/milestones/{milestone_id}', 'Delete a milestone; its tasks stay (milestone.manage)', 'Project setup', [], None, '200', ['403', '404']),
]

STR, INT, BOOL, DATE = dict(type='string'), dict(type='integer'), dict(type='boolean'), dict(type='string', format='date')
UUID = dict(type='string', format='uuid')
COLOR = dict(type='string', example='#3B82F6')
SCHEMAS = {
    'ProjectInput': dict(required=['key', 'name'], props={'key': STR, 'name': STR, 'description': STR, 'visibility': dict(type='string', enum=['private', 'tenant']),
        'mode': dict(type='string', enum=['kanban', 'scrum']), 'start_on': DATE, 'due_on': DATE, 'icon': STR, 'color': COLOR,
        'time_tracking_enabled': BOOL, 'version': INT}),
    'MemberInput': dict(required=[], props={'user_id': UUID, 'role': dict(type='string', enum=['owner', 'manager', 'member', 'viewer']), 'custom_role_id': UUID}),
    'ProjectRoleInput': dict(required=['name', 'permissions'], props={'name': STR, 'description': STR, 'permissions': dict(type='array', items=STR)}),
    'ColumnInput': dict(required=[], props={'name': STR, 'category': dict(type='string', enum=['backlog', 'todo', 'in_progress', 'done', 'canceled']),
        'color': COLOR, 'wip_limit': INT, 'clear_wip_limit': BOOL, 'wip_hard': BOOL, 'is_default': BOOL}),
    'ColumnOrderInput': dict(required=['column_ids'], props={'column_ids': dict(type='array', items=UUID)}),
    'LabelInput': dict(required=['name'], props={'name': STR, 'color': COLOR}),
    'MilestoneInput': dict(required=[], props={'name': STR, 'due_on': DATE, 'status': dict(type='string', enum=['open', 'reached'])}),
}
def load_context_tables():
    """Merge ROUTES and SCHEMAS from scripts/pm_routes_<context>.py (one file per M2 context, same tuple format)."""
    import glob
    import importlib.util
    import os
    for f in sorted(glob.glob(os.path.join(os.path.dirname(os.path.abspath(__file__)), 'pm_routes_*.py'))):
        spec = importlib.util.spec_from_file_location(os.path.basename(f)[:-3], f)
        mod = importlib.util.module_from_spec(spec)
        spec.loader.exec_module(mod)
        ROUTES.extend(mod.ROUTES)
        SCHEMAS.update(mod.SCHEMAS)


load_context_tables()
BEGIN, END = '# BEGIN M2 projects (generated by scripts/gen_projects_swagger.py)', '# END M2 projects'


def flow(v):
    """Render a value in YAML flow style, the file style."""
    if isinstance(v, dict):
        return '{' + ', '.join(f'{k}: {flow(x)}' for k, x in v.items()) + '}'
    if isinstance(v, list):
        return '[' + ', '.join(flow(x) for x in v) + ']'
    if isinstance(v, bool):
        return 'true' if v else 'false'
    if isinstance(v, (int, float)):
        return str(v)
    s = str(v)
    return s if re.fullmatch(r'[A-Za-z0-9_./-]+', s) and s not in ('true', 'false', 'null') else "'" + s.replace("'", "''") + "'"


def operation(method, path, summary, tag, query, body, status, errors, public=None):
    security = '[]' if public == 'public' else '[{BearerAuth: []}, {TenantSubdomain: []}]'
    out = [f'    {method}:', f"      summary: '{summary}'", f'      tags: [{tag}]', f'      security: {security}']
    params = [dict(name=n, **{'in': 'path'}, required=True, schema=UUID) for n in re.findall(r'\{(\w+)\}', path)] + query
    if params:
        out.append('      parameters: ' + flow(params))
    if body:
        out.append('      requestBody: ' + flow({'required': True, 'content': {'application/json': {'schema': {'$ref': f'#/components/schemas/{body}'}}}}))
    envelope = 'ProjectsListEnvelope' if method == 'get' and not re.search(r'\{[^}]+\}$', path) else 'ProjectsObjectEnvelope'
    out.append('      responses:')
    content = {'text/calendar': {'schema': {'type': 'string'}}} if public == 'public' else {'application/json': {'schema': {'$ref': f'#/components/schemas/{envelope}'}}}
    out.append(f"        '{status}': " + flow({'description': 'OK', 'content': content}))
    codes = {'400': 'VALIDATION_ERROR', '403': 'FORBIDDEN', '404': 'NOT_FOUND', '409': 'CONFLICT', '429': 'RATE_LIMITED'}
    out += [f"        '{c}': {{description: '{codes[c]}'}}" for c in errors]
    return out


def paths_block():
    by_path = {}
    for r in ROUTES:
        by_path.setdefault(r[1], []).append(r)
    lines = [BEGIN]
    for path, ops in by_path.items():
        lines.append(f'  {path}:')
        for r in ops:
            lines += operation(*[r[0]] + list(r[1:]))
    lines.append(END)
    return '\n'.join(lines)


def schemas_block():
    lines = [BEGIN.replace('BEGIN', 'BEGIN schemas')]
    lines.append("    ProjectsObjectEnvelope: {type: object, properties: {data: {type: object}}}")
    lines.append("    ProjectsListEnvelope: {type: object, properties: {data: {type: array, items: {type: object}}, meta: {type: object, properties: {limit: {type: integer}, next_cursor: {type: string, nullable: true}}}}}")
    for name, s in SCHEMAS.items():
        body = {'type': 'object', 'properties': s['props']}
        if s['required']:
            body['required'] = s['required']
        lines.append(f'    {name}: {flow(body)}')
    lines.append(END.replace('END', 'END schemas'))
    return '\n'.join(lines)


def splice(text, begin, end, block, anchor):
    pattern = re.compile(re.escape(begin) + r'.*?' + re.escape(end), re.S)
    if pattern.search(text):
        return pattern.sub(lambda _: block, text, count=1)
    if anchor is None:  # append at the end of the file
        return text.rstrip('\n') + '\n' + block + '\n'
    return text.replace(anchor, '\n' + block + anchor, 1)


def main(root):
    path = root + '/backend/api/swagger.yaml'
    raw = open(path, encoding='utf-8', newline='').read()
    nl = '\r\n' if '\r\n' in raw else '\n'
    text = raw.replace('\r\n', '\n')
    text = splice(text, BEGIN, END, paths_block(), '\ncomponents:')
    if '\ncomponents:' in text and BEGIN not in text.split('\ncomponents:')[0]:
        raise SystemExit('paths block was not placed before components')
    sb = schemas_block()
    text = splice(text, sb.split('\n')[0], sb.split('\n')[-1], sb, None)
    open(path, 'w', encoding='utf-8', newline='').write(text.replace('\n', nl))
    print('operations:', len(ROUTES))


if __name__ == '__main__':
    main(sys.argv[1])
