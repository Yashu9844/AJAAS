"""M2 collaboration routes (comments, mentions, watchers, time tracking, attachments).
Merged into the swagger by scripts/gen_projects_swagger.py. Attachment endpoints also answer 503 STORAGE_UNAVAILABLE when
object storage is not configured (the generator has no 503 entry, so it is described here and in the summaries)."""

STR, INT, BOOL, DATE = dict(type='string'), dict(type='integer'), dict(type='boolean'), dict(type='string', format='date')
UUID = dict(type='string', format='uuid')
DATETIME = dict(type='string', format='date-time')
Q = lambda name, typ='string', **kw: dict(name=name, **{'in': 'query'}, schema=dict(type=typ, **kw))
CURSOR = [Q('cursor'), Q('limit', 'integer', minimum=1, maximum=100)]

# (method, path, summary, tag, query params, body schema, success status, error codes)
ROUTES = [
    # --- comments and mentions (CM-01, CM-02)
    ('get', '/tasks/{id}/comments', 'Comments of a task, oldest first; deleted ones are tombstones with an empty body (task.read)', 'Comments', CURSOR, None, '200', ['400', '404']),
    ('post', '/tasks/{id}/comments', 'Add a markdown comment (max 10000); @[Name](user:uuid) mentions of users who cannot read the project are dropped (task.comment)', 'Comments', [], 'CommentInput', '201', ['400', '403', '404', '409']),
    ('patch', '/comments/{id}', 'Edit your own comment; sets edited_at (task.comment)', 'Comments', [], 'CommentInput', '200', ['400', '403', '404', '409']),
    ('delete', '/comments/{id}', 'Delete your own comment, or any with comment.moderate; leaves a tombstone', 'Comments', [], None, '200', ['403', '404', '409']),
    # --- watchers (CM-03)
    ('get', '/tasks/{id}/watch', 'Whether the caller watches the task, and why (task.read)', 'Watchers', [], None, '200', ['404']),
    ('put', '/tasks/{id}/watch', 'Watch the task as the caller (task.read)', 'Watchers', [], None, '200', ['404']),
    ('delete', '/tasks/{id}/watch', 'Stop watching the task as the caller', 'Watchers', [], None, '200', ['404']),
    ('get', '/tasks/{id}/watchers', 'Watchers of a task by user id (task.read)', 'Watchers', CURSOR, None, '200', ['400', '404']),
    # --- time tracking (TM-01..TM-03)
    ('get', '/tasks/{id}/time', 'Time entries of a task, newest first; mine=true limits to the caller (task.read)', 'Time tracking', [Q('mine', 'boolean')] + CURSOR, None, '200', ['400', '404']),
    ('post', '/tasks/{id}/time', 'Log finished work: started_at, minutes 1..1440, note (time.log); 409 TIME_TRACKING_DISABLED, TIME_LOCKED', 'Time tracking', [], 'TimeLogInput', '201', ['400', '403', '404', '409']),
    ('patch', '/time-entries/{id}', 'Edit an entry: the author within the 30-day lock window, time.manage holders always; 409 TIMER_RUNNING for a running timer', 'Time tracking', [], 'TimeUpdateInput', '200', ['400', '403', '404', '409']),
    ('delete', '/time-entries/{id}', 'Delete an entry (author within the lock window, or time.manage); also discards a running timer', 'Time tracking', [], None, '200', ['403', '404', '409']),
    ('post', '/time-entries/timer/start', 'Start the caller timer on a task; one running timer per user, else 409 TIMER_RUNNING (time.log)', 'Time tracking', [], 'TimerStartInput', '201', ['400', '403', '404', '409']),
    ('post', '/time-entries/timer/stop', 'Stop the caller running timer and log the elapsed minutes; 404 when none runs', 'Time tracking', [], None, '200', ['404']),
    ('get', '/time-entries/timer', 'The caller running timer, or data null when none runs', 'Time tracking', [], None, '200', []),
    ('get', '/projects/{id}/time/report', 'Logged minutes of a project for a date range, per task (with estimate) or per user (task.read)', 'Time tracking',
     [Q('from', format='date'), Q('to', format='date'), Q('group_by', enum=['task', 'user'])], None, '200', ['400', '403', '404']),
    # --- attachments (AT-01..AT-04); every one answers 503 STORAGE_UNAVAILABLE when storage is not configured
    ('post', '/tasks/{id}/attachments', 'Start an upload: validates the file and returns the pending attachment with a presigned PUT valid for 5 minutes (attachment.upload)', 'Attachments', [], 'AttachmentInitInput', '201', ['400', '403', '404', '409']),
    ('get', '/tasks/{id}/attachments', 'Ready attachments of a task (task.read)', 'Attachments', CURSOR, None, '200', ['400', '404']),
    ('post', '/attachments/{id}/complete', 'Verify the uploaded object (size, type, checksum), scan it and mark it ready; 409 UPLOAD_MISMATCH keeps it pending', 'Attachments', [], None, '200', ['403', '404', '409']),
    ('get', '/attachments/{id}/download', 'Presigned download URL valid for 2 minutes, authorised against the task project (task.read)', 'Attachments', [], None, '200', ['404']),
    ('delete', '/attachments/{id}', 'Delete an attachment and its object (the uploader, or attachment.delete)', 'Attachments', [], None, '200', ['403', '404', '409']),
]

SCHEMAS = {
    'CommentInput': dict(required=['body'], props={'body': STR}),
    'TimeLogInput': dict(required=['started_at', 'minutes'], props={'started_at': DATETIME, 'minutes': INT, 'note': STR}),
    'TimeUpdateInput': dict(required=[], props={'started_at': DATETIME, 'minutes': INT, 'note': STR}),
    'TimerStartInput': dict(required=['task_id'], props={'task_id': UUID}),
    'AttachmentInitInput': dict(required=['filename', 'size', 'content_type'], props={'filename': STR, 'size': INT, 'content_type': STR,
        'sha256': dict(type='string', description='optional lowercase hex checksum the object must match')}),
}
