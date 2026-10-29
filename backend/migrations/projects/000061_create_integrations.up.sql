-- Git integrations (specification section 9): one incoming-webhook endpoint per integration, and the links it creates.
-- The secret is encrypted (AES-256-GCM). Tenant and project come only from this row when a provider calls in.
CREATE TABLE integrations (
    id UUID PRIMARY KEY,
    tenant_id UUID NOT NULL,
    project_id UUID NOT NULL REFERENCES projects (id) ON DELETE CASCADE,
    provider VARCHAR(8) NOT NULL CHECK (provider IN ('github', 'gitea')),
    name VARCHAR(60) NOT NULL,
    secret_enc BYTEA NOT NULL,
    secret_hint VARCHAR(8) NOT NULL DEFAULT '',
    active BOOLEAN NOT NULL DEFAULT TRUE,
    created_by UUID NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP
);
CREATE UNIQUE INDEX uq_integrations_project_name ON integrations (project_id, lower(name));
CREATE INDEX idx_integrations_project ON integrations (tenant_id, project_id);

-- Links from tasks to pull requests, branches and commits. Only ids, urls and state are kept: never titles or messages.
-- The url is validated (https) and never fetched by the backend.
CREATE TABLE external_links (
    id UUID PRIMARY KEY,
    tenant_id UUID NOT NULL,
    task_id UUID NOT NULL REFERENCES tasks (id) ON DELETE CASCADE,
    provider VARCHAR(8) NOT NULL CHECK (provider IN ('github', 'gitea')),
    kind VARCHAR(8) NOT NULL CHECK (kind IN ('pr', 'branch', 'commit')),
    external_id VARCHAR(200) NOT NULL,
    url VARCHAR(500) NOT NULL,
    state VARCHAR(8) NOT NULL DEFAULT 'open' CHECK (state IN ('open', 'merged', 'closed')),
    created_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP
);
CREATE UNIQUE INDEX uq_external_links_ref ON external_links (task_id, provider, kind, external_id);
CREATE INDEX idx_external_links_task ON external_links (tenant_id, task_id, created_at);
