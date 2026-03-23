CREATE TABLE IF NOT EXISTS cluster_oidc_settings (
    singleton BOOLEAN PRIMARY KEY DEFAULT TRUE,
    enabled BOOLEAN NOT NULL DEFAULT FALSE,
    issuer_url TEXT NOT NULL DEFAULT '',
    client_id TEXT NOT NULL DEFAULT '',
    client_secret_ciphertext TEXT NOT NULL DEFAULT '',
    redirect_url TEXT NOT NULL DEFAULT '',
    scopes JSONB NOT NULL DEFAULT '[]'::jsonb,
    role_claim TEXT NOT NULL DEFAULT '',
    default_role TEXT NOT NULL DEFAULT 'admin',
    role_map JSONB NOT NULL DEFAULT '{}'::jsonb,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    CHECK (singleton = TRUE)
);
