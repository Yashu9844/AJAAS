-- Notes: append-only versions per section (an agenda item, or NULL for the general section). Minutes: frozen published
-- versions (DMT-04). Updates are refused by trigger; deletes happen only through the retention purge.
CREATE FUNCTION meetings_refuse_update() RETURNS trigger LANGUAGE plpgsql AS $$
BEGIN
    RAISE EXCEPTION 'rows of % are append-only', TG_TABLE_NAME USING ERRCODE = 'check_violation';
END $$;

CREATE TABLE meeting_note_versions (
    id UUID PRIMARY KEY,
    tenant_id UUID NOT NULL,
    occurrence_id UUID NOT NULL REFERENCES meeting_occurrences(id) ON DELETE CASCADE,
    section UUID,
    version INT NOT NULL CHECK (version >= 1),
    body_md TEXT NOT NULL,
    author_id UUID NOT NULL,
    source VARCHAR(60) NOT NULL DEFAULT 'human',
    created_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP
);
CREATE UNIQUE INDEX uq_meeting_note_version ON meeting_note_versions (occurrence_id, section, version) NULLS NOT DISTINCT;
CREATE TRIGGER trg_meeting_note_versions_append_only BEFORE UPDATE ON meeting_note_versions
    FOR EACH ROW EXECUTE FUNCTION meetings_refuse_update();

CREATE TABLE meeting_minutes (
    id UUID PRIMARY KEY,
    tenant_id UUID NOT NULL,
    occurrence_id UUID NOT NULL REFERENCES meeting_occurrences(id) ON DELETE CASCADE,
    version INT NOT NULL CHECK (version >= 1),
    body_md TEXT NOT NULL,
    sha256 CHAR(64) NOT NULL,
    published_by UUID NOT NULL,
    reason VARCHAR(500) NOT NULL DEFAULT '',
    created_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP
);
CREATE UNIQUE INDEX uq_meeting_minutes_version ON meeting_minutes (occurrence_id, version);
CREATE INDEX idx_meeting_minutes_search ON meeting_minutes USING gin (to_tsvector('simple', body_md));
CREATE TRIGGER trg_meeting_minutes_append_only BEFORE UPDATE ON meeting_minutes
    FOR EACH ROW EXECUTE FUNCTION meetings_refuse_update();

CREATE TABLE meeting_decisions (
    id UUID PRIMARY KEY,
    tenant_id UUID NOT NULL,
    occurrence_id UUID NOT NULL REFERENCES meeting_occurrences(id) ON DELETE CASCADE,
    agenda_item_id UUID REFERENCES meeting_agenda_items(id) ON DELETE SET NULL,
    text VARCHAR(2000) NOT NULL,
    status VARCHAR(12) NOT NULL DEFAULT 'approved' CHECK (status IN ('proposed', 'approved', 'superseded')),
    decided_by UUID,
    decided_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP,
    recorded_by UUID NOT NULL,
    version INT NOT NULL DEFAULT 1,
    created_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP
);
CREATE INDEX idx_meeting_decisions_occurrence ON meeting_decisions (occurrence_id);
