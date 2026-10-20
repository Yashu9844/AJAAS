-- Audit rows carry the request correlation ID (docs/ARCHITECTURE.md §3.10).
ALTER TABLE audit_logs ADD COLUMN correlation_id UUID;
CREATE INDEX idx_audit_logs_correlation_id ON audit_logs (correlation_id);
