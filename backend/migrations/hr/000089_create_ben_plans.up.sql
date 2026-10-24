-- HR/Benefits plans and effective-dated versions (BN-C01..BN-C03).
CREATE TABLE ben_plans (
    id UUID PRIMARY KEY,
    tenant_id UUID NOT NULL,
    code VARCHAR(30) NOT NULL,
    name VARCHAR(150) NOT NULL,
    category VARCHAR(12) NOT NULL CHECK (category IN ('health', 'life', 'accident', 'retirement', 'wellness', 'other')),
    provider VARCHAR(100),
    plan_year INT NOT NULL CHECK (plan_year BETWEEN 2000 AND 2100),
    eligibility JSONB NOT NULL DEFAULT '{}',
    status VARCHAR(10) NOT NULL DEFAULT 'draft' CHECK (status IN ('draft', 'active', 'retired')),
    created_by_user_id UUID NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP
);
CREATE UNIQUE INDEX uq_ben_plans_code ON ben_plans (tenant_id, lower(code));
CREATE INDEX idx_ben_plans_tenant ON ben_plans (tenant_id, status);

CREATE TABLE ben_plan_versions (
    tenant_id UUID NOT NULL,
    plan_id UUID NOT NULL REFERENCES ben_plans(id) ON DELETE CASCADE,
    version INT NOT NULL,
    effective_from DATE NOT NULL,
    effective_to DATE,
    levels JSONB NOT NULL,
    created_by_user_id UUID NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP,
    PRIMARY KEY (plan_id, version),
    UNIQUE (plan_id, effective_from),
    CHECK (effective_to IS NULL OR effective_to > effective_from)
);
