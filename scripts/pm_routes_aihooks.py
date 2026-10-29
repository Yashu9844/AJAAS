"""M2 AI hook routes (read-only seams for a separate AI module). Merged into the swagger by scripts/gen_projects_swagger.py."""

STR = dict(type='string')


def Q(name, typ='string', **kw):
    return dict(name=name, **{'in': 'query'}, schema=dict(type=typ, **kw))


ROUTES = [
    ('get', '/ai-hooks/tools', 'Machine-readable catalogue of the actions M2 offers to an AI; with project_id only the tools the caller may use there', 'AI hooks', [Q('project_id', format='uuid')], None, '200', ['404']),
    ('get', '/ai-hooks/readable-projects', 'Ids of the projects the caller can read, for filtering retrieval results to what this user may see', 'AI hooks', [], None, '200', []),
    ('get', '/ai-hooks/context/projects/{id}', 'Compact project context pack for a model: columns, counts, planning, setup and the caller permissions (project.read)', 'AI hooks', [], None, '200', ['403', '404']),
    ('get', '/ai-hooks/context/tasks/{id}', 'Compact task context pack: task, subtasks, relations, latest comments, activity and custom fields; id or ticket key (task.read)', 'AI hooks', [], None, '200', ['403', '404']),
    ('get', '/ai-hooks/documents', 'Incremental text feed (tasks or comments, with tombstones) of one project for indexing (task.read)', 'AI hooks',
     [Q('project_id', format='uuid'), Q('kind', enum=['task', 'comment']), Q('cursor'), Q('limit', 'integer', minimum=1, maximum=100)], None, '200', ['400', '403', '404']),
]

SCHEMAS = {}
