ALTER TABLE objects
    ADD COLUMN IF NOT EXISTS lifecycle_delete_at TIMESTAMPTZ;
