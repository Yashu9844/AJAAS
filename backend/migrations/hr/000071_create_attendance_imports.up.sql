-- HR/Attendance bulk import log (AP-11): counts only, never punch times.
CREATE TABLE attendance_imports (
    id UUID PRIMARY KEY,
    tenant_id UUID NOT NULL,
    rows_total INT NOT NULL,
    rows_applied INT NOT NULL,
    overwrite BOOLEAN NOT NULL DEFAULT FALSE,
    created_by_user_id UUID NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP
);
CREATE INDEX idx_attendance_imports_tenant ON attendance_imports (tenant_id, created_at DESC);
