CREATE TABLE attendance_regularizations (
    id UUID PRIMARY KEY,
    tenant_id UUID NOT NULL REFERENCES tenants(id) ON DELETE CASCADE,
    employee_profile_id UUID NOT NULL REFERENCES employee_profiles(id) ON DELETE CASCADE,
    attendance_date DATE NOT NULL,
    attendance_record_id UUID REFERENCES attendance_records(id) ON DELETE SET NULL,
    requested_punch_in TIMESTAMPTZ NOT NULL,
    requested_punch_out TIMESTAMPTZ NOT NULL,
    reason VARCHAR(500) NOT NULL,
    status VARCHAR(20) NOT NULL DEFAULT 'pending',
    requested_by_user_id UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    reviewer_user_id UUID REFERENCES users(id) ON DELETE SET NULL,
    reviewed_at TIMESTAMPTZ,
    review_comment VARCHAR(500),
    created_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP,
    CONSTRAINT chk_attendance_regularizations_window CHECK (requested_punch_out > requested_punch_in)
);

-- AT-015: at most one pending request per employee per date.
CREATE UNIQUE INDEX uq_attendance_regularizations_pending ON attendance_regularizations (tenant_id, employee_profile_id, attendance_date) WHERE status = 'pending';
CREATE INDEX idx_attendance_regularizations_employee_date ON attendance_regularizations (tenant_id, employee_profile_id, attendance_date);
CREATE INDEX idx_attendance_regularizations_tenant_status ON attendance_regularizations (tenant_id, status);
