-- Agenda items ordered by fractional rank (as DPJ-09) and private talking points readable by their author only (DMT-07).
CREATE TABLE meeting_agenda_items (
    id UUID PRIMARY KEY,
    tenant_id UUID NOT NULL,
    occurrence_id UUID NOT NULL REFERENCES meeting_occurrences(id) ON DELETE CASCADE,
    rank VARCHAR(64) COLLATE "C" NOT NULL,
    title VARCHAR(200) NOT NULL,
    body_md TEXT NOT NULL DEFAULT '',
    owner_id UUID,
    timebox_min INT CHECK (timebox_min BETWEEN 1 AND 480),
    kind VARCHAR(12) NOT NULL DEFAULT 'discussion' CHECK (kind IN ('info', 'discussion', 'decision')),
    state VARCHAR(12) NOT NULL DEFAULT 'open' CHECK (state IN ('open', 'discussed', 'deferred')),
    created_by UUID NOT NULL,
    version INT NOT NULL DEFAULT 1,
    created_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP
);
CREATE UNIQUE INDEX uq_meeting_agenda_rank ON meeting_agenda_items (occurrence_id, rank);

CREATE TABLE meeting_private_points (
    id UUID PRIMARY KEY,
    tenant_id UUID NOT NULL,
    occurrence_id UUID NOT NULL REFERENCES meeting_occurrences(id) ON DELETE CASCADE,
    author_id UUID NOT NULL,
    body_md TEXT NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP
);
CREATE INDEX idx_meeting_private_points ON meeting_private_points (occurrence_id, author_id);
