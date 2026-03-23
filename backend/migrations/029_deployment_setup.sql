CREATE TABLE IF NOT EXISTS deployment_setups (
    singleton BOOLEAN PRIMARY KEY DEFAULT TRUE CHECK (singleton),
    completed BOOLEAN NOT NULL DEFAULT FALSE,
    desired_storage_backend TEXT NOT NULL DEFAULT 'local',
    distributed_scope TEXT NOT NULL DEFAULT '',
    remote_endpoints JSONB NOT NULL DEFAULT '[]'::jsonb,
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);
