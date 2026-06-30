DROP INDEX IF EXISTS idx_sessions_expires_at;
DROP INDEX IF EXISTS idx_sessions_user_tenant;
DROP INDEX IF EXISTS idx_sessions_tenant_id;
DROP INDEX IF EXISTS idx_sessions_user_id;

DROP TABLE IF EXISTS sessions;
