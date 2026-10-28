"""Generate the exhaustive appendix of docs/SYSTEM_REFERENCE.md straight from the source tree (read-only)."""
import os
import re
import sys

ROOT = sys.argv[1]            # repo root
OUT = sys.argv[2]             # output markdown path for the appendix
BACKEND = os.path.join(ROOT, 'backend')
INTERNAL = os.path.join(BACKEND, 'internal')

# M2 Projects & Tasks packages live under internal/projects/<dir>.
M2 = {'m2_kernel': 'kernel', 'm2_api': 'api', 'm2_project': 'project', 'm2_task': 'task', 'm2_collab': 'collab', 'm2_planning': 'planning', 'm2_fields': 'fields'}

MODULE_ORDER = ['shared', 'app', 'identity', 'organization', 'employee', 'attendance', 'leave', 'payroll',
                'recruitment', 'performance', 'core', 'compensation', 'onboarding', 'learning', 'benefits',
                'm2_kernel', 'm2_api', 'm2_project', 'm2_task', 'm2_collab', 'm2_planning', 'm2_fields']
TITLES = {'shared': 'Shared infrastructure', 'app': 'Application wiring (internal/app)', 'identity': 'M0 Identity & Access',
          'organization': 'M1 HR / Organization', 'employee': 'M1 HR / Employee', 'attendance': 'M1 HR / Attendance',
          'leave': 'M1 HR / Leave', 'payroll': 'M1 HR / Payroll', 'recruitment': 'M1 HR / Recruitment',
          'performance': 'M1 HR / Performance & OKRs', 'core': 'M1 HR / Core HCM', 'compensation': 'M1 HR / Compensation', 'onboarding': 'M1 HR / Onboarding', 'learning': 'M1 HR / Learning', 'benefits': 'M1 HR / Benefits',
          'm2_kernel': 'M2 Projects / kernel', 'm2_api': 'M2 Projects / api', 'm2_project': 'M2 Projects / project',
          'm2_task': 'M2 Projects / task', 'm2_collab': 'M2 Projects / collab', 'm2_planning': 'M2 Projects / planning', 'm2_fields': 'M2 Projects / fields'}


def read(path):
    with open(path, encoding='utf-8', errors='replace') as f:
        return f.read().replace('\r\n', '\n')


def decls(src):
    """Yield (kind, signature, doc) for top-level types, funcs, and interface methods."""
    lines = src.split('\n')
    out = []
    i = 0
    in_iface = None
    while i < len(lines):
        line = lines[i]
        doc = []
        j = i - 1
        while j >= 0 and lines[j].startswith('//'):
            doc.insert(0, lines[j][2:].strip())
            j -= 1
        if in_iface is not None:
            if line.startswith('}'):
                in_iface = None
            else:
                m = re.match(r'^\t([A-Z]\w*)\((.*)$', line)
                if m:
                    mdoc = lines[i - 1].strip()[2:].strip() if lines[i - 1].strip().startswith('//') else ''
                    out.append(('method', f'{in_iface}.{line.strip()}', mdoc))
        elif line.startswith('func '):
            sig = line.rstrip(' {')
            out.append(('func', sig[5:], ' '.join(doc)))
        elif re.match(r'^type \w+ ', line):
            sig = line.rstrip(' {')
            out.append(('type', sig[5:], ' '.join(doc)))
            m = re.match(r'^type (\w+) interface \{$', line)
            if m:
                in_iface = m.group(1)
        i += 1
    return out


def package_doc(src):
    m = re.search(r'((?://[^\n]*\n)+)package \w+', src)
    if not m:
        return ''
    return ' '.join(l[2:].strip() for l in m.group(1).strip().split('\n'))


HR = {'organization', 'employee', 'attendance', 'leave', 'payroll', 'recruitment', 'performance', 'core', 'compensation', 'onboarding', 'learning', 'benefits'}


def module_dir(module):
    if module in M2:
        return os.path.join(INTERNAL, 'projects', M2[module])
    return os.path.join(INTERNAL, 'hr', module) if module in HR else os.path.join(INTERNAL, module)


def go_files(module):
    base = module_dir(module)
    for dirpath, _, files in sorted(os.walk(base)):
        for f in sorted(files):
            if f.endswith('.go') and not f.endswith('_test.go'):
                yield os.path.join(dirpath, f)


