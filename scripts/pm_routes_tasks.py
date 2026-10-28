"""M2 task core routes and schemas (merged by scripts/gen_projects_swagger.py). Same tuple format as that file:
(method, path, summary, tag, query params, body schema, success status, error codes)."""

Q = lambda name, typ='string', **kw: dict(name=name, **{'in': 'query'}, schema=dict(type=typ, **kw))
CURSOR = [Q('cursor'), Q('limit', 'integer', minimum=1, maximum=100)]
T = 'Tasks'

ROUTES = [
    ('post', '/projects/{id}/tasks', 'Create a task with the next ticket number, appended to its column (task.create)', T, [], 'TaskInput', '201', ['400', '403', '404', '409']),
    ('get', '/projects/{id}/tasks', 'List tasks with filters, sort and keyset cursor (task.read)', T,
     [Q('column_id', format='uuid'), Q('category', enum=['backlog', 'todo', 'in_progress', 'done', 'canceled']),
      Q('priority', enum=['none', 'low', 'medium', 'high', 'urgent']), Q('assignee', description='user id or me'),
      Q('label_id', format='uuid'), Q('milestone_id', format='uuid'), Q('sprint_id', format='uuid'), Q('parent_id', format='uuid'),
      Q('due_from'), Q('due_to'), Q('overdue', 'boolean'), Q('blocked', 'boolean'), Q('top_level', 'boolean'), Q('archived', 'boolean'),
      Q('q'), Q('sort', enum=['rank', 'created', 'updated', 'due', 'priority', 'number']), Q('dir', enum=['asc', 'desc'])] + CURSOR,
     None, '200', ['400', '403', '404']),
    ('post', '/projects/{id}/tasks/bulk', 'Update up to 100 tasks; the response lists each task outcome (task.update)', T, [], 'TaskBulkInput', '200', ['400', '403', '404']),
    ('get', '/projects/{id}/activity', 'Project activity, newest first, cursor by sequence (task.read)', T, [Q('order', enum=['asc', 'desc'])] + CURSOR, None, '200', ['400', '404']),
    ('get', '/tasks/{id}', 'One task with ticket id, derived flags, assignees and labels', T, [], None, '200', ['400', '404']),
    ('patch', '/tasks/{id}', 'Edit a task; version (body or If-Match) is required (task.update)', T, [], 'TaskUpdateInput', '200', ['400', '403', '404', '409']),
    ('delete', '/tasks/{id}', 'Soft-delete a task and its subtasks (task.delete)', T, [], None, '200', ['403', '404']),
    ('get', '/tickets/{key}', 'Resolve a ticket id such as KEY-12; moved tasks keep their old ids', T, [], None, '200', ['400', '404']),
    ('post', '/tasks/{id}/move', 'Move to a column and position; WIP and done rules apply (task.update)', T, [], 'TaskMoveInput', '200', ['400', '403', '404', '409']),
    ('post', '/tasks/{id}/reorder', 'Place next to a neighbour with one before_id or after_id (task.update)', T, [], 'TaskMoveInput', '200', ['400', '403', '404', '409']),
    ('post', '/tasks/{id}/move-project', 'Move to another project: new number, alias for the old ticket id (task.update, task.create)', T, [], 'TaskMoveProjectInput', '200', ['400', '403', '404', '409']),
    ('post', '/tasks/{id}/duplicate', 'Copy a task, optionally with labels and assignees (task.create)', T, [], 'TaskDuplicateInput', '201', ['400', '403', '404', '409']),
    ('post', '/tasks/{id}/archive', 'Archive a task (task.update)', T, [], None, '200', ['403', '404', '409']),
    ('post', '/tasks/{id}/restore', 'Restore an archived task (task.update)', T, [], None, '200', ['403', '404', '409']),
    ('post', '/tasks/{id}/assignees', 'Assign project members (task.assign)', T, [], 'TaskAssigneesInput', '200', ['400', '403', '404', '409']),
    ('delete', '/tasks/{id}/assignees/{user_id}', 'Remove an assignee (task.assign)', T, [], None, '200', ['403', '404', '409']),
    ('post', '/tasks/{id}/labels', 'Add labels of the project or tenant (task.update)', T, [], 'TaskLabelsInput', '200', ['400', '403', '404', '409']),
    ('delete', '/tasks/{id}/labels/{label_id}', 'Remove a label (task.update)', T, [], None, '200', ['403', '404', '409']),
    ('get', '/tasks/{id}/subtasks', 'List direct subtasks with progress', T, CURSOR, None, '200', ['400', '404']),
    ('get', '/tasks/{id}/relations', 'List relations; ends in unreadable projects are hidden', T, [], None, '200', ['404']),
    ('post', '/tasks/{id}/relations', 'Relate to another task; blocks may not form a cycle (task.update)', T, [], 'TaskRelationInput', '201', ['400', '403', '404', '409']),
    ('delete', '/tasks/{id}/relations/{relation_id}', 'Remove a relation (task.update)', T, [], None, '200', ['403', '404']),
    ('get', '/tasks/{id}/activity', 'Task history, newest first, cursor by sequence', T, [Q('order', enum=['asc', 'desc'])] + CURSOR, None, '200', ['400', '404']),
]

