ALTER TABLE objects
    ADD COLUMN IF NOT EXISTS cache_control TEXT NOT NULL DEFAULT '',
    ADD COLUMN IF NOT EXISTS content_disposition TEXT NOT NULL DEFAULT '',
    ADD COLUMN IF NOT EXISTS content_encoding TEXT NOT NULL DEFAULT '',
    ADD COLUMN IF NOT EXISTS user_metadata JSONB NOT NULL DEFAULT '{}'::jsonb;

ALTER TABLE multipart_uploads
    ADD COLUMN IF NOT EXISTS content_type TEXT NOT NULL DEFAULT 'application/octet-stream',
    ADD COLUMN IF NOT EXISTS cache_control TEXT NOT NULL DEFAULT '',
    ADD COLUMN IF NOT EXISTS content_disposition TEXT NOT NULL DEFAULT '',
    ADD COLUMN IF NOT EXISTS content_encoding TEXT NOT NULL DEFAULT '',
    ADD COLUMN IF NOT EXISTS user_metadata JSONB NOT NULL DEFAULT '{}'::jsonb;