def test_files(module):
    base = module_dir(module)
    res = []
    for dirpath, _, files in sorted(os.walk(base)):
        for f in sorted(files):
            if f.endswith('_test.go'):
                p = os.path.join(dirpath, f)
                names = re.findall(r'^func (Test\w+)', read(p), re.M)
                res.append((os.path.relpath(p, BACKEND).replace('\\', '/'), names))
    return res


def routes(module):
    """Routes of a context: the controller-style routes/routes.go plus every RegisterRoutes in the context's other
    packages (single-package contexts and the plus/ops/records sub-packages register routes in http.go)."""
    rows = []
    if module in M2:
        return m2_routes(module)
    main = os.path.join(module_dir(module), 'routes', 'routes.go')
    if os.path.exists(main):
        rows += routes_in(module, main)
    if module in HR:
        for dirpath, _, files in os.walk(module_dir(module)):
            for f in sorted(files):
                p = os.path.join(dirpath, f)
                if f.endswith('.go') and not f.endswith('_test.go') and os.path.abspath(p) != os.path.abspath(main) and 'RegisterRoutes(' in read(p):
                    rows += routes_in(module, p)
    if module == 'employee':
        rows += records_routes()
    return rows


def m2_routes(module):
    """M2 handlers register routes through closures, so list them from the route tables that generate the contract
    (scripts/pm_routes_*.py plus the base table in gen_projects_swagger.py); TestSwaggerMatchesRoutes proves them equal to the router."""
    import glob
    import importlib.util
    here = os.path.dirname(os.path.abspath(__file__))
    owner = {'tasks': 'm2_task', 'collab': 'm2_collab', 'planning': 'm2_planning', 'platform': 'm2_project', 'fields': 'm2_fields'}
    spec = importlib.util.spec_from_file_location('gen_projects_swagger', os.path.join(here, 'gen_projects_swagger.py'))
    base = importlib.util.module_from_spec(spec)
    spec.loader.exec_module(base)
    separate, rows = set(), []
    for f in sorted(glob.glob(os.path.join(here, 'pm_routes_*.py'))):
        name = os.path.basename(f)[len('pm_routes_'):-3]
        sp = importlib.util.spec_from_file_location('pm_' + name, f)
        mod = importlib.util.module_from_spec(sp)
        sp.loader.exec_module(mod)
        for r in mod.ROUTES:
            separate.add((r[0], r[1]))
            if owner.get(name) == module:
                rows.append((r[0].upper(), '/api/v1' + re.sub(r'\{(\w+)\}', r':\1', r[1]), '(from the contract; ' + r[2] + ')'))
    if module == 'm2_project':
        for r in base.ROUTES:
            if (r[0], r[1]) not in separate:
                rows.append((r[0].upper(), '/api/v1' + re.sub(r'\{(\w+)\}', r':\1', r[1]), '(from the contract; ' + r[2] + ')'))
    return rows


def records_routes():
    """employee/records registers its routes from a loop, so they cannot be read statically; list them from the contract,
    which TestSwaggerMatchesRoutes proves equal to the router."""
    import yaml
    doc = yaml.safe_load(read(os.path.join(BACKEND, 'api', 'swagger.yaml')))
    kinds = ('/emergency-contacts', '/education', '/prior-employment', '/change-requests')
    out = []
    for path, item in doc['paths'].items():
        if path.startswith('/hr/employee-change-requests') or any(path.startswith(b + k) for b in ('/hr/employees/me', '/hr/employees/{id}') for k in kinds):
            for method in ('get', 'post', 'put', 'patch', 'delete'):
                if method in item:
                    out.append((method.upper(), '/api/v1' + re.sub(r'\{(\w+)\}', r':\1', path), '(employee/records.Register; from the contract)'))
    return out


