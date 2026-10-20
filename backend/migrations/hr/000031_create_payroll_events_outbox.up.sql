CREATE TABLE payroll_events_outbox (
    id UUID PRIMARY KEY,
    tenant_id UUID NOT NULL,
    event_id UUID NOT NULL,
    event_type VARCHAR(100) NOT NULL,
    routing_key VARCHAR(100) NOT NULL,
    payload JSONB NOT NULL,
    published BOOLEAN NOT NULL DEFAULT FALSE,
    published_at TIMESTAMPTZ,
    attempts INT NOT NULL DEFAULT 0,
    last_error TEXT,
    created_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP
);

CREATE UNIQUE INDEX uq_payroll_outbox_event_id ON payroll_events_outbox (event_id);
CREATE INDEX idx_payroll_outbox_tenant ON payroll_events_outbox (tenant_id);
CREATE INDEX idx_payroll_outbox_published ON payroll_events_outbox (published) WHERE published = FALSE;
