-- Outgoing webhooks (specification section 9, phase 4): per-project endpoints that receive M2 events.
-- The signing secret is stored encrypted (AES-256-GCM, nonce prefixed); only a four-character hint is readable.
CREATE TABLE webhooks (
    id UUID PRIMARY KEY,
    tenant_id UUID NOT NULL,
    project_id UUID NOT NULL REFERENCES projects (id) ON DELETE CASCADE,
    name VARCHAR(60) NOT NULL,
    url VARCHAR(500) NOT NULL,
    format VARCHAR(8) NOT NULL DEFAULT 'generic' CHECK (format IN ('generic', 'slack', 'discord')),
    event_types JSONB NOT NULL DEFAULT '["*"]',
    active BOOLEAN NOT NULL DEFAULT TRUE,
    secret_enc BYTEA NOT NULL,
    secret_hint VARCHAR(8) NOT NULL DEFAULT '',
    failure_count INT NOT NULL DEFAULT 0,
    disabled_at TIMESTAMPTZ,
    created_by UUID NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP
);
CREATE UNIQUE INDEX uq_webhooks_project_name ON webhooks (project_id, lower(name));
CREATE INDEX idx_webhooks_project ON webhooks (tenant_id, project_id);

-- One row per (webhook, event). attempts counts sends; next_attempt_at is NULL once the delivery is settled.
CREATE TABLE webhook_deliveries (
    id UUID PRIMARY KEY,
    tenant_id UUID NOT NULL,
    webhook_id UUID NOT NULL REFERENCES webhooks (id) ON DELETE CASCADE,
    event_id UUID NOT NULL,
    event_type VARCHAR(100) NOT NULL,
    status VARCHAR(10) NOT NULL DEFAULT 'pending' CHECK (status IN ('pending', 'delivered', 'failed', 'dead')),
    attempts INT NOT NULL DEFAULT 0,
    next_attempt_at TIMESTAMPTZ,
    response_status INT,
    last_error VARCHAR(120) NOT NULL DEFAULT '',
    created_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP,
    delivered_at TIMESTAMPTZ
);
CREATE UNIQUE INDEX uq_webhook_deliveries_event ON webhook_deliveries (webhook_id, event_id);
CREATE INDEX idx_webhook_deliveries_due ON webhook_deliveries (next_attempt_at) WHERE status IN ('pending', 'failed') AND next_attempt_at IS NOT NULL;
CREATE INDEX idx_webhook_deliveries_list ON webhook_deliveries (tenant_id, webhook_id, created_at DESC, id DESC);

-- The dispatcher's own position in projects_events_outbox, independent of the broker relay's published flag.
CREATE TABLE projects_dispatch_cursor (
    id SMALLINT PRIMARY KEY CHECK (id = 1),
    last_created_at TIMESTAMPTZ NOT NULL,
    updated_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP
);
INSERT INTO projects_dispatch_cursor (id, last_created_at) VALUES (1, CURRENT_TIMESTAMP);
CREATE INDEX idx_projects_outbox_created ON projects_events_outbox (created_at, id);
