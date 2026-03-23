CREATE EXTENSION IF NOT EXISTS "pgcrypto";

CREATE TABLE IF NOT EXISTS users (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    email TEXT NOT NULL UNIQUE,
    password_hash TEXT NOT NULL,
    role TEXT NOT NULL,
    must_change_password BOOLEAN NOT NULL DEFAULT FALSE,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE TABLE IF NOT EXISTS refresh_tokens (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    user_id UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    token_hash TEXT NOT NULL,
    expires_at TIMESTAMPTZ NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE TABLE IF NOT EXISTS credentials (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    user_id UUID REFERENCES users(id) ON DELETE SET NULL,
    access_key TEXT NOT NULL UNIQUE,
    secret_hash TEXT NOT NULL,
    description TEXT NOT NULL DEFAULT '',
    role TEXT NOT NULL DEFAULT 'readonly',
    last_used_at TIMESTAMPTZ,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE TABLE IF NOT EXISTS buckets (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    tenant TEXT NOT NULL DEFAULT 'default',
    name TEXT NOT NULL UNIQUE,
    created_by UUID REFERENCES users(id) ON DELETE SET NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE TABLE IF NOT EXISTS objects (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    bucket_id UUID NOT NULL REFERENCES buckets(id) ON DELETE CASCADE,
    object_key TEXT NOT NULL,
    version_id UUID,
    size_bytes BIGINT NOT NULL DEFAULT 0,
    checksum_sha256 TEXT NOT NULL DEFAULT '',
    etag TEXT NOT NULL DEFAULT '',
    content_type TEXT NOT NULL DEFAULT 'application/octet-stream',
    storage_path TEXT NOT NULL,
    storage_backend TEXT NOT NULL DEFAULT 'filesystem',
    encryption_state TEXT NOT NULL DEFAULT 'plaintext',
    retention_until TIMESTAMPTZ,
    deleted_at TIMESTAMPTZ,
    scan_status TEXT NOT NULL DEFAULT 'pending-scan',
    created_by UUID REFERENCES users(id) ON DELETE SET NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    UNIQUE(bucket_id, object_key)
);

CREATE TABLE IF NOT EXISTS bucket_quotas (
    bucket_id UUID PRIMARY KEY REFERENCES buckets(id) ON DELETE CASCADE,
    max_bytes BIGINT,
    max_objects BIGINT,
    warning_threshold_percent INTEGER
);

CREATE TABLE IF NOT EXISTS user_quotas (
    user_id UUID PRIMARY KEY REFERENCES users(id) ON DELETE CASCADE,
    max_bytes BIGINT,
    warning_threshold_percent INTEGER
);

CREATE TABLE IF NOT EXISTS multipart_uploads (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    bucket_id UUID NOT NULL REFERENCES buckets(id) ON DELETE CASCADE,
    object_key TEXT NOT NULL,
    initiated_by UUID REFERENCES users(id) ON DELETE SET NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    expires_at TIMESTAMPTZ NOT NULL
);

CREATE TABLE IF NOT EXISTS audit_logs (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    actor TEXT NOT NULL,
    action TEXT NOT NULL,
    resource TEXT NOT NULL,
    outcome TEXT NOT NULL,
    request_id TEXT NOT NULL,
    detail JSONB NOT NULL DEFAULT '{}'::jsonb,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE TABLE IF NOT EXISTS jobs (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    job_type TEXT NOT NULL,
    payload JSONB NOT NULL DEFAULT '{}'::jsonb,
    status TEXT NOT NULL DEFAULT 'pending',
    attempts INTEGER NOT NULL DEFAULT 0,
    next_run_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);
