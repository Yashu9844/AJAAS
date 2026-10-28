-- Custom fields (specification section 9, phase 4): typed, project-scoped definitions and per-task values.
CREATE TABLE custom_field_definitions (
    id UUID PRIMARY KEY,
    tenant_id UUID NOT NULL,
    project_id UUID NOT NULL REFERENCES projects (id) ON DELETE CASCADE,
    name VARCHAR(60) NOT NULL,
    key VARCHAR(40) NOT NULL,
    type VARCHAR(16) NOT NULL CHECK (type IN ('text', 'number', 'checkbox', 'date', 'url', 'select', 'multiselect', 'user')),
    options JSONB NOT NULL DEFAULT '[]',
    position INT NOT NULL DEFAULT 0,
    created_by UUID NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP
);
CREATE UNIQUE INDEX uq_custom_field_key ON custom_field_definitions (project_id, key);
CREATE UNIQUE INDEX uq_custom_field_name ON custom_field_definitions (project_id, lower(name));

CREATE TABLE custom_field_values (
    task_id UUID NOT NULL REFERENCES tasks (id) ON DELETE CASCADE,
    field_id UUID NOT NULL REFERENCES custom_field_definitions (id) ON DELETE CASCADE,
    tenant_id UUID NOT NULL,
    value JSONB NOT NULL,
    updated_by UUID NOT NULL,
    updated_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP,
    PRIMARY KEY (task_id, field_id)
);
CREATE INDEX idx_custom_field_values_field ON custom_field_values (field_id);
