-- HR/Onboarding exit clearance sign-offs (OB-07).
CREATE TABLE onb_clearances (
    tenant_id UUID NOT NULL,
    checklist_id UUID NOT NULL REFERENCES onb_checklists(id) ON DELETE CASCADE,
    area VARCHAR(20) NOT NULL CHECK (area IN ('manager', 'hr', 'it', 'finance')),
    cleared_by_user_id UUID,
    cleared_at TIMESTAMPTZ,
    note VARCHAR(1000),
    PRIMARY KEY (checklist_id, area)
);
