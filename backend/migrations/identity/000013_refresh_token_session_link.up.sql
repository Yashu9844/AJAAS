-- Link refresh tokens to the login session they were issued for so logout can revoke them,
-- and remember why a token was revoked (logout vs rotation) so only rotated tokens trigger reuse detection.
ALTER TABLE refresh_tokens ADD COLUMN IF NOT EXISTS session_id UUID;
ALTER TABLE refresh_tokens ADD COLUMN IF NOT EXISTS revoked_reason VARCHAR(20);
CREATE INDEX IF NOT EXISTS idx_refresh_tokens_session_id ON refresh_tokens(session_id);
