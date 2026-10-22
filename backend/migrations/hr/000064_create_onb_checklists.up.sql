-- HR/Onboarding checklists (OB-03): one per source, one open per worker and kind.
CREATE TABLE onb_checklists (
    id UUID PRIMARY KEY,
    tenant_id UUID NOT NULL,
    employee_profile_id UUID NOT NULL REFERENCES employee_profiles(id),
    kind VARCHAR(20) NOT NULL CHECK (kind IN ('onboarding', 'offboarding')),
    source VARCHAR(20) NOT NULL CHECK (source IN ('core_action', 'recruitment', 'manual')),
    source_id UUID,
    anchor_date DATE NOT NULL,
    department_id UUID,
    manager_employee_id UUID REFERENCES employee_profiles(id),
    status VARCHAR(20) NOT NULL CHECK (status IN ('open', 'completed', 'cancelled')),
    clearance_completed_at TIMESTAMPTZ,
    completed_at TIMESTAMPTZ,
    closed_by_user_id UUID,
    close_reason VARCHAR(500),
    created_by_user_id UUID,
    created_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP
);
CREATE UNIQUE INDEX uq_onb_checklists_open ON onb_checklists (employee_profile_id, kind) WHERE status = 'open';
CREATE UNIQUE INDEX uq_onb_checklists_source ON onb_checklists (tenant_id, kind, source_id) WHERE source_id IS NOT NULL;
CREATE INDEX idx_onb_checklists_tenant ON onb_checklists (tenant_id, kind, status);
CREATE INDEX idx_onb_checklists_manager ON onb_checklists (tenant_id, manager_employee_id);
