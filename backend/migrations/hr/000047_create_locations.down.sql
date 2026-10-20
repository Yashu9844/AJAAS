DROP INDEX IF EXISTS idx_mappings_location_id;
ALTER TABLE mappings DROP COLUMN IF EXISTS location_id;
DROP TABLE IF EXISTS locations;
