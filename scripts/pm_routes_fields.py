"""M2 custom-field routes (phase 4). Merged into the swagger by scripts/gen_projects_swagger.py."""

STR = dict(type='string')

ROUTES = [
    ('get', '/projects/{id}/custom-fields', 'List the custom fields of a project (project.read)', 'Custom fields', [], None, '200', ['404']),
    ('post', '/projects/{id}/custom-fields', 'Add a typed custom field; at most 30 per project (project.update)', 'Custom fields', [], 'CustomFieldInput', '201', ['400', '403', '404', '409']),
    ('patch', '/projects/{id}/custom-fields/{field_id}', 'Rename a field or change its options; values using a removed option are cleared (project.update)', 'Custom fields', [], 'CustomFieldUpdate', '200', ['400', '403', '404', '409']),
    ('delete', '/projects/{id}/custom-fields/{field_id}', 'Delete a field and all its values (project.update)', 'Custom fields', [], None, '200', ['403', '404']),
    ('get', '/tasks/{id}/custom-fields', 'Every field of the task project with the task value (task.read)', 'Custom fields', [], None, '200', ['404']),
    ('put', '/tasks/{id}/custom-fields', 'Set or clear values by field key; send version (or If-Match) for optimistic concurrency (task.update)', 'Custom fields', [], 'CustomFieldValuesInput', '200', ['400', '403', '404', '409']),
]

SCHEMAS = {
    'CustomFieldInput': dict(required=['name', 'type'], props={'name': STR, 'type': dict(type='string', enum=['text', 'number', 'checkbox', 'date', 'url', 'select', 'multiselect', 'user']),
        'options': dict(type='array', items=STR)}),
    'CustomFieldUpdate': dict(required=[], props={'name': STR, 'options': dict(type='array', items=STR)}),
    'CustomFieldValuesInput': dict(required=['values'], props={'values': dict(type='object', description='field key to value; null clears'), 'version': dict(type='integer')}),
}
