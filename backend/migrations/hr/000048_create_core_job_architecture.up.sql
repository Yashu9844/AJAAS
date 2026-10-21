-- HR/Core job architecture (CO-001..CO-003). btree_gist backs the non-overlap exclusion constraints (DC-03).
CREATE EXTENSION IF NOT EXISTS btree_gist;

CREATE TABLE core_job_families (
    id UUID PRIMARY KEY,
    tenant_id UUID NOT NULL,
    code VARCHAR(32) NOT NULL,
    name VARCHAR(150) NOT NULL,
    description VARCHAR(1000),
    status VARCHAR(20) NOT NULL DEFAULT 'active' CHECK (status IN ('active', 'inactive')),
    created_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP
);
CREATE UNIQUE INDEX uq_core_job_families_code ON core_job_families (tenant_id, code);

CREATE TABLE core_grades (
    id UUID PRIMARY KEY,
    tenant_id UUID NOT NULL,
    code VARCHAR(32) NOT NULL,
    name VARCHAR(150) NOT NULL,
    rank INT NOT NULL CHECK (rank >= 1),
    status VARCHAR(20) NOT NULL DEFAULT 'active' CHECK (status IN ('active', 'inactive')),
    created_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP
);
CREATE UNIQUE INDEX uq_core_grades_code ON core_grades (tenant_id, code);
CREATE UNIQUE INDEX uq_core_grades_rank ON core_grades (tenant_id, rank);

CREATE TABLE core_job_profiles (
    id UUID PRIMARY KEY,
    tenant_id UUID NOT NULL,
    code VARCHAR(32) NOT NULL,
    title VARCHAR(150) NOT NULL,
    family_id UUID NOT NULL REFERENCES core_job_families(id),
    default_grade_id UUID REFERENCES core_grades(id),
    category VARCHAR(20) NOT NULL CHECK (category IN ('staff', 'management', 'executive')),
    probation_days INT NOT NULL DEFAULT 90 CHECK (probation_days BETWEEN 0 AND 365),
    description VARCHAR(2000),
    status VARCHAR(20) NOT NULL DEFAULT 'active' CHECK (status IN ('active', 'inactive')),
    created_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP
);
CREATE UNIQUE INDEX uq_core_job_profiles_code ON core_job_profiles (tenant_id, code);
CREATE INDEX idx_core_job_profiles_family ON core_job_profiles (tenant_id, family_id);
