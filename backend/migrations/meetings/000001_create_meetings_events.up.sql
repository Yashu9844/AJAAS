-- Transactional outbox and consumer inbox of M3 Meetings (docs/modules/m3-meetings/design.md section 2).
CREATE TABLE meetings_events_outbox (
    id UUID PRIMARY KEY,
    tenant_id UUID NOT NULL,
    event_id UUID NOT NULL UNIQUE,
    routing_key VARCHAR(100) NOT NULL,
    payload JSONB NOT NULL,
    published BOOLEAN NOT NULL DEFAULT FALSE,
    published_at TIMESTAMPTZ,
    attempts INT NOT NULL DEFAULT 0,
    last_error TEXT,
    created_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP
);
CREATE INDEX idx_meetings_outbox_pending ON meetings_events_outbox (created_at) WHERE published = FALSE;

CREATE TABLE meetings_events_inbox (
    event_id UUID NOT NULL,
    consumer VARCHAR(100) NOT NULL,
    tenant_id UUID NOT NULL,
    event_type VARCHAR(100) NOT NULL,
    received_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP,
    PRIMARY KEY (consumer, event_id)
);
CREATE INDEX idx_meetings_inbox_received ON meetings_events_inbox (received_at);
