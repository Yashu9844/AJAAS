DROP INDEX IF EXISTS uq_recruitment_offers_open;
UPDATE recruitment_offers SET status = 'withdrawn' WHERE status = 'pending_approval';
CREATE UNIQUE INDEX uq_recruitment_offers_open ON recruitment_offers (candidate_id) WHERE status = 'offered';
ALTER TABLE recruitment_offers DROP CONSTRAINT IF EXISTS recruitment_offers_status_check;
ALTER TABLE recruitment_offers ADD CONSTRAINT recruitment_offers_status_check CHECK (status IN ('offered', 'accepted', 'declined', 'withdrawn'));
ALTER TABLE recruitment_candidates DROP COLUMN IF EXISTS seat_reserved;
ALTER TABLE recruitment_jobs DROP COLUMN IF EXISTS reserved_count;
