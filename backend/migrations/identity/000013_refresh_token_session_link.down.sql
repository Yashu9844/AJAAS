DROP INDEX IF EXISTS idx_refresh_tokens_session_id;
ALTER TABLE refresh_tokens DROP COLUMN IF EXISTS revoked_reason;
ALTER TABLE refresh_tokens DROP COLUMN IF EXISTS session_id;
