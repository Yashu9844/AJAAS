CREATE EXTENSION IF NOT EXISTS "uuid-ossp";

CREATE TABLE tenants (
    id UUID PRIMARY KEY,
    name VARCHAR(255) NOT NULL,
    slug VARCHAR(64) NOT NULL,
    domain VARCHAR(255),
    status VARCHAR(20) NOT NULL DEFAULT 'active',
    plan VARCHAR(50) NOT NULL DEFAULT 'free',
    created_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP,
    deleted_at TIMESTAMPTZ
);

CREATE UNIQUE INDEX idx_tenants_slug ON tenants (slug) WHERE deleted_at IS NULL;
CREATE UNIQUE INDEX idx_tenants_domain ON tenants (domain) WHERE domain IS NOT NULL AND deleted_at IS NULL;
CREATE INDEX idx_tenants_status ON tenants (status);
CREATE INDEX idx_tenants_deleted_at ON tenants (deleted_at);
