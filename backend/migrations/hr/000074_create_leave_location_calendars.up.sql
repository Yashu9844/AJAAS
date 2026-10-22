-- HR/Leave per-location holidays and weekly-off patterns (LP-03, LP-04).
CREATE TABLE leave_location_holidays (
    id UUID PRIMARY KEY,
    tenant_id UUID NOT NULL,
    location_id UUID NOT NULL REFERENCES locations(id),
    holiday_date DATE NOT NULL,
    name VARCHAR(100) NOT NULL,
    is_optional BOOLEAN NOT NULL DEFAULT FALSE,
    created_by_user_id UUID NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP,
    UNIQUE (tenant_id, location_id, holiday_date)
);
CREATE INDEX idx_leave_location_holidays_range ON leave_location_holidays (tenant_id, location_id, holiday_date);

CREATE TABLE leave_week_offs (
    tenant_id UUID NOT NULL,
    location_id UUID NOT NULL REFERENCES locations(id),
    week_off_mask SMALLINT NOT NULL CHECK (week_off_mask BETWEEN 0 AND 127), -- bit n = weekday n (0 = Sunday)
    updated_by_user_id UUID NOT NULL,
    updated_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP,
    PRIMARY KEY (tenant_id, location_id)
);
