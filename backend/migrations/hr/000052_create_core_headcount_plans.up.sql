-- HR/Core headcount plans (CO-050): periods per department never overlap.
CREATE TABLE core_headcount_plans (
    id UUID PRIMARY KEY,
    tenant_id UUID NOT NULL,
    department_id UUID NOT NULL REFERENCES departments(id),
    period_start DATE NOT NULL,
    period_end DATE NOT NULL,
    planned_fte NUMERIC(7,2) NOT NULL CHECK (planned_fte >= 0),
    created_by_user_id UUID NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP,
    CHECK (period_end >= period_start),
    CONSTRAINT ex_core_headcount_plans_overlap EXCLUDE USING gist (
        tenant_id WITH =, department_id WITH =, daterange(period_start, period_end, '[]') WITH &&
    )
);
