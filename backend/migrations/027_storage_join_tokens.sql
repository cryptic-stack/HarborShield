CREATE TABLE IF NOT EXISTS storage_join_tokens (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    token_hash TEXT NOT NULL UNIQUE,
    description TEXT NOT NULL DEFAULT '',
    intended_name TEXT NOT NULL DEFAULT '',
    intended_endpoint TEXT NOT NULL DEFAULT '',
    zone TEXT NOT NULL DEFAULT '',
    expires_at TIMESTAMPTZ NOT NULL,
    used_at TIMESTAMPTZ,
    used_by_name TEXT NOT NULL DEFAULT '',
    used_by_endpoint TEXT NOT NULL DEFAULT '',
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS storage_join_tokens_expires_at_idx
    ON storage_join_tokens (expires_at, used_at);
