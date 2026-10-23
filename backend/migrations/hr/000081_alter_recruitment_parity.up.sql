-- HR/Recruitment parity (RP-01, RP-06): seat reservation and offers waiting for approval.
ALTER TABLE recruitment_jobs ADD COLUMN reserved_count INT NOT NULL DEFAULT 0 CHECK (reserved_count >= 0);
ALTER TABLE recruitment_candidates ADD COLUMN seat_reserved BOOLEAN NOT NULL DEFAULT FALSE;
ALTER TABLE recruitment_offers DROP CONSTRAINT IF EXISTS recruitment_offers_status_check;
ALTER TABLE recruitment_offers ADD CONSTRAINT recruitment_offers_status_check CHECK (status IN ('pending_approval', 'offered', 'accepted', 'declined', 'withdrawn'));
DROP INDEX IF EXISTS uq_recruitment_offers_open;
CREATE UNIQUE INDEX uq_recruitment_offers_open ON recruitment_offers (candidate_id) WHERE status IN ('offered', 'pending_approval');
