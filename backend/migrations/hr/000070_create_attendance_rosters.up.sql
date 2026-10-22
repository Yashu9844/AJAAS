-- HR/Attendance rosters: a repeating pattern of shift segments (AP-05).
CREATE TABLE attendance_rosters (
    id UUID PRIMARY KEY,
    tenant_id UUID NOT NULL,
    name VARCHAR(100) NOT NULL,
    pattern JSONB NOT NULL,
    status VARCHAR(20) NOT NULL CHECK (status IN ('active', 'archived')),
    created_by_user_id UUID NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP
);
CREATE UNIQUE INDEX uq_attendance_rosters_name ON attendance_rosters (tenant_id, lower(name));
