ALTER TABLE multipart_uploads
ADD COLUMN IF NOT EXISTS content_type TEXT NOT NULL DEFAULT 'application/octet-stream';

CREATE TABLE IF NOT EXISTS multipart_upload_parts (
    upload_id UUID NOT NULL REFERENCES multipart_uploads(id) ON DELETE CASCADE,
    part_number INTEGER NOT NULL,
    size_bytes BIGINT NOT NULL,
    etag TEXT NOT NULL,
    storage_path TEXT NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    PRIMARY KEY (upload_id, part_number)
);
