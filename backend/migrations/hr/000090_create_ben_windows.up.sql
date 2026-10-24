-- HR/Benefits enrolment windows and settings (BN-C04).
CREATE TABLE ben_windows (
    id UUID PRIMARY KEY,
    tenant_id UUID NOT NULL,
    name VARCHAR(100) NOT NULL,
    starts_on DATE NOT NULL,
    ends_on DATE NOT NULL,
    status VARCHAR(10) NOT NULL DEFAULT 'active' CHECK (status IN ('active', 'cancelled')),
    created_by_user_id UUID NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP,
    CHECK (ends_on >= starts_on)
);
CREATE INDEX idx_ben_windows_tenant ON ben_windows (tenant_id, starts_on);

CREATE TABLE ben_settings (
    tenant_id UUID PRIMARY KEY,
    new_hire_days INT NOT NULL CHECK (new_hire_days BETWEEN 0 AND 365),
    life_event_days INT NOT NULL CHECK (life_event_days BETWEEN 0 AND 365),
    updated_by_user_id UUID NOT NULL,
    updated_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP
);
