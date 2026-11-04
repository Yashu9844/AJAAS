-- Membership of a series; the role is an M0 resource role on the series (DMT-09).
CREATE TABLE meeting_participants (
    tenant_id UUID NOT NULL,
    series_id UUID NOT NULL REFERENCES meeting_series(id) ON DELETE CASCADE,
    user_id UUID NOT NULL,
    kind VARCHAR(10) NOT NULL DEFAULT 'required' CHECK (kind IN ('required', 'optional')),
    active BOOLEAN NOT NULL DEFAULT TRUE,
    added_by UUID,
    created_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP,
    PRIMARY KEY (series_id, user_id)
);
CREATE INDEX idx_meeting_participants_user ON meeting_participants (tenant_id, user_id) WHERE active;

-- RSVP: occurrence_id NULL is the answer for the whole series; an occurrence row overrides it.
CREATE TABLE meeting_responses (
    id UUID PRIMARY KEY,
    tenant_id UUID NOT NULL,
    series_id UUID NOT NULL REFERENCES meeting_series(id) ON DELETE CASCADE,
    occurrence_id UUID REFERENCES meeting_occurrences(id) ON DELETE CASCADE,
    user_id UUID NOT NULL,
    rsvp VARCHAR(12) NOT NULL CHECK (rsvp IN ('needs_action', 'accepted', 'tentative', 'declined')),
    comment VARCHAR(500) NOT NULL DEFAULT '',
    responded_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP
);
CREATE UNIQUE INDEX uq_meeting_response ON meeting_responses (series_id, occurrence_id, user_id) NULLS NOT DISTINCT;

-- Recording consent per participant; M9 must check it before any capture (DMT-13).
CREATE TABLE meeting_consents (
    tenant_id UUID NOT NULL,
    series_id UUID NOT NULL REFERENCES meeting_series(id) ON DELETE CASCADE,
    user_id UUID NOT NULL,
    state VARCHAR(10) NOT NULL CHECK (state IN ('granted', 'declined')),
    method VARCHAR(30) NOT NULL DEFAULT 'api',
    decided_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP,
    PRIMARY KEY (series_id, user_id)
);
