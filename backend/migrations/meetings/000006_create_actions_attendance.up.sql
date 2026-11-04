-- Action items are M2 tasks; M3 keeps the link and a display snapshot only (DMT-05).
CREATE TABLE meeting_action_links (
    id UUID PRIMARY KEY,
    tenant_id UUID NOT NULL,
    occurrence_id UUID NOT NULL REFERENCES meeting_occurrences(id) ON DELETE CASCADE,
    agenda_item_id UUID REFERENCES meeting_agenda_items(id) ON DELETE SET NULL,
    task_id UUID NOT NULL,
    created_by UUID NOT NULL,
    snapshot_title VARCHAR(500) NOT NULL DEFAULT '',
    snapshot_status VARCHAR(30) NOT NULL DEFAULT '',
    snapshot_due_at TIMESTAMPTZ,
    stale_at TIMESTAMPTZ,
    created_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP
);
CREATE UNIQUE INDEX uq_meeting_action_link ON meeting_action_links (occurrence_id, task_id);
CREATE INDEX idx_meeting_action_links_task ON meeting_action_links (tenant_id, task_id);

CREATE TABLE meeting_attendance (
    tenant_id UUID NOT NULL,
    occurrence_id UUID NOT NULL REFERENCES meeting_occurrences(id) ON DELETE CASCADE,
    user_id UUID NOT NULL,
    state VARCHAR(10) NOT NULL CHECK (state IN ('present', 'late', 'absent', 'excused')),
    marked_by UUID NOT NULL,
    updated_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP,
    PRIMARY KEY (occurrence_id, user_id)
);
