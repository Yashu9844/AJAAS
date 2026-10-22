-- HR/Onboarding exit interviews (OB-08); restricted visibility.
CREATE TABLE onb_exit_interviews (
    checklist_id UUID PRIMARY KEY REFERENCES onb_checklists(id) ON DELETE CASCADE,
    tenant_id UUID NOT NULL,
    answers JSONB NOT NULL,
    submitted_by_user_id UUID NOT NULL,
    submitted_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP
);
