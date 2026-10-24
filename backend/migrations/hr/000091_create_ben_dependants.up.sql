-- HR/Benefits dependants and life events: sensitive (BN-C09).
CREATE TABLE ben_dependants (
    id UUID PRIMARY KEY,
    tenant_id UUID NOT NULL,
    employee_profile_id UUID NOT NULL,
    name VARCHAR(100) NOT NULL,
    relationship VARCHAR(10) NOT NULL CHECK (relationship IN ('spouse', 'child', 'parent', 'other')),
    date_of_birth DATE NOT NULL,
    status VARCHAR(10) NOT NULL DEFAULT 'active' CHECK (status IN ('active', 'removed')),
    created_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP
);
CREATE INDEX idx_ben_dependants_employee ON ben_dependants (tenant_id, employee_profile_id, status);

CREATE TABLE ben_life_events (
    id UUID PRIMARY KEY,
    tenant_id UUID NOT NULL,
    employee_profile_id UUID NOT NULL,
    event_type VARCHAR(10) NOT NULL CHECK (event_type IN ('marriage', 'birth', 'adoption', 'divorce', 'death', 'other')),
    event_date DATE NOT NULL,
    evidence_ref VARCHAR(300),
    recorded_by_user_id UUID NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP
);
CREATE INDEX idx_ben_life_events_employee ON ben_life_events (tenant_id, employee_profile_id, event_date DESC);
