-- Read-only anonymous project links. Only the SHA-256 hash of the token is stored; the raw token is shown once. The public
-- route has no bearer token, so the tenant and the project are resolved from this row, never from the client.
CREATE TABLE public_links (
    id UUID PRIMARY KEY,
    tenant_id UUID NOT NULL,
    project_id UUID NOT NULL REFERENCES projects (id) ON DELETE CASCADE,
    token_hash CHAR(64) NOT NULL,
    show_descriptions BOOLEAN NOT NULL DEFAULT FALSE,
    created_by UUID NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP,
    revoked_at TIMESTAMPTZ
);
CREATE UNIQUE INDEX uq_public_links_hash ON public_links (token_hash);
CREATE INDEX idx_public_links_project ON public_links (tenant_id, project_id) WHERE revoked_at IS NULL;
