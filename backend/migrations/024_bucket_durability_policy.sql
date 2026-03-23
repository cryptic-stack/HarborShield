ALTER TABLE buckets
    ADD COLUMN IF NOT EXISTS storage_class TEXT NOT NULL DEFAULT 'standard',
    ADD COLUMN IF NOT EXISTS replica_target INTEGER;

UPDATE buckets
SET storage_class = 'standard'
WHERE storage_class IS NULL OR storage_class = '';
