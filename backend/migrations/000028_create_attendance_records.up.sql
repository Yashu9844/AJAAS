CREATE TABLE attendance_records (
    id UUID PRIMARY KEY,
    tenant_id UUID NOT NULL REFERENCES tenants(id) ON DELETE CASCADE,
    employee_profile_id UUID NOT NULL REFERENCES employee_profiles(id) ON DELETE CASCADE,
    attendance_date DATE NOT NULL,
    shift_id UUID REFERENCES shifts(id) ON DELETE SET NULL,
    first_punch_in TIMESTAMPTZ,
    last_punch_out TIMESTAMPTZ,
    total_work_minutes INT NOT NULL DEFAULT 0,
    total_break_minutes INT NOT NULL DEFAULT 0,
    late_minutes INT NOT NULL DEFAULT 0,
    overtime_minutes INT NOT NULL DEFAULT 0,
    status VARCHAR(20) NOT NULL,
    source VARCHAR(20) NOT NULL DEFAULT 'web',
    is_regularized BOOLEAN NOT NULL DEFAULT FALSE,
    created_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP
);

CREATE UNIQUE INDEX uq_attendance_records_employee_date ON attendance_records (tenant_id, employee_profile_id, attendance_date);
CREATE INDEX idx_attendance_records_tenant_date_status ON attendance_records (tenant_id, attendance_date, status);
