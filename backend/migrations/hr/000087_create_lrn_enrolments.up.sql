-- HR/Learning assignments, enrolments and completions (LN-C05..LN-C14).
CREATE TABLE lrn_assignments (
    id UUID PRIMARY KEY,
    tenant_id UUID NOT NULL,
    target_kind VARCHAR(12) NOT NULL CHECK (target_kind IN ('employee', 'department', 'job_profile', 'location', 'all')),
    target_id UUID,
    course_id UUID REFERENCES lrn_courses(id),
    plan_id UUID REFERENCES lrn_plans(id),
    mandatory BOOLEAN NOT NULL DEFAULT TRUE,
    due_date DATE,
    due_in_days INT CHECK (due_in_days BETWEEN 1 AND 1000),
    recurrence_days INT CHECK (recurrence_days BETWEEN 30 AND 3650),
    status VARCHAR(10) NOT NULL DEFAULT 'active' CHECK (status IN ('active', 'cancelled')),
    cycle INT NOT NULL DEFAULT 0,
    next_cycle_on DATE,
    created_by_user_id UUID NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP,
    CONSTRAINT lrn_assignments_one_content CHECK ((course_id IS NULL) <> (plan_id IS NULL)),
    CONSTRAINT lrn_assignments_target CHECK ((target_kind = 'all') = (target_id IS NULL)),
    CONSTRAINT lrn_assignments_due CHECK (due_date IS NULL OR due_in_days IS NULL),
    CONSTRAINT lrn_assignments_recurrence CHECK (recurrence_days IS NULL OR (due_date IS NULL AND next_cycle_on IS NOT NULL))
);
CREATE INDEX idx_lrn_assignments_tenant ON lrn_assignments (tenant_id, status);

CREATE TABLE lrn_enrolments (
    id UUID PRIMARY KEY,
    tenant_id UUID NOT NULL,
    employee_profile_id UUID NOT NULL,
    course_id UUID NOT NULL REFERENCES lrn_courses(id),
    plan_id UUID REFERENCES lrn_plans(id),
    assignment_id UUID REFERENCES lrn_assignments(id),
    source VARCHAR(10) NOT NULL CHECK (source IN ('assigned', 'self')),
    mandatory BOOLEAN NOT NULL DEFAULT FALSE,
    cycle INT NOT NULL DEFAULT 0,
    status VARCHAR(12) NOT NULL DEFAULT 'not_started' CHECK (status IN ('not_started', 'in_progress', 'completed', 'waived')),
    due_date DATE,
    department_id UUID,
    dedupe_key VARCHAR(120) NOT NULL,
    started_at TIMESTAMPTZ,
    completed_at TIMESTAMPTZ,
    waive_reason VARCHAR(500),
    created_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP,
    CONSTRAINT uq_lrn_enrolments_dedupe UNIQUE (tenant_id, employee_profile_id, dedupe_key)
);
CREATE UNIQUE INDEX uq_lrn_enrolments_live_self ON lrn_enrolments (tenant_id, employee_profile_id, course_id)
    WHERE source = 'self' AND status IN ('not_started', 'in_progress');
CREATE INDEX idx_lrn_enrolments_employee ON lrn_enrolments (tenant_id, employee_profile_id, status);
CREATE INDEX idx_lrn_enrolments_course ON lrn_enrolments (tenant_id, course_id, status);

CREATE TABLE lrn_completions (
    id UUID PRIMARY KEY,
    tenant_id UUID NOT NULL,
    enrolment_id UUID NOT NULL REFERENCES lrn_enrolments(id),
    attestation VARCHAR(10) NOT NULL CHECK (attestation IN ('self', 'manager', 'hr')),
    status VARCHAR(12) NOT NULL CHECK (status IN ('pending', 'verified', 'rejected', 'superseded')),
    score INT CHECK (score BETWEEN 0 AND 100),
    evidence_ref VARCHAR(300),
    certificate_ref VARCHAR(300),
    completed_on DATE NOT NULL,
    submitted_by_user_id UUID NOT NULL,
    decided_by_user_id UUID,
    decision_comment VARCHAR(500),
    supersedes_id UUID REFERENCES lrn_completions(id),
    correction_reason VARCHAR(500),
    created_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP,
    decided_at TIMESTAMPTZ
);
CREATE UNIQUE INDEX uq_lrn_completions_live ON lrn_completions (enrolment_id) WHERE status IN ('pending', 'verified');
CREATE INDEX idx_lrn_completions_pending ON lrn_completions (tenant_id) WHERE status = 'pending';
