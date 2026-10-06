#!/usr/bin/env bash
# DEV-ONLY helper for the /dev-test verification console.
# The backend has NO API to create the first user of a tenant (POST /users needs auth),
# so this seeds one active user + the tenant_admin role directly via SQL.
# Usage: scripts/dev-test-bootstrap-admin.sh <tenant-slug> [email] [password]
set -euo pipefail
SLUG="${1:?tenant slug required}"
EMAIL="${2:-admin@${SLUG}.com}"
PASS="${3:-Secret123!}"
PG="docker exec -i ajass-postgres-1 psql -U postgres -d jaas_identity -v ON_ERROR_STOP=1 -q"
$PG <<SQL
CREATE EXTENSION IF NOT EXISTS pgcrypto;
-- Nothing in the backend seeds the global permissions table (finding); seed the pairs the routes check.
INSERT INTO permissions (id, resource, action, description, created_at, updated_at)
SELECT gen_random_uuid(), v.r, v.a, 'dev-test seed', now(), now() FROM (VALUES
 ('users','create'),('users','read'),('users','update'),('users','delete'),
 ('roles','create'),('roles','read'),('roles','update'),('roles','delete'),('permissions','read'),
 ('organization','read'),('organization','update'),
 ('employee','read'),('employee','create'),('employee','update'),('employee','update_sensitive'),('employee','admin')
) AS v(r,a) ON CONFLICT (resource, action) DO NOTHING;
WITH t AS (SELECT id FROM tenants WHERE slug='${SLUG}'),
u AS (
  INSERT INTO users (id, tenant_id, email, password_hash, first_name, last_name, status, created_at, updated_at)
  SELECT gen_random_uuid(), t.id, '${EMAIL}', crypt('${PASS}', gen_salt('bf', 12)), 'Dev', 'Admin', 'active', now(), now() FROM t
  ON CONFLICT (tenant_id, email) DO UPDATE SET status='active'
  RETURNING id, tenant_id)
INSERT INTO user_roles (id, tenant_id, user_id, role_id, created_at, updated_at)
SELECT gen_random_uuid(), u.tenant_id, u.id, r.id, now(), now()
FROM u JOIN roles r ON r.tenant_id=u.tenant_id AND r.name='tenant_admin'
ON CONFLICT DO NOTHING;
SQL
echo "Seeded ${EMAIL} (tenant_admin) in tenant ${SLUG}"
