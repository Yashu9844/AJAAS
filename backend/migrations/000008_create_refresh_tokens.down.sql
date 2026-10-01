DROP INDEX IF EXISTS idx_refresh_tokens_user_tenant;
DROP INDEX IF EXISTS idx_refresh_tokens_tenant_id;
DROP INDEX IF EXISTS idx_refresh_tokens_user_id;
DROP INDEX IF EXISTS uq_refresh_tokens_token_hash;

DROP TABLE IF EXISTS refresh_tokens;
