-- HR/Compensation pay ranges per core grade (CO-C01), effective-dated, never overlapping.
CREATE TABLE comp_pay_ranges (
    id UUID PRIMARY KEY,
    tenant_id UUID NOT NULL,
    grade_id UUID NOT NULL REFERENCES core_grades(id),
    min_amount NUMERIC(14,2) NOT NULL CHECK (min_amount > 0),
    mid_amount NUMERIC(14,2) NOT NULL,
    max_amount NUMERIC(14,2) NOT NULL,
    effective_from DATE NOT NULL,
    effective_to DATE,
    created_by_user_id UUID NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP,
    CHECK (min_amount <= mid_amount AND mid_amount <= max_amount),
    CHECK (effective_to IS NULL OR effective_to > effective_from),
    CONSTRAINT ex_comp_pay_ranges_overlap EXCLUDE USING gist (tenant_id WITH =, grade_id WITH =, daterange(effective_from, effective_to) WITH &&)
);
