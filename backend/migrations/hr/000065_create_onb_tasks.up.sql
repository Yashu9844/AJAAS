-- HR/Onboarding checklist tasks (OB-04, OB-05).
CREATE TABLE onb_tasks (
    id UUID PRIMARY KEY,
    tenant_id UUID NOT NULL,
    checklist_id UUID NOT NULL REFERENCES onb_checklists(id) ON DELETE CASCADE,
    position INT NOT NULL,
    title VARCHAR(200) NOT NULL,
    description VARCHAR(2000),
    owner_role VARCHAR(20) NOT NULL CHECK (owner_role IN ('employee', 'manager', 'hr', 'it', 'finance')),
    assignee_employee_id UUID REFERENCES employee_profiles(id),
    due_date DATE NOT NULL,
    document_type VARCHAR(50),
    blocking BOOLEAN NOT NULL DEFAULT FALSE,
    status VARCHAR(20) NOT NULL CHECK (status IN ('open', 'done', 'skipped')),
    evidence_document_id UUID,
    note VARCHAR(1000),
    done_by_user_id UUID,
    done_at TIMESTAMPTZ,
    updated_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP,
    UNIQUE (checklist_id, position)
);
CREATE INDEX idx_onb_tasks_assignee ON onb_tasks (tenant_id, assignee_employee_id, status);
CREATE INDEX idx_onb_tasks_pool ON onb_tasks (tenant_id, status) WHERE assignee_employee_id IS NULL;
