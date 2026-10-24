-- HR/Benefits elections (BN-C05, BN-C06, BN-C08).
CREATE TABLE ben_elections (
    id UUID PRIMARY KEY,
    tenant_id UUID NOT NULL,
    employee_profile_id UUID NOT NULL,
    plan_id UUID NOT NULL REFERENCES ben_plans(id),
    level_code VARCHAR(30),
    dependant_ids JSONB NOT NULL DEFAULT '[]',
    status VARCHAR(10) NOT NULL CHECK (status IN ('active', 'waived', 'ended')),
    effective_from DATE NOT NULL,
    effective_to DATE,
    end_reason VARCHAR(12) CHECK (end_reason IN ('changed', 'ended', 'waived', 'replaced', 'termination')),
    end_action_id UUID,
    created_by_user_id UUID NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP,
    CONSTRAINT ben_elections_level CHECK (status <> 'active' OR level_code IS NOT NULL),
    CHECK (effective_to IS NULL OR effective_to >= effective_from)
);
CREATE UNIQUE INDEX uq_ben_elections_live ON ben_elections (tenant_id, employee_profile_id, plan_id) WHERE effective_to IS NULL;
CREATE INDEX idx_ben_elections_employee ON ben_elections (tenant_id, employee_profile_id);
CREATE INDEX idx_ben_elections_plan ON ben_elections (tenant_id, plan_id, status);
