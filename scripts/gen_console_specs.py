"""Generate test-console operation specs from backend/api/swagger.yaml (docs/TEST_CONSOLE.md).

Usage: python scripts/gen_console_specs.py <repo>
Writes frontend/src/modules/devtest/generated/hrSpecs.ts with one OpSpec list per HR context listed in CONTEXTS.
Every operation of the context becomes a form: path params, query params and request-body fields (types, enums,
required flags) come from the contract; POST operations capture data.id so later forms default to it.
Re-run whenever the swagger changes; never hand-edit the output.
"""
import json
import os
import re
import sys

import yaml

# context → (export name, path prefixes)
CONTEXTS = {
    'attendance': ('HR_ATTENDANCE', ['/hr/attendance', '/hr/shifts', '/hr/shift-assignments']),
    'leave': ('HR_LEAVE', ['/hr/leave']),
    'payroll': ('HR_PAYROLL', ['/hr/payroll']),
    'recruitment': ('HR_RECRUITMENT', ['/hr/recruitment']),
    'performance': ('HR_PERFORMANCE', ['/hr/performance']),
    'core': ('HR_CORE', ['/hr/core']),
    'compensation': ('HR_COMPENSATION', ['/hr/compensation']),
    'onboarding': ('HR_ONBOARDING', ['/hr/onboarding']),
    'learning': ('HR_LEARNING', ['/hr/learning']),
    'benefits': ('HR_BENEFITS', ['/hr/benefits']),
    'employee_records': ('HR_EMPLOYEE_RECORDS', [b + r for b in ('/hr/employees/me', '/hr/employees/{id}') for r in
                         ('/emergency-contacts', '/education', '/prior-employment', '/change-requests')] + ['/hr/employee-change-requests']),
}
# M2 Projects & Tasks: routes live at the /api/v1 root, written to their own generated file.
# A path belongs to the context with the longest matching prefix, so nested prefixes can be split between contexts.
PM_CONTEXTS = {
    'projects': ('PM_PROJECTS', ['/projects', '/project-roles', '/project-permissions', '/project-labels']),
    'tasks': ('PM_TASKS', ['/projects/{id}/tasks', '/projects/{id}/activity', '/projects/{id}/time', '/tasks', '/tickets', '/comments', '/time-entries', '/attachments']),
    'planning': ('PM_PLANNING', ['/projects/{id}/sprints', '/projects/{id}/backlog', '/projects/{id}/board', '/projects/{id}/calendar', '/projects/{id}/gantt',
                                 '/projects/{id}/plan', '/sprints', '/my-work', '/search', '/saved-views', '/calendar-tokens', '/calendar']),
    'platform': ('PM_PLATFORM', ['/projects/{id}/group-rules']),
}


def pm_owner(path):
    best, owner = -1, None
    for ctx, (_, prefixes) in PM_CONTEXTS.items():
        for p in prefixes:
            if (path == p or path.startswith(p + '/')) and len(p) > best:
                best, owner = len(p), ctx
    return owner
METHODS = ['post', 'get', 'put', 'patch', 'delete']


def resolve(doc, node):
    seen = 0
    while isinstance(node, dict) and '$ref' in node and seen < 10:
        _, _, kind, name = node['$ref'].split('/')
        node = doc['components'][kind][name]
        seen += 1
    return node or {}


def merged(doc, schema):
    """Flatten allOf into one object schema."""
    schema = resolve(doc, schema)
    if 'allOf' not in schema:
        return schema
    props, req = {}, []
    for part in schema['allOf']:
        part = merged(doc, part)
        props.update(part.get('properties', {}))
        req += part.get('required', [])
    return {'type': 'object', 'properties': props, 'required': req}


def field_type(doc, prop):
    prop = resolve(doc, prop)
    if 'enum' in prop:
        return 'select', [''] + [str(x) for x in prop['enum']]
    t, fmt = prop.get('type'), prop.get('format', '')
    if t in ('integer', 'number'):
        return 'number', None
    if t == 'boolean':
        return 'bool', None
    if t == 'array':
        items = resolve(doc, prop.get('items', {}))
        return ('list', None) if items.get('type', 'string') == 'string' else ('json', None)
    if t == 'object' or 'properties' in prop:
        return 'json', None
    if fmt == 'date-time':
        return 'datetime', None
    return 'text', None


def placeholder(doc, prop):
    prop = resolve(doc, prop)
    if 'example' in prop:
        return str(prop['example'])
    fmt = prop.get('format', '')
    if fmt == 'date':
        return 'YYYY-MM-DD'
    if fmt == 'uuid':
        return 'uuid'
    if prop.get('pattern'):
        return prop['pattern']
    if prop.get('description'):
        return prop['description'][:60]
    return ''


def id_key(path):
    """Collection name of a path → captured id key, e.g. /hr/leave/types/{id} → leaveTypesId."""
    segs = [s for s in path.split('/') if s and not s.startswith('{') and s != 'hr']
    base = segs[:2] if len(segs) > 1 else segs
    words = re.split(r'[-_]', '-'.join(base))
    return words[0] + ''.join(w.capitalize() for w in words[1:]) + 'Id'


