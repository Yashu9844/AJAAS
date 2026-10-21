-- HR/Core actions (CO-030..CO-039): one business-process instance per row; nothing is deleted.
CREATE TABLE core_hr_actions (
    id UUID PRIMARY KEY,
    tenant_id UUID NOT NULL,
    type VARCHAR(30) NOT NULL CHECK (type IN ('hire', 'promote', 'demote', 'transfer', 'change_job', 'change_manager',
        'change_location', 'probation_confirm', 'terminate', 'rehire')),
    employee_id UUID NOT NULL REFERENCES employee_profiles(id),
    effective_date DATE NOT NULL,
    status VARCHAR(20) NOT NULL DEFAULT 'draft' CHECK (status IN ('draft', 'submitted', 'completed', 'rejected', 'cancelled', 'rescinded')),
    reason_code VARCHAR(50) NOT NULL,
    comment VARCHAR(2000),
    changes JSONB NOT NULL DEFAULT '{}'::jsonb,
    initiated_by_user_id UUID NOT NULL,
    decided_by_user_id UUID,
    decision_comment VARCHAR(1000),
    submitted_at TIMESTAMPTZ,
    completed_at TIMESTAMPTZ,
    rescinded_at TIMESTAMPTZ,
    rescind_reason VARCHAR(1000),
    previous_assignment_id UUID,
    previous_effective_to DATE,
    new_assignment_id UUID,
    previous_employee_status VARCHAR(20),
    effects_applied_at TIMESTAMPTZ,
    effects_reversed_at TIMESTAMPTZ,
    created_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP
);
CREATE INDEX idx_core_hr_actions_employee ON core_hr_actions (tenant_id, employee_id, status, effective_date);
CREATE INDEX idx_core_hr_actions_status ON core_hr_actions (tenant_id, status, created_at DESC);
CREATE INDEX idx_core_hr_actions_pending_effects ON core_hr_actions (effective_date)
    WHERE status = 'completed' AND effects_applied_at IS NULL;
