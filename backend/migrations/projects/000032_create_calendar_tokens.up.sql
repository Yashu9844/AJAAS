-- Per-user revocable iCalendar feed tokens (VW-03). Only the SHA-256 hash is stored; the raw token is shown once.
-- The feed route has no bearer token, so the tenant and user are resolved from this row, never from the client.
CREATE TABLE calendar_tokens (
    id UUID PRIMARY KEY,
    tenant_id UUID NOT NULL,
    user_id UUID NOT NULL,
    token_hash CHAR(64) NOT NULL,
    label VARCHAR(60) NOT NULL DEFAULT '',
    created_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP,
    last_used_at TIMESTAMPTZ,
    revoked_at TIMESTAMPTZ
);
CREATE UNIQUE INDEX uq_calendar_tokens_hash ON calendar_tokens (token_hash);
CREATE INDEX idx_calendar_tokens_user ON calendar_tokens (tenant_id, user_id) WHERE revoked_at IS NULL;
