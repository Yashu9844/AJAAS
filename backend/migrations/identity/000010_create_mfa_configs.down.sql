DROP INDEX IF EXISTS idx_mfa_configs_tenant_id;
DROP INDEX IF EXISTS idx_mfa_configs_user_id;
DROP INDEX IF EXISTS uq_mfa_configs_user_type;

DROP TABLE IF EXISTS mfa_configs;
