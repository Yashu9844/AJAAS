-- HR/Onboarding templates and their tasks (OB-01, OB-02).
CREATE TABLE onb_templates (
    id UUID PRIMARY KEY,
    tenant_id UUID NOT NULL,
    kind VARCHAR(20) NOT NULL CHECK (kind IN ('onboarding', 'offboarding')),
    name VARCHAR(150) NOT NULL,
    department_id UUID REFERENCES departments(id),
    job_profile_id UUID REFERENCES core_job_profiles(id),
    location_id UUID,
    worker_type VARCHAR(20),
    status VARCHAR(20) NOT NULL CHECK (status IN ('active', 'archived')),
    created_by_user_id UUID NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP
);
CREATE INDEX idx_onb_templates_tenant ON onb_templates (tenant_id, kind, status);

CREATE TABLE onb_template_tasks (
    id UUID PRIMARY KEY,
    tenant_id UUID NOT NULL,
    template_id UUID NOT NULL REFERENCES onb_templates(id) ON DELETE CASCADE,
    position INT NOT NULL,
    title VARCHAR(200) NOT NULL,
    description VARCHAR(2000),
    owner_role VARCHAR(20) NOT NULL CHECK (owner_role IN ('employee', 'manager', 'hr', 'it', 'finance')),
    due_offset_days INT NOT NULL CHECK (due_offset_days BETWEEN -90 AND 365),
    document_type VARCHAR(50),
    blocking BOOLEAN NOT NULL DEFAULT FALSE,
    UNIQUE (template_id, position)
);
