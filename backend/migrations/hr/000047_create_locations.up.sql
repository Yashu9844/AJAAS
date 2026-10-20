-- HR/Organization locations (FR-L001..FR-L007): physical work sites with state (PT, holidays) and time zone.
CREATE TABLE locations (
    id UUID PRIMARY KEY,
    tenant_id UUID NOT NULL,
    code VARCHAR(32) NOT NULL,
    name VARCHAR(150) NOT NULL,
    address_line1 VARCHAR(200),
    address_line2 VARCHAR(200),
    city VARCHAR(100) NOT NULL,
    state_code VARCHAR(10) NOT NULL,
    country_code CHAR(2) NOT NULL DEFAULT 'IN',
    postal_code VARCHAR(20),
    time_zone VARCHAR(64) NOT NULL DEFAULT 'Asia/Kolkata',
    status VARCHAR(20) NOT NULL DEFAULT 'active' CHECK (status IN ('active', 'inactive')),
    created_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP,
    deleted_at TIMESTAMPTZ
);

CREATE UNIQUE INDEX uq_locations_tenant_code ON locations (tenant_id, code) WHERE deleted_at IS NULL;
CREATE UNIQUE INDEX uq_locations_tenant_name ON locations (tenant_id, lower(name)) WHERE deleted_at IS NULL;
CREATE INDEX idx_locations_tenant_status ON locations (tenant_id, status);
CREATE INDEX idx_locations_deleted_at ON locations (deleted_at);

-- FR-L005: a mapping may name the worker's location.
ALTER TABLE mappings ADD COLUMN location_id UUID REFERENCES locations(id) ON DELETE RESTRICT;
CREATE INDEX idx_mappings_location_id ON mappings (location_id);