STR, INT, BOOL = dict(type='string'), dict(type='integer'), dict(type='boolean')
DATE, TIME, UUID = dict(type='string', format='date'), dict(type='string', format='date-time'), dict(type='string', format='uuid')
UUIDS = dict(type='array', items=UUID)
PRIORITY = dict(type='string', enum=['none', 'low', 'medium', 'high', 'urgent'])

SCHEMAS = {
    'TaskInput': dict(required=['title'], props={'title': STR, 'description': STR, 'column_id': UUID, 'priority': PRIORITY, 'start_on': DATE, 'due_at': TIME,
        'estimate_minutes': INT, 'milestone_id': UUID, 'sprint_id': UUID, 'parent_task_id': UUID, 'assignee_ids': UUIDS, 'label_ids': UUIDS}),
    'TaskUpdateInput': dict(required=['version'], props={'version': INT, 'title': STR, 'description': STR, 'priority': PRIORITY, 'start_on': dict(DATE, nullable=True),
        'due_at': dict(TIME, nullable=True), 'estimate_minutes': dict(INT, nullable=True), 'milestone_id': dict(UUID, nullable=True),
        'sprint_id': dict(UUID, nullable=True), 'parent_task_id': dict(UUID, nullable=True)}),
    'TaskMoveInput': dict(required=[], props={'column_id': UUID, 'before_id': UUID, 'after_id': UUID, 'version': INT, 'force': BOOL}),
    'TaskMoveProjectInput': dict(required=['project_id'], props={'project_id': UUID, 'column_id': UUID, 'version': INT}),
    'TaskDuplicateInput': dict(required=[], props={'copy_assignees': BOOL, 'copy_labels': BOOL}),
    'TaskAssigneesInput': dict(required=['user_ids'], props={'user_ids': UUIDS}),
    'TaskLabelsInput': dict(required=['label_ids'], props={'label_ids': UUIDS}),
    'TaskRelationInput': dict(required=['target_task_id', 'type'], props={'target_task_id': UUID, 'type': dict(type='string', enum=['blocks', 'related', 'duplicates'])}),
    'TaskBulkInput': dict(required=['task_ids'], props={'task_ids': UUIDS, 'column_id': UUID, 'priority': PRIORITY, 'assignees_add': UUIDS, 'assignees_remove': UUIDS,
        'labels_add': UUIDS, 'labels_remove': UUIDS, 'milestone_id': dict(UUID, nullable=True), 'sprint_id': dict(UUID, nullable=True), 'archive': BOOL, 'force': BOOL}),
}