def routes_in(module, p):
    rows = []
    groups = {}
    # Join statements that span lines (e.g. users.POST("",\n mw,\n handler)) so every route keeps its chain.
    joined, buf, depth = [], '', 0
    for raw in read(p).split('\n'):
        if depth == 0 and not re.search(r'\.(GET|POST|PUT|PATCH|DELETE|Group)\(', raw):
            joined.append(raw)
            continue
        buf += ' ' + raw.strip()
        depth += raw.count('(') - raw.count(')')
        if depth <= 0:
            joined.append(buf)
            buf, depth = '', 0
    for line in joined:
        g = re.search(r'(\w+)\s*:?=\s*\w+\.Group\("([^"]*)"(.*)\)', line)
        if g:
            parent = re.search(r'=\s*(\w+)\.Group', line).group(1)
            pprefix, pmw = groups.get(parent, ('', ''))
            gmw = g.group(3).strip().strip(', ')
            groups[g.group(1)] = (pprefix + g.group(2), ', '.join(x for x in (pmw, gmw) if x))
            continue
        u = re.search(r'^\s*(\w+)\.Use\((.*)\)\s*$', line)
        if u and u.group(1) in groups:
            pfx, mw = groups[u.group(1)]
            groups[u.group(1)] = (pfx, ', '.join(x for x in (mw, u.group(2).strip()) if x))
            continue
        r = re.search(r'(\w+)\.(GET|POST|PUT|PATCH|DELETE)\("([^"]*)"(.*)', line)
        if r:
            prefix, gmw = groups.get(r.group(1), ('', ''))
            rest = r.group(4).strip().rstrip(')').strip(', ')
            if 'func(' in rest:  # inline handler: keep only the middleware before it
                rest = rest.split('func(')[0].strip(', ') + (', ' if rest.split('func(')[0].strip(', ') else '') + '(inline handler)'
            if gmw:
                rest = f'[group: {gmw}] ' + rest
            base = '/api/v1/hr' if module in HR else '/api/v1'
            rows.append((r.group(2), base + prefix + r.group(3), rest))
    return rows


def migration_files():
    """Yield (label, path) for every up migration, per schema set (identity, then hr)."""
    base = os.path.join(BACKEND, 'migrations')
    for schema in ('identity', 'hr', 'projects'):
        d = os.path.join(base, schema)
        for f in sorted(os.listdir(d)):
            if f.endswith('.up.sql'):
                yield schema + '/' + f, os.path.join(d, f)


def migrations():
    out = []
    for f, path in migration_files():
        sql = read(path)
        for m in re.finditer(r'CREATE TABLE (?:IF NOT EXISTS )?(\w+) \((.*?)\n\);', sql, re.S):
            cols = []
            for raw in m.group(2).split('\n'):
                raw = raw.strip().rstrip(',')
                if not raw or raw.startswith('--'):
                    continue
                cm = re.match(r'^(\w+) ([A-Z]+(?:\([0-9, ]+\))?(?:\[\])?)(.*)$', raw)
                if cm and cm.group(1).upper() not in ('CHECK', 'UNIQUE', 'PRIMARY', 'CONSTRAINT', 'FOREIGN'):
                    extra = cm.group(3).strip()
                    cols.append(f'`{cm.group(1)}` {cm.group(2)}' + (f' — {extra}' if extra else ''))
                else:
                    cols.append(f'_constraint:_ {raw}')
            out.append((f, m.group(1), cols))
        for m in re.finditer(r'CREATE (UNIQUE )?INDEX (?:IF NOT EXISTS )?(\w+) ON (\w+)\s*(.*?);', sql, re.S):
            out.append((f, '__index__', [f'{"UNIQUE " if m.group(1) else ""}index `{m.group(2)}` on `{m.group(3)}` {" ".join(m.group(4).split())}']))
        for m in re.finditer(r'ALTER TABLE (\w+)(.*?);', sql, re.S):
            out.append((f, '__alter__', [f'ALTER `{m.group(1)}` {" ".join(m.group(2).split())}']))
    return out


def md_escape(s):
    return s.replace('|', '\\|')


