-- HR/Performance calibration (PM-018..PM-022): append-only overrides recorded next to the manager review.
CREATE TABLE performance_calibrations (
    id UUID PRIMARY KEY,
    tenant_id UUID NOT NULL,
    cycle_id UUID NOT NULL REFERENCES performance_cycles(id) ON DELETE CASCADE,
    employee_id UUID NOT NULL REFERENCES employee_profiles(id),
    before_score NUMERIC(3,2) NOT NULL CHECK (before_score >= 1 AND before_score <= 5),
    before_potential VARCHAR(10) NOT NULL CHECK (before_potential IN ('low', 'medium', 'high')),
    after_score NUMERIC(3,2) NOT NULL CHECK (after_score >= 1 AND after_score <= 5),
    after_potential VARCHAR(10) NOT NULL CHECK (after_potential IN ('low', 'medium', 'high')),
    reason VARCHAR(1000) NOT NULL,
    calibrated_by_user_id UUID NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP,
    CHECK (before_score <> after_score OR before_potential <> after_potential)
);

CREATE INDEX idx_performance_calibrations_cycle_employee ON performance_calibrations (tenant_id, cycle_id, employee_id, created_at DESC);
