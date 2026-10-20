-- Durable Idempotency-Key claims and outcomes (docs/ARCHITECTURE.md §3.6). Scope = caller (tenant + user or the
-- platform operator); the unique key prevents concurrent duplicate execution.
CREATE TABLE idempotency_records (
    id UUID PRIMARY KEY,
    scope VARCHAR(200) NOT NULL,
    method VARCHAR(10) NOT NULL,
    path VARCHAR(500) NOT NULL,
    key VARCHAR(255) NOT NULL,
    request_hash CHAR(64) NOT NULL,
    status VARCHAR(20) NOT NULL CONSTRAINT chk_idempotency_status CHECK (status IN ('processing', 'completed')),
    response_status INTEGER NOT NULL DEFAULT 0,
    response_body BYTEA,
    content_type VARCHAR(100) NOT NULL DEFAULT '',
    created_at TIMESTAMPTZ NOT NULL,
    expires_at TIMESTAMPTZ NOT NULL
);
CREATE UNIQUE INDEX uq_idempotency_key ON idempotency_records (scope, method, path, key);
CREATE INDEX idx_idempotency_expires ON idempotency_records (expires_at);