def main():
    w = []
    w.append('# Appendix — Generated inventory (from source, read-only)\n')
    w.append('Every non-test Go file under `backend/internal`, with its exported and unexported top-level types, functions, '
             'methods and interface methods (signature + doc comment), every HTTP route with its middleware/permission chain, '
             'every SQL table with its columns and constraints, and every test function. Regenerate rather than hand-edit.\n')

    w.append('\n## A. HTTP routes (all modules)\n')
    total = 0
    for mod in MODULE_ORDER:
        rs = routes(mod)
        if not rs:
            continue
        total += len(rs)
        w.append(f'\n### {TITLES[mod]} — {len(rs)} routes\n')
        w.append('| Method | Path | Middleware / permission → handler |\n|---|---|---|')
        for meth, path, rest in rs:
            w.append(f'| {meth} | `{path}` | {md_escape(rest)} |')
    w.append(f'\nTotal routes registered by module route files: **{total}** (plus `GET /health`).\n')

    w.append('\n## B. Database schema (migrations, in order)\n')
    cur = None
    for f, table, cols in migrations():
        if f != cur:
            w.append(f'\n#### `{f}`')
            cur = f
        if table.startswith('__'):
            for c in cols:
                w.append(f'- {c}')
        else:
            w.append(f'- **table `{table}`**')
            for c in cols:
                w.append(f'  - {c}')

    w.append('\n## C. Code inventory — every file, type and function\n')
    for mod in MODULE_ORDER:
        w.append(f'\n### {TITLES[mod]}\n')
        for path in go_files(mod):
            src = read(path)
            rel = os.path.relpath(path, BACKEND).replace('\\', '/')
            loc = src.count('\n')
            w.append(f'\n#### `{rel}` ({loc} lines)')
            pd = package_doc(src)
            if pd:
                w.append(f'_Package:_ {pd}')
            for kind, sig, doc in decls(src):
                line = f'- **{kind}** `{sig}`'
                if doc:
                    line += f' — {doc}'
                w.append(line)

    w.append('\n## D. Test inventory\n')
    for mod in MODULE_ORDER:
        tf = test_files(mod)
        if not tf:
            continue
        n = sum(len(x[1]) for x in tf)
        w.append(f'\n### {TITLES[mod]} — {n} test functions in {len(tf)} files')
        for rel, names in tf:
            w.append(f'- `{rel}`: {", ".join(names) if names else "(helpers only)"}')
    integ = os.path.join(BACKEND, 'tests', 'integration')
    w.append('\n### Integration suite (`backend/tests/integration`)')
    for f in sorted(os.listdir(integ)):
        if f.endswith('_test.go'):
            names = re.findall(r'^func (Test\w+)', read(os.path.join(integ, f)), re.M)
            w.append(f'- `{f}`: {", ".join(names) if names else "(helpers only)"}')

    w.append('\n## E. Business rules, errors and decisions (verbatim from each module bundle)\n')
    mods = os.path.join(ROOT, 'docs', 'modules')
    bundles = [('m0-identity-access', os.path.join(mods, 'm0-identity-access')), ('m2-projects-tasks', os.path.join(mods, 'm2-projects-tasks'))]
    ctx_root = os.path.join(mods, 'm1-hr-management', 'contexts')
    for ctx in MODULE_ORDER:
        if os.path.isdir(os.path.join(ctx_root, ctx)):
            bundles.append((f'm1-hr-management/contexts/{ctx}', os.path.join(ctx_root, ctx)))
    for b, base in bundles:
        sources = [f for f in ('specification.md',)
                   if os.path.exists(os.path.join(base, f))]
        spec = '\n'.join(read(os.path.join(base, f)) for f in sources)
        if not spec or 'DESIGN PENDING' in spec[:400]:
            continue
        w.append(f'\n### {b}  _(from {", ".join(sources)})_')
        rows = [l for l in spec.split('\n') if re.match(r'^\| *\**[A-Z]{1,4}(-[A-Z]{1,4})?-?\d{1,3}\**[a-z]? *\|', l)]
        if rows:
            w.append('\n**Requirements and rules (ID-tagged table rows)**\n\n| ID | Text | … |\n|---|---|---|')
            w.extend(rows)
        frs = [l for l in spec.split('\n') if re.match(r'^\s*[-*] \**(FR|NFR|BR|EC|AC)-?[A-Z]*-?\d', l)]
        if frs:
            w.append('\n**Requirements and rules (ID-tagged bullets)**\n')
            w.extend(frs)
        err = re.search(r'\n#{2,3} \d+\. Errors\n(.*?)(?=\n#{2,3} )', spec, re.S)
        if err:
            w.append('\n**Errors**\n')
            w.append(err.group(1).strip())
        dec = os.path.join(base, 'decisions.md')
        if os.path.exists(dec):
            body = [l for l in read(dec).split('\n') if l.strip() and not l.startswith('# ')]
            if body:
                w.append('\n**Decision log (decisions.md, verbatim)**\n')
                w.extend(l.replace('## ', '##### ', 1) if l.startswith('## ') else l for l in body)

    with open(OUT, 'w', encoding='utf-8', newline='\n') as fh:
        fh.write('\n'.join(w) + '\n')
    print(OUT, len(w), 'lines')


main()
