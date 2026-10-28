"""M2 planning and views routes (sprints, backlog, plan, board, calendar, Gantt, My work, search, saved views, iCalendar).
Merged into the swagger by scripts/gen_projects_swagger.py. A 9th tuple element 'public' marks a route without a bearer token."""

STR, INT, BOOL, DATE = dict(type='string'), dict(type='integer'), dict(type='boolean'), dict(type='string', format='date')
UUID = dict(type='string', format='uuid')
Q = lambda name, typ='string', **kw: dict(name=name, **{'in': 'query'}, schema=dict(type=typ, **kw))
CURSOR = [Q('cursor'), Q('limit', 'integer', minimum=1, maximum=100)]
WINDOW = [Q('from', format='date'), Q('to', format='date')]

# (method, path, summary, tag, query params, body schema, success status, error codes[, 'public'])
ROUTES = [
    # --- sprints (scrum projects only; kanban answers 409 PROJECT_MODE)
    ('post', '/projects/{id}/sprints', 'Create a planned sprint in a scrum project (sprint.manage)', 'Planning', [], 'SprintInput', '201', ['400', '403', '404', '409']),
    ('get', '/projects/{id}/sprints', 'List the sprints of a scrum project with task counts, newest first', 'Planning',
     [Q('status', enum=['planned', 'active', 'completed'])] + CURSOR, None, '200', ['400', '404', '409']),
    ('get', '/sprints/{id}', 'One sprint with its task count', 'Planning', [], None, '200', ['404', '409']),
    ('patch', '/sprints/{id}', 'Edit a planned or active sprint; send version (or If-Match) (sprint.manage)', 'Planning', [], 'SprintInput', '200', ['400', '403', '404', '409']),
    ('delete', '/sprints/{id}', 'Delete a planned sprint; its tasks return to the backlog (sprint.manage)', 'Planning', [], None, '200', ['403', '404', '409']),
    ('post', '/sprints/{id}/start', 'Start a planned sprint that has dates; one active sprint per project (sprint.manage)', 'Planning', [], None, '200', ['403', '404', '409']),
    ('post', '/sprints/{id}/complete', 'Complete the active sprint, moving unfinished tasks to the next planned sprint or the backlog (sprint.manage)', 'Planning', [], 'SprintCompleteInput', '200', ['400', '403', '404', '409']),
    ('get', '/sprints/{id}/report', 'Committed versus completed counts and estimates (stored when completed, live otherwise)', 'Planning', [], None, '200', ['404', '409']),
    # --- backlog and planning
    ('get', '/projects/{id}/backlog', 'Backlog tasks by rank: backlog-category columns (kanban) or tasks without a sprint (scrum) (task.read)', 'Planning', CURSOR, None, '200', ['400', '403', '404']),
    ('post', '/projects/{id}/plan', 'Move up to 100 tasks between the backlog and a sprint; per-task result list (task.update)', 'Planning', [], 'PlanInput', '200', ['400', '403', '404', '409']),
    # --- read models
    ('get', '/projects/{id}/board', 'Board: columns with counts and cursor-paged tasks per column; cursor_<columnId> pages one column (task.read)', 'Views',
     [Q('per_column', 'integer', minimum=1, maximum=100), Q('sprint')], None, '200', ['400', '403', '404']),
    ('get', '/projects/{id}/calendar', 'Tasks by due date or start date in a window of at most 62 days (task.read)', 'Views', WINDOW, None, '200', ['400', '403', '404']),
    ('get', '/projects/{id}/gantt', 'Dated tasks, milestones and dependencies for a window, at most 500 tasks per page (task.read)', 'Views',
     WINDOW + [Q('cursor'), Q('limit', 'integer', minimum=1, maximum=500)], None, '200', ['400', '403', '404']),
    ('get', '/my-work', 'Tasks assigned to me, grouped overdue / today / this_week / later / no_date, with counts', 'Views',
     [Q('group', enum=['overdue', 'today', 'this_week', 'later', 'no_date']), Q('per_group', 'integer', minimum=1, maximum=100), Q('cursor')], None, '200', ['400']),
    ('get', '/my-work/counts', 'Badge counts for My work', 'Views', [], None, '200', []),
    ('get', '/calendar', 'My calendar: assigned tasks across readable projects in a window of at most 62 days', 'Views', WINDOW, None, '200', ['400']),
    ('get', '/search', 'Full-text and ticket-id search over readable projects of the tenant, at most 50 hits', 'Views',
     [Q('q'), Q('project_id', format='uuid'), Q('limit', 'integer', minimum=1, maximum=50)], None, '200', ['400', '404']),
    # --- saved views
    ('post', '/saved-views', 'Save a view; shared views belong to a project and need view.share', 'Views', [], 'SavedViewInput', '201', ['400', '403', '404', '409']),
    ('get', '/saved-views', 'My views plus the shared views of readable projects', 'Views', [Q('project_id', format='uuid')] + CURSOR, None, '200', ['400', '404']),
    ('patch', '/saved-views/{id}', 'Edit a view (owner, or view.share for a shared one); send version (or If-Match)', 'Views', [], 'SavedViewInput', '200', ['400', '403', '404', '409']),
    ('delete', '/saved-views/{id}', 'Delete a view (owner, or view.share for a shared one)', 'Views', [], None, '200', ['403', '404']),
    # --- iCalendar feed
    ('post', '/calendar-tokens', 'Create an iCalendar feed token; the secret is shown once in this response', 'Views', [], 'CalendarTokenInput', '201', ['400', '409']),
    ('get', '/calendar-tokens', 'List my active feed tokens (never the secrets)', 'Views', [], None, '200', []),
    ('delete', '/calendar-tokens/{id}', 'Revoke one of my feed tokens at once', 'Views', [], None, '200', ['404']),
    ('get', '/calendar/feed.ics', 'iCalendar feed of my assigned tasks, authenticated by the token query parameter only (no bearer token, rate limited per IP)', 'Views',
     [Q('token')], None, '200', ['404', '429'], 'public'),
]

SCHEMAS = {
    'SprintInput': dict(required=['name'], props={'name': STR, 'goal': STR, 'start_on': DATE, 'end_on': DATE, 'version': INT}),
    'SprintCompleteInput': dict(required=['move_unfinished_to'], props={'move_unfinished_to': dict(type='string', description='backlog or the id of the next planned sprint')}),
    'PlanInput': dict(required=['task_ids'], props={'task_ids': dict(type='array', maxItems=100, items=UUID), 'sprint_id': dict(type='string', format='uuid', nullable=True),
        'versions': dict(type='object', additionalProperties=INT)}),
    'SavedViewInput': dict(required=[], props={'name': STR, 'layout': dict(type='string', enum=['board', 'list', 'calendar', 'gantt']), 'project_id': UUID,
        'filters': dict(type='object'), 'sort': dict(type='array', items=dict(type='object')),
        'grouping': dict(type='string', enum=['', 'column', 'assignee', 'priority', 'milestone', 'sprint']), 'shared': BOOL, 'version': INT}),
    'CalendarTokenInput': dict(required=[], props={'label': STR}),
}
