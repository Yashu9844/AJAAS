-- HR/Employee education history (FR-ER002).
CREATE TABLE employee_education (
    id UUID PRIMARY KEY,
    tenant_id UUID NOT NULL,
    employee_profile_id UUID NOT NULL REFERENCES employee_profiles(id) ON DELETE CASCADE,
    institution VARCHAR(200) NOT NULL,
    qualification VARCHAR(150) NOT NULL,
    field_of_study VARCHAR(150),
    start_year INT CHECK (start_year IS NULL OR start_year BETWEEN 1900 AND 2200),
    end_year INT CHECK (end_year IS NULL OR end_year BETWEEN 1900 AND 2200),
    grade VARCHAR(50),
    created_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP,
    CHECK (start_year IS NULL OR end_year IS NULL OR end_year >= start_year)
);
CREATE INDEX idx_employee_education_profile ON employee_education (tenant_id, employee_profile_id);
