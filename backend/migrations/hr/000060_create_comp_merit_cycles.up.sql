-- HR/Compensation merit cycles, budgets and proposals (CO-C20..CO-C26).
CREATE TABLE comp_merit_cycles (
    id UUID PRIMARY KEY,
    tenant_id UUID NOT NULL,
    name VARCHAR(150) NOT NULL,
    effective_date DATE NOT NULL,
    performance_cycle_id UUID,
    guidelines JSONB NOT NULL DEFAULT '[]'::jsonb,
    status VARCHAR(20) NOT NULL CHECK (status IN ('draft', 'open', 'in_review', 'approved', 'applied')),
    created_by_user_id UUID NOT NULL,
    opened_by_user_id UUID,
    approved_by_user_id UUID,
    opened_at TIMESTAMPTZ,
    approved_at TIMESTAMPTZ,
    applied_at TIMESTAMPTZ,
    created_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP
);
CREATE INDEX idx_comp_merit_cycles_tenant ON comp_merit_cycles (tenant_id, status);

CREATE TABLE comp_merit_budgets (
    tenant_id UUID NOT NULL,
    cycle_id UUID NOT NULL REFERENCES comp_merit_cycles(id) ON DELETE CASCADE,
    department_id UUID NOT NULL REFERENCES departments(id),
    amount NUMERIC(14,2) NOT NULL CHECK (amount >= 0),
    PRIMARY KEY (cycle_id, department_id)
);

CREATE TABLE comp_merit_proposals (
    id UUID PRIMARY KEY,
    tenant_id UUID NOT NULL,
    cycle_id UUID NOT NULL REFERENCES comp_merit_cycles(id) ON DELETE CASCADE,
    employee_profile_id UUID NOT NULL REFERENCES employee_profiles(id),
    department_id UUID NOT NULL REFERENCES departments(id),
    manager_employee_id UUID REFERENCES employee_profiles(id),
    current_base NUMERIC(14,2) NOT NULL,
    compa_ratio NUMERIC(6,2),
    compa_band VARCHAR(10) NOT NULL,
    rating_band VARCHAR(10) NOT NULL,
    increase_pct NUMERIC(5,2) NOT NULL DEFAULT 0 CHECK (increase_pct >= 0 AND increase_pct <= 50),
    amount NUMERIC(14,2) NOT NULL DEFAULT 0,
    justification VARCHAR(1000),
    outside_guideline BOOLEAN NOT NULL DEFAULT FALSE,
    override_budget BOOLEAN NOT NULL DEFAULT FALSE,
    updated_by_user_id UUID,
    created_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP,
    UNIQUE (cycle_id, employee_profile_id)
);
CREATE INDEX idx_comp_merit_proposals_manager ON comp_merit_proposals (tenant_id, cycle_id, manager_employee_id);