def collection_of(path, param):
    """The collection a {param} belongs to: the segment before it."""
    segs = path.split('/')
    i = segs.index('{' + param + '}')
    return '/'.join(segs[:i])


def op_fields(doc, path, op, shared=()):
    fields = []
    for prm in list(shared) + op.get('parameters', []):
        prm = resolve(doc, prm) if '$ref' in prm else prm
        if prm.get('in') not in ('path', 'query'):
            continue
        name = prm['name']
        f = {'name': name, 'where': prm['in']}
        if prm['in'] == 'path':
            f.update(required=True, ref=id_key(collection_of(path, name) + '/x'))
        t, opts = field_type(doc, prm.get('schema', {}))
        if t != 'text':
            f['type'] = t
        if opts:
            f['options'] = opts
        if prm.get('required') and prm['in'] == 'query':
            f['required'] = True
        fields.append(f)
    for name in re.findall(r'\{(\w+)\}', path):
        if not any(f['name'] == name and f['where'] == 'path' for f in fields):
            fields.insert(0, {'name': name, 'where': 'path', 'required': True, 'ref': id_key(collection_of(path, name) + '/x')})
    body = op.get('requestBody', {}).get('content', {}).get('application/json', {}).get('schema')
    if body:
        schema = merged(doc, body)
        req = set(schema.get('required', []))
        for name, prop in schema.get('properties', {}).items():
            t, opts = field_type(doc, prop)
            f = {'name': name, 'where': 'body'}
            if t != 'text':
                f['type'] = t
            if opts:
                f['options'] = opts
            if name in req:
                f['required'] = True
            ph = placeholder(doc, prop)
            if ph:
                f['placeholder'] = ph
            fields.append(f)
    return fields


def ops_for(doc, prefixes, ctx, ns='hr'):
    out = []
    for path in sorted(doc['paths']):
        if ns == 'pm':
            if pm_owner(path) != ctx:
                continue
        elif not any(path == p or path.startswith(p + '/') for p in prefixes):
            continue
        item = doc['paths'][path]
        for method in METHODS:
            op = item.get(method)
            if not op:
                continue
            expect = sorted(int(c) for c in op.get('responses', {}) if str(c).isdigit() and 200 <= int(c) < 300) or [200]
            console_path = re.sub(r'\{(\w+)\}', r':\1', path)
            spec = {'key': '%s.%s.%s.%s' % (ns, ctx, method, re.sub(r'[^a-z0-9]+', '.', console_path.lower()).strip('.')),
                    'label': op.get('summary', method.upper() + ' ' + path), 'method': method.upper(), 'path': console_path,
                    'scope': 'tenant', 'auth': True, 'fields': op_fields(doc, path, op, item.get('parameters', [])), 'expect': expect}
            if method == 'post' and 201 in expect and not path.endswith('}'):
                spec['capture'] = [{'idKey': id_key(path + '/x'), 'from': 'data.id'}]
            errs = ['%s %s' % (c, op['responses'][c].get('description', '')) for c in sorted(op.get('responses', {}), key=str)
                    if str(c).isdigit() and int(c) >= 400 and op['responses'][c].get('description')]
            if errs:
                spec['note'] = 'Errors: ' + '; '.join(errs)
            out.append(spec)
    return out


def main(repo):
    with open(os.path.join(repo, 'backend', 'api', 'swagger.yaml'), encoding='utf-8') as fh:
        doc = yaml.safe_load(fh)
    lines = ['// GENERATED by scripts/gen_console_specs.py from backend/api/swagger.yaml — do not edit by hand.',
             'import type { OpSpec } from "../types";', '']
    for ctx, (name, prefixes) in CONTEXTS.items():
        ops = ops_for(doc, prefixes, ctx)
        lines.append('export const %s: OpSpec[] = %s;' % (name, json.dumps(ops, indent=2, ensure_ascii=False)))
        lines.append('')
        print(ctx, len(ops), 'operations')
    out = os.path.join(repo, 'frontend', 'src', 'modules', 'devtest', 'generated', 'hrSpecs.ts')
    os.makedirs(os.path.dirname(out), exist_ok=True)
    with open(out, 'w', encoding='utf-8', newline='\n') as fh:
        fh.write('\n'.join(lines))
    pm = lines[:2] + ['']
    for ctx, (name, prefixes) in PM_CONTEXTS.items():
        ops = ops_for(doc, prefixes, ctx, 'pm')
        pm.append('export const %s: OpSpec[] = %s;' % (name, json.dumps(ops, indent=2, ensure_ascii=False)))
        pm.append('')
        print('pm', ctx, len(ops), 'operations')
    with open(out.replace('hrSpecs.ts', 'projectsSpecs.ts'), 'w', encoding='utf-8', newline='\n') as fh:
        fh.write('\n'.join(pm) + '\n')


main(sys.argv[1])
