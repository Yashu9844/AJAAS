-- HR/Recruitment offer approval policy and decisions (RP-06, RP-07).
CREATE TABLE recruitment_offer_policies (
    tenant_id UUID PRIMARY KEY,
    required_approvals INT NOT NULL CHECK (required_approvals BETWEEN 0 AND 3),
    threshold_ctc NUMERIC(14,2) NOT NULL DEFAULT 0 CHECK (threshold_ctc >= 0),
    updated_by_user_id UUID NOT NULL,
    updated_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP
);

CREATE TABLE recruitment_offer_approvals (
    id UUID PRIMARY KEY,
    tenant_id UUID NOT NULL,
    offer_id UUID NOT NULL REFERENCES recruitment_offers(id) ON DELETE CASCADE,
    approver_user_id UUID NOT NULL,
    decision VARCHAR(10) NOT NULL CHECK (decision IN ('approved', 'rejected')),
    comment VARCHAR(500),
    decided_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP,
    UNIQUE (offer_id, approver_user_id)
);
