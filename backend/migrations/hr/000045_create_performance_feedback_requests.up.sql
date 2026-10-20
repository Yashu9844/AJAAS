-- HR/Performance continuous feedback requests (FB-004, FB-005).
CREATE TABLE performance_feedback_requests (
    id UUID PRIMARY KEY,
    tenant_id UUID NOT NULL,
    requester_employee_id UUID NOT NULL REFERENCES employee_profiles(id),
    responder_employee_id UUID NOT NULL REFERENCES employee_profiles(id),
    about_employee_id UUID NOT NULL REFERENCES employee_profiles(id),
    question VARCHAR(500) NOT NULL,
    status VARCHAR(20) NOT NULL DEFAULT 'pending' CHECK (status IN ('pending', 'answered', 'declined')),
    feedback_id UUID,
    created_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP,
    CHECK (responder_employee_id <> about_employee_id),
    CHECK ((status = 'answered') = (feedback_id IS NOT NULL))
);

CREATE UNIQUE INDEX uq_performance_feedback_requests_pending ON performance_feedback_requests (tenant_id, requester_employee_id, responder_employee_id, about_employee_id)
    WHERE status = 'pending';
CREATE INDEX idx_performance_feedback_requests_responder ON performance_feedback_requests (tenant_id, responder_employee_id, status);
CREATE INDEX idx_performance_feedback_requests_requester ON performance_feedback_requests (tenant_id, requester_employee_id, status);
