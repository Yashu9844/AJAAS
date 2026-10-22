-- HR/Attendance policy per tenant (AP-03): week-off days, overtime rules, auto-absent switch.
CREATE TABLE attendance_policies (
    tenant_id UUID PRIMARY KEY,
    week_off_mask SMALLINT NOT NULL DEFAULT 65 CHECK (week_off_mask BETWEEN 0 AND 127), -- bit n = weekday n (0 = Sunday)
    overtime_min_daily INT NOT NULL DEFAULT 30 CHECK (overtime_min_daily BETWEEN 0 AND 1440),
    overtime_max_daily INT NOT NULL DEFAULT 240 CHECK (overtime_max_daily BETWEEN 0 AND 1440),
    overtime_max_monthly INT NOT NULL DEFAULT 3600 CHECK (overtime_max_monthly >= 0),
    overtime_rate_weekday INT NOT NULL DEFAULT 150 CHECK (overtime_rate_weekday BETWEEN 100 AND 500),
    overtime_rate_week_off INT NOT NULL DEFAULT 200 CHECK (overtime_rate_week_off BETWEEN 100 AND 500),
    auto_absent_enabled BOOLEAN NOT NULL DEFAULT FALSE,
    updated_by_user_id UUID NOT NULL,
    updated_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP,
    CHECK (overtime_min_daily <= overtime_max_daily)
);
