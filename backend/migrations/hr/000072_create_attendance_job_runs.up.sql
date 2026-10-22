-- HR/Attendance auto-absent run log (AP-08).
CREATE TABLE attendance_job_runs (
    id UUID PRIMARY KEY,
    tenant_id UUID NOT NULL,
    kind VARCHAR(30) NOT NULL,
    run_date DATE NOT NULL,
    absent INT NOT NULL,
    week_off INT NOT NULL,
    holiday INT NOT NULL,
    skipped INT NOT NULL,
    triggered_by_user_id UUID,
    ran_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP
);
CREATE INDEX idx_attendance_job_runs_tenant ON attendance_job_runs (tenant_id, kind, run_date DESC);
