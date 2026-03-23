ALTER TABLE objects
    ADD COLUMN IF NOT EXISTS is_latest BOOLEAN NOT NULL DEFAULT TRUE,
    ADD COLUMN IF NOT EXISTS is_delete_marker BOOLEAN NOT NULL DEFAULT FALSE;

UPDATE objects
SET version_id = gen_random_uuid()
WHERE version_id IS NULL;

ALTER TABLE objects
    ALTER COLUMN version_id SET DEFAULT gen_random_uuid();

DO $$
DECLARE
    constraint_name text;
BEGIN
    SELECT conname INTO constraint_name
    FROM pg_constraint
    WHERE conrelid = 'objects'::regclass
      AND contype = 'u'
      AND conname LIKE '%bucket_id%object_key%';

    IF constraint_name IS NOT NULL THEN
        EXECUTE format('ALTER TABLE objects DROP CONSTRAINT %I', constraint_name);
    END IF;
END $$;

CREATE UNIQUE INDEX IF NOT EXISTS objects_latest_unique_idx
    ON objects (bucket_id, object_key)
    WHERE is_latest = TRUE;

CREATE INDEX IF NOT EXISTS objects_bucket_key_created_idx
    ON objects (bucket_id, object_key, created_at DESC);
