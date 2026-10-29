"""M2 automation routes: workflow rules, recurring tasks and public project links (phase 4). Merged into the swagger by
scripts/gen_projects_swagger.py. A 9th tuple element 'public' marks a route without a bearer token."""

STR, INT, BOOL, DATE = dict(type='string'), dict(type='integer'), dict(type='boolean'), dict(type='string', format='date')
UUID = dict(type='string', format='uuid')
Q = lambda name, typ='string', **kw: dict(name=name, **{'in': 'query'}, schema=dict(type=typ, **kw))
CURSOR = [Q('cursor'), Q('limit', 'integer', minimum=1, maximum=100)]

# (method, path, summary, tag, query params, body schema, success status, error codes[, 'public'])
ROUTES = [
    # --- workflow rules (engine reads the project's own outbox; rules run as their creator)
    ('get', '/projects/{id}/rules', 'List the automation rules of a project (project.read)', 'Automation', [], None, '200', ['404']),
    ('post', '/projects/{id}/rules', 'Create a rule: trigger, all-of conditions, up to 10 actions; it runs as the caller; at most 50 per project (project.update)', 'Automation', [], 'RuleInput', '201', ['400', '403', '404', '409']),
    ('patch', '/projects/{id}/rules/{rule_id}', 'Edit, enable or disable a rule; any edit clears its failure flag; send version (or If-Match) (project.update)', 'Automation', [], 'RuleUpdate', '200', ['400', '403', '404', '409']),
    ('delete', '/projects/{id}/rules/{rule_id}', 'Delete a rule and its run history (project.update)', 'Automation', [], None, '200', ['403', '404']),
    ('get', '/projects/{id}/rules/{rule_id}/runs', 'What a rule did, newest first; status ok, skipped or failed (project.update)', 'Automation',
     [Q('status', enum=['ok', 'skipped', 'failed'])] + CURSOR, None, '200', ['400', '403', '404']),
    ('post', '/projects/{id}/rules/{rule_id}/test', 'Dry run against a task of the project: which conditions fail and which actions would run; changes nothing (project.update)', 'Automation', [], 'RuleTestInput', '200', ['400', '403', '404']),
    # --- recurring tasks
    ('get', '/projects/{id}/recurrences', 'List the recurring tasks of a project (project.read)', 'Recurrence', [], None, '200', ['404']),
    ('post', '/projects/{id}/recurrences', 'Schedule copies of a template task daily, weekly or monthly; at most 20 per project (project.update)', 'Recurrence', [], 'RecurrenceInput', '201', ['400', '403', '404', '409']),
    ('patch', '/projects/{id}/recurrences/{recurrence_id}', 'Edit, pause or re-plan a recurrence; send version (or If-Match) (project.update)', 'Recurrence', [], 'RecurrenceUpdate', '200', ['400', '403', '404', '409']),
    ('delete', '/projects/{id}/recurrences/{recurrence_id}', 'Delete a recurrence; tasks it already created stay (project.update)', 'Recurrence', [], None, '200', ['403', '404']),
    ('post', '/projects/{id}/recurrences/{recurrence_id}/run-now', 'Create the next copy now, as the caller, without moving the schedule; returns the new task id (project.update)', 'Recurrence', [], None, '201', ['403', '404', '409']),
    # --- public project links
    ('get', '/projects/{id}/public-links', 'List the active public links of a project, never their secrets (project.update)', 'Public links', [], None, '200', ['403', '404']),
    ('post', '/projects/{id}/public-links', 'Create a read-only public link; the secret token is shown once in this response; at most 3 active per project (project.update)', 'Public links', [], 'PublicLinkInput', '201', ['403', '404', '409']),
    ('delete', '/projects/{id}/public-links/{link_id}', 'Revoke a public link at once (project.update)', 'Public links', [], None, '200', ['403', '404']),
    ('get', '/public/projects/{token}', 'Read-only page of a project (name, columns, tasks without people), authenticated by the secret token in the path only (no bearer token, rate limited per IP, never cached); unknown, revoked and archived answer the same 404', 'Public links',
     CURSOR, None, '200', ['400', '404', '429'], 'public'),
]

SCHEMAS = {
    'RuleInput': dict(required=['name', 'trigger', 'actions'], props={'name': STR,
        'trigger': dict(type='string', enum=['task.created', 'task.moved', 'task.assigned', 'task.completed', 'task.commented', 'task.due_changed', 'task.pr_merged']),
        'conditions': dict(type='array', maxItems=10, items=dict(type='object', description='{field, value}: column_id, column_category, priority, label_id, assignee_id; to_column_id and from_column_id for task.moved; assigned_user_id for task.assigned')),
        'actions': dict(type='array', minItems=1, maxItems=10, items=dict(type='object', description='{type: move_to_column|set_priority|add_label|assign_user|add_comment|set_due_in_days, column_id|priority|label_id|user_id|text|days}')),
        'active': BOOL}),
    'RuleUpdate': dict(required=[], props={'name': STR, 'trigger': STR, 'conditions': dict(type='array', items=dict(type='object')),
        'actions': dict(type='array', items=dict(type='object')), 'active': BOOL, 'version': INT}),
    'RuleTestInput': dict(required=['task_id'], props={'task_id': UUID}),
    'RecurrenceInput': dict(required=['template_task_id', 'frequency'], props={'template_task_id': UUID, 'frequency': dict(type='string', enum=['daily', 'weekly', 'monthly']),
        'interval': dict(type='integer', minimum=1, maximum=52), 'weekday': dict(type='integer', minimum=0, maximum=6, description='0 = Sunday; weekly only'),
        'day_of_month': dict(type='integer', minimum=1, maximum=31, description='monthly only; clamped to the last day of short months'),
        'first_run_at': dict(type='string', format='date-time'), 'until_on': DATE}),
    'RecurrenceUpdate': dict(required=[], props={'frequency': dict(type='string', enum=['daily', 'weekly', 'monthly']), 'interval': dict(type='integer', minimum=1, maximum=52),
        'weekday': dict(type='integer', minimum=0, maximum=6), 'day_of_month': dict(type='integer', minimum=1, maximum=31),
        'next_run_at': dict(type='string', format='date-time'), 'until_on': dict(type='string', format='date', nullable=True), 'active': BOOL, 'version': INT}),
    'PublicLinkInput': dict(required=[], props={'show_descriptions': BOOL}),
}
