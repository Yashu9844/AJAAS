-- HR/Learning plans (LN-C04).
CREATE TABLE lrn_plans (
    id UUID PRIMARY KEY,
    tenant_id UUID NOT NULL,
    name VARCHAR(150) NOT NULL,
    description VARCHAR(1000),
    status VARCHAR(10) NOT NULL DEFAULT 'active' CHECK (status IN ('active', 'archived')),
    created_by_user_id UUID NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP
);
CREATE UNIQUE INDEX uq_lrn_plans_name ON lrn_plans (tenant_id, lower(name));

CREATE TABLE lrn_plan_items (
    tenant_id UUID NOT NULL,
    plan_id UUID NOT NULL REFERENCES lrn_plans(id) ON DELETE CASCADE,
    course_id UUID NOT NULL REFERENCES lrn_courses(id),
    position INT NOT NULL,
    mandatory BOOLEAN NOT NULL DEFAULT TRUE,
    PRIMARY KEY (plan_id, course_id),
    UNIQUE (plan_id, position)
);
