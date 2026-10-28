-- Consumer deduplication: a handler records the event id in the same transaction as its effect, so a redelivered
-- event is recognised and skipped (at-least-once delivery, exactly-once effect).
CREATE TABLE projects_events_inbox (
    event_id UUID NOT NULL,
    consumer VARCHAR(100) NOT NULL,
    tenant_id UUID NOT NULL,
    event_type VARCHAR(100) NOT NULL,
    received_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP,
    PRIMARY KEY (consumer, event_id)
);
CREATE INDEX idx_projects_inbox_received ON projects_events_inbox (received_at);
