-- HR/Leave policy per tenant (LP-01, LP-02): leave-year start, comp-off type and expiry.
CREATE TABLE leave_policies (
    tenant_id UUID PRIMARY KEY,
    year_start_month SMALLINT NOT NULL DEFAULT 1 CHECK (year_start_month BETWEEN 1 AND 12),
    comp_off_leave_type_id UUID REFERENCES leave_types(id),
    comp_off_expiry_days INT NOT NULL DEFAULT 90 CHECK (comp_off_expiry_days BETWEEN 1 AND 365),
    updated_by_user_id UUID NOT NULL,
    updated_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP
);
