-- Personal iCalendar feed tokens (hash only, as DPJ-29) and the append-only change log (SC-6).
CREATE TABLE meeting_feed_tokens (
    id UUID PRIMARY KEY,
    tenant_id UUID NOT NULL,
    user_id UUID NOT NULL,
    token_hash CHAR(64) NOT NULL UNIQUE,
    label VARCHAR(60) NOT NULL DEFAULT '',
    created_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP,
    last_used_at TIMESTAMPTZ,
    revoked_at TIMESTAMPTZ
);
CREATE INDEX idx_meeting_feed_tokens_user ON meeting_feed_tokens (tenant_id, user_id) WHERE revoked_at IS NULL;

CREATE TABLE meeting_change_log (
    id UUID PRIMARY KEY,
    tenant_id UUID NOT NULL,
    series_id UUID NOT NULL REFERENCES meeting_series(id) ON DELETE CASCADE,
    occurrence_id UUID REFERENCES meeting_occurrences(id) ON DELETE CASCADE,
    actor_id UUID,
    action VARCHAR(60) NOT NULL,
    changes JSONB NOT NULL DEFAULT '{}',
    reason VARCHAR(500) NOT NULL DEFAULT '',
    created_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP
);
CREATE INDEX idx_meeting_change_log_series ON meeting_change_log (series_id, created_at);
CREATE TRIGGER trg_meeting_change_log_append_only BEFORE UPDATE ON meeting_change_log
    FOR EACH ROW EXECUTE FUNCTION meetings_refuse_update();
