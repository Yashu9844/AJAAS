-- A series holds the recurrence rule; occurrences are materialised rows (DMT-02, DMT-03).
CREATE TABLE meeting_series (
    id UUID PRIMARY KEY,
    tenant_id UUID NOT NULL,
    title VARCHAR(200) NOT NULL,
    description_md TEXT NOT NULL DEFAULT '',
    organizer_id UUID NOT NULL,
    tz VARCHAR(64) NOT NULL,
    dtstart_local TIMESTAMP NOT NULL,
    duration_min INT NOT NULL CHECK (duration_min BETWEEN 1 AND 1440),
    all_day BOOLEAN NOT NULL DEFAULT FALSE,
    rrule VARCHAR(500),
    window_until TIMESTAMPTZ,
    visibility VARCHAR(12) NOT NULL DEFAULT 'private' CHECK (visibility IN ('private', 'internal', 'busy_only')),
    confidential BOOLEAN NOT NULL DEFAULT FALSE,
    location VARCHAR(300) NOT NULL DEFAULT '',
    video_url VARCHAR(2048) NOT NULL DEFAULT '',
    default_project_id UUID,
    recording_allowed BOOLEAN NOT NULL DEFAULT FALSE,
    ical_uid VARCHAR(120) NOT NULL,
    sequence INT NOT NULL DEFAULT 0,
    status VARCHAR(12) NOT NULL DEFAULT 'active' CHECK (status IN ('active', 'cancelled')),
    split_from UUID REFERENCES meeting_series(id) ON DELETE SET NULL,
    version INT NOT NULL DEFAULT 1,
    created_by UUID NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP,
    deleted_at TIMESTAMPTZ
);
CREATE UNIQUE INDEX uq_meeting_series_ical ON meeting_series (tenant_id, ical_uid);
CREATE INDEX idx_meeting_series_window ON meeting_series (window_until)
    WHERE rrule IS NOT NULL AND deleted_at IS NULL AND status = 'active';
CREATE INDEX idx_meeting_series_tenant ON meeting_series (tenant_id, visibility) WHERE deleted_at IS NULL;
CREATE INDEX idx_meeting_series_deleted ON meeting_series (deleted_at) WHERE deleted_at IS NOT NULL;
CREATE INDEX idx_meeting_series_title ON meeting_series USING gin (to_tsvector('simple', title));

CREATE TABLE meeting_occurrences (
    id UUID PRIMARY KEY,
    tenant_id UUID NOT NULL,
    series_id UUID NOT NULL REFERENCES meeting_series(id) ON DELETE CASCADE,
    recurrence_id TIMESTAMPTZ NOT NULL,
    start_at TIMESTAMPTZ NOT NULL,
    end_at TIMESTAMPTZ NOT NULL,
    is_exception BOOLEAN NOT NULL DEFAULT FALSE,
    title_override VARCHAR(200),
    location_override VARCHAR(300),
    status VARCHAR(20) NOT NULL DEFAULT 'scheduled'
        CHECK (status IN ('scheduled', 'in_progress', 'ended', 'minutes_published', 'cancelled')),
    status_manual BOOLEAN NOT NULL DEFAULT FALSE,
    cancel_reason VARCHAR(500) NOT NULL DEFAULT '',
    orphaned BOOLEAN NOT NULL DEFAULT FALSE,
    reminder_sent_at TIMESTAMPTZ,
    version INT NOT NULL DEFAULT 1,
    created_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP,
    CONSTRAINT chk_meeting_occurrence_span CHECK (end_at > start_at)
);
CREATE UNIQUE INDEX uq_meeting_occurrence ON meeting_occurrences (series_id, recurrence_id);
CREATE INDEX idx_meeting_occurrences_start ON meeting_occurrences (tenant_id, start_at);
CREATE INDEX idx_meeting_occurrences_series_start ON meeting_occurrences (series_id, start_at);
CREATE INDEX idx_meeting_occurrences_reminder ON meeting_occurrences (start_at) WHERE reminder_sent_at IS NULL AND status = 'scheduled';
