CREATE TABLE org_events_outbox (
    id UUID PRIMARY KEY,
    tenant_id UUID NOT NULL,
    event_type VARCHAR(100) NOT NULL,
    routing_key VARCHAR(200) NOT NULL,
    payload JSONB NOT NULL,
    correlation_id UUID,
    attempts INT NOT NULL DEFAULT 0,
    next_retry_at TIMESTAMPTZ,
    created_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP
);

CREATE INDEX idx_org_outbox_next_retry ON org_events_outbox (next_retry_at) WHERE next_retry_at IS NOT NULL;
CREATE INDEX idx_org_outbox_tenant_id ON org_events_outbox (tenant_id);
