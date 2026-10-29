"""M2 outgoing webhooks, chat messages and git integrations (phase 4). Merged into the swagger by scripts/gen_projects_swagger.py.
A 9th tuple element 'public' marks a route without a bearer token; the one public route here is the provider call, which is
authenticated by the HMAC of the raw body (X-Hub-Signature-256 for GitHub, X-Gitea-Signature for Gitea) and answers JSON."""

STR, BOOL = dict(type='string'), dict(type='boolean')
Q = lambda name, typ='string', **kw: dict(name=name, **{'in': 'query'}, schema=dict(type=typ, **kw))
CURSOR = [Q('cursor'), Q('limit', 'integer', minimum=1, maximum=100)]
EVENTS = dict(type='array', maxItems=30, items=STR, description='"*" or event names such as task.created, task.moved, sprint.started')

# (method, path, summary, tag, query params, body schema, success status, error codes[, 'public'])
ROUTES = [
    # --- outgoing webhooks (project.update manages, project.read lists deliveries)
    ('get', '/projects/{id}/webhooks', 'List the webhooks of a project; the signing secret is never returned (project.update)', 'Webhooks', [], None, '200', ['403', '404']),
    ('post', '/projects/{id}/webhooks', 'Add a webhook (https URL on port 443/8443, never a private address); at most 20 per project; the signing secret is returned once; 503 SECRETS_UNAVAILABLE when no secrets key is configured (project.update)', 'Webhooks', [], 'WebhookInput', '201', ['400', '403', '404', '409']),
    ('patch', '/projects/{id}/webhooks/{webhook_id}', 'Edit a webhook; active=true re-enables an automatically disabled one (project.update)', 'Webhooks', [], 'WebhookUpdate', '200', ['400', '403', '404', '409']),
    ('delete', '/projects/{id}/webhooks/{webhook_id}', 'Delete a webhook and its deliveries (project.update)', 'Webhooks', [], None, '200', ['403', '404']),
    ('post', '/projects/{id}/webhooks/{webhook_id}/rotate-secret', 'Replace the signing secret; the new one is returned once (project.update)', 'Webhooks', [], None, '200', ['403', '404']),
    ('post', '/projects/{id}/webhooks/{webhook_id}/test', 'Send a signed test ping now; the outcome (also a failure) is recorded as a delivery (project.update)', 'Webhooks', [], None, '200', ['403', '404', '429']),
    ('get', '/projects/{id}/webhooks/{webhook_id}/deliveries', 'Deliveries of a webhook, newest first, with status, attempts and the short last error (project.read)', 'Webhooks',
     [Q('status', enum=['pending', 'delivered', 'failed', 'dead'])] + CURSOR, None, '200', ['400', '403', '404']),
    ('post', '/projects/{id}/webhooks/{webhook_id}/deliveries/{delivery_id}/redeliver', 'Queue a settled delivery for another round of attempts (project.update)', 'Webhooks', [], None, '200', ['403', '404', '409']),
    # --- git integrations (GitHub, Gitea)
    ('get', '/projects/{id}/integrations', 'List the git integrations of a project; secrets are never returned (project.update)', 'Integrations', [], None, '200', ['403', '404']),
    ('post', '/projects/{id}/integrations', 'Add a GitHub or Gitea integration; at most 10 per project; the secret and the incoming path are returned once (project.update)', 'Integrations', [], 'IntegrationInput', '201', ['400', '403', '404', '409']),
    ('delete', '/projects/{id}/integrations/{integration_id}', 'Delete an integration; the links it created stay (project.update)', 'Integrations', [], None, '200', ['403', '404']),
    ('post', '/projects/{id}/integrations/{integration_id}/rotate-secret', 'Replace the integration secret; the new one is returned once (project.update)', 'Integrations', [], None, '200', ['403', '404']),
    ('get', '/tasks/{id}/links', 'Pull requests, branches and commits linked to a task: provider, kind, id, https url and state (task.read)', 'Integrations', [], None, '200', ['404']),
    ('post', '/integrations/{provider}/{integration_id}', 'Provider call (pull_request opened/closed/reopened, push): verifies the HMAC of the raw body before parsing, links tasks of the integration project named by KEY-123; unknown ids and bad signatures answer the same 404; rate limited per IP', 'Integrations',
     [], 'ProviderEvent', '200', ['400', '404', '429'], 'public'),
]

SCHEMAS = {
    'WebhookInput': dict(required=['name', 'url'], props={'name': STR, 'url': dict(type='string', format='uri', description='https only, ports 443 and 8443, no credentials, no private or internal address'),
        'format': dict(type='string', enum=['generic', 'slack', 'discord']), 'event_types': EVENTS}),
    'WebhookUpdate': dict(required=[], props={'name': STR, 'url': STR, 'format': dict(type='string', enum=['generic', 'slack', 'discord']), 'event_types': EVENTS, 'active': BOOL}),
    'IntegrationInput': dict(required=['provider', 'name'], props={'provider': dict(type='string', enum=['github', 'gitea']), 'name': STR}),
    'ProviderEvent': dict(required=[], props={'action': STR, 'ref': STR, 'pull_request': dict(type='object'), 'commits': dict(type='array', items=dict(type='object')),
        'repository': dict(type='object')}),
}
