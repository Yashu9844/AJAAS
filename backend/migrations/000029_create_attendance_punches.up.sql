CREATE TABLE attendance_punches (
    id UUID PRIMARY KEY,
    tenant_id UUID NOT NULL REFERENCES tenants(id) ON DELETE CASCADE,
    attendance_record_id UUID NOT NULL REFERENCES attendance_records(id) ON DELETE CASCADE,
    employee_profile_id UUID NOT NULL REFERENCES employee_profiles(id) ON DELETE CASCADE,
    punch_time TIMESTAMPTZ NOT NULL,
    punch_type VARCHAR(10) NOT NULL CHECK (punch_type IN ('in', 'out')),
    source VARCHAR(20) NOT NULL,
    latitude NUMERIC(10, 8) CHECK (latitude BETWEEN -90 AND 90),
    longitude NUMERIC(11, 8) CHECK (longitude BETWEEN -180 AND 180),
    device_id VARCHAR(100),
    ip_address VARCHAR(45),
    is_superseded BOOLEAN NOT NULL DEFAULT FALSE,
    created_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP
);

CREATE INDEX idx_attendance_punches_employee_time ON attendance_punches (tenant_id, employee_profile_id, punch_time);
CREATE INDEX idx_attendance_punches_record ON attendance_punches (attendance_record_id);
